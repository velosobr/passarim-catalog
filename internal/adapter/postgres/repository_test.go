package postgres_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/velosobr/passarim-catalog/internal/adapter/postgres"
	"github.com/velosobr/passarim-catalog/internal/domain"
	"github.com/velosobr/passarim-catalog/internal/usecase"
)

// newRepo sobe um PostgreSQL DE VERDADE num container (testcontainers),
// aplica as migrations e devolve o repositório. Lento (segundos), mas é a
// única forma de provar que o SQL funciona.
func newRepo(t *testing.T) (*postgres.Repository, *pgxpool.Pool) {
	t.Helper()
	if testing.Short() {
		t.Skip("integração: rode sem -short")
	}
	ctx := context.Background()
	ctr, err := tcpostgres.Run(ctx, "postgres:18-alpine",
		tcpostgres.WithDatabase("passarim"), tcpostgres.WithUsername("passarim"), tcpostgres.WithPassword("test"),
		tcpostgres.BasicWaitStrategies())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ctr.Terminate(context.Background()) })
	url, err := ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	if err := postgres.Migrate(url); err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return postgres.NewRepository(pool), pool
}

func sp(sci, common string, biomes []domain.Biome, states ...string) domain.Species {
	size := 20
	return domain.Species{
		ID: domain.SpeciesID(sci), ScientificName: sci, CommonNamePt: common, Family: "Familia",
		SizeCm: &size, ConservationStatus: domain.StatusLC, Biomes: biomes, States: states,
		Description: "Descrição.", DescriptionCredit: domain.Credit{Author: "Equipe Passarim", License: "CC-BY-4.0", Source: "curated", SourceURL: "https://example.org"},
		Facts: []domain.Fact{{Text: "Fato 1", Source: "s1"}, {Text: "Fato 2", Source: "s2"}},
	}
}

func seed(t *testing.T, repo *postgres.Repository, all ...domain.Species) {
	t.Helper()
	for _, s := range all {
		if err := repo.UpsertCurated(context.Background(), s); err != nil {
			t.Fatal(err)
		}
	}
}

func ids(list []domain.SpeciesSummary) []string {
	out := make([]string, len(list))
	for i, s := range list {
		out[i] = s.ID
	}
	return out
}

func TestRepository(t *testing.T) {
	repo, pool := newRepo(t)
	ctx := context.Background()
	seed(t, repo,
		sp("Turdus rufiventris", "Sabiá-laranjeira", []domain.Biome{domain.BiomeMataAtlantica, domain.BiomeCerrado}, "SP", "RJ"),
		sp("Pitangus sulphuratus", "Bem-te-vi", []domain.Biome{domain.BiomeCerrado}, "SP"),
		sp("Ramphastos toco", "Tucano-toco", []domain.Biome{domain.BiomePantanal}, "MS"),
		sp("Fakeus percentus", "Ave 100% teste", nil),
	)

	t.Run("lista ordenada por nome sem acento", func(t *testing.T) {
		got, err := repo.ListSpecies(ctx, usecase.ListFilter{Limit: 10})
		if err != nil {
			t.Fatal(err)
		}
		want := []string{"fakeus-percentus", "pitangus-sulphuratus", "turdus-rufiventris", "ramphastos-toco"}
		if !equal(ids(got), want) {
			t.Fatalf("ordem = %v, want %v", ids(got), want)
		}
	})

	// Review Focus #1
	t.Run("TestListSpecies_SearchIgnoresAccentsAndCase", func(t *testing.T) {
		for _, q := range []string{"sabia", domain.NormalizeForSearch("SABIÁ"), "rufiventris"} {
			got, _ := repo.ListSpecies(ctx, usecase.ListFilter{Query: q, Limit: 10})
			if !equal(ids(got), []string{"turdus-rufiventris"}) {
				t.Errorf("busca %q = %v", q, ids(got))
			}
		}
	})

	// Review Focus #2
	t.Run("TestListSpecies_WildcardsAreLiteral", func(t *testing.T) {
		got, _ := repo.ListSpecies(ctx, usecase.ListFilter{Query: "%", Limit: 10})
		if !equal(ids(got), []string{"fakeus-percentus"}) {
			t.Fatalf("'%%' deveria achar só o nome com %% literal, veio %v", ids(got))
		}
		got, _ = repo.ListSpecies(ctx, usecase.ListFilter{Query: "_", Limit: 10})
		if len(got) != 0 {
			t.Fatalf("'_' deveria ser literal e não achar nada, veio %v", ids(got))
		}
	})

	t.Run("filtros por bioma e estado", func(t *testing.T) {
		got, _ := repo.ListSpecies(ctx, usecase.ListFilter{Biome: domain.BiomeCerrado, State: "SP", Limit: 10})
		if !equal(ids(got), []string{"pitangus-sulphuratus", "turdus-rufiventris"}) {
			t.Fatalf("cerrado+SP = %v", ids(got))
		}
	})

	t.Run("cursor continua depois do último item", func(t *testing.T) {
		after := &usecase.Cursor{SortName: domain.NormalizeForSearch("Bem-te-vi"), ID: "pitangus-sulphuratus"}
		got, _ := repo.ListSpecies(ctx, usecase.ListFilter{After: after, Limit: 10})
		if !equal(ids(got), []string{"turdus-rufiventris", "ramphastos-toco"}) {
			t.Fatalf("depois do bem-te-vi = %v", ids(got))
		}
	})

	t.Run("detalhe completo e ausentes como nil", func(t *testing.T) {
		s, err := repo.GetSpecies(ctx, "turdus-rufiventris")
		if err != nil {
			t.Fatal(err)
		}
		if s.CommonNamePt != "Sabiá-laranjeira" || *s.SizeCm != 20 || s.Diet != nil || s.Audio != nil {
			t.Fatalf("detalhe errado: %+v", s)
		}
		if len(s.Facts) != 2 || s.Facts[0].Text != "Fato 1" || !equal(s.States, []string{"RJ", "SP"}) || len(s.Biomes) != 2 {
			t.Fatalf("listas erradas: %+v", s)
		}
		if s.DescriptionCredit.License != "CC-BY-4.0" {
			t.Fatalf("crédito: %+v", s.DescriptionCredit)
		}
	})

	t.Run("id inexistente", func(t *testing.T) {
		if _, err := repo.GetSpecies(ctx, "nao-existe"); err != domain.ErrNotFound {
			t.Fatalf("esperava ErrNotFound, veio %v", err)
		}
	})

	t.Run("contagens", func(t *testing.T) {
		b, _ := repo.CountByBiome(ctx)
		s, _ := repo.CountByState(ctx)
		if b[domain.BiomeCerrado] != 2 || b[domain.BiomePantanal] != 1 || s["SP"] != 2 || s["MS"] != 1 {
			t.Fatalf("biomas=%v estados=%v", b, s)
		}
	})

	// Review Focus #3
	t.Run("TestUpsertCurated_PreservesWorkerData", func(t *testing.T) {
		_, err := pool.Exec(ctx, `INSERT INTO media (species_id, kind, position, audio_key, duration_ms, author, license, source, source_url)
			VALUES ('turdus-rufiventris', 'audio', 0, 'a.m4a', 12000, 'Autor', 'CC-BY-NC', 'xeno-canto', 'https://xeno-canto.org/1')`)
		if err != nil {
			t.Fatal(err)
		}
		_, err = pool.Exec(ctx, `INSERT INTO occurrence_cluster VALUES ('turdus-rufiventris', -23.5, -46.6, 10, 0.5)`)
		if err != nil {
			t.Fatal(err)
		}
		changed := sp("Turdus rufiventris", "Sabiá-laranjeira", []domain.Biome{domain.BiomeMataAtlantica}, "SP")
		changed.Facts = changed.Facts[:1]
		seed(t, repo, changed, changed) // duas vezes: idempotente

		s, _ := repo.GetSpecies(ctx, "turdus-rufiventris")
		if s.Audio == nil || s.Audio.Key != "a.m4a" || s.Audio.DurationMs != 12000 || len(s.Clusters) != 1 || s.Clusters[0].Precision != 0.5 {
			t.Fatalf("seed apagou dados do worker: audio=%+v clusters=%+v", s.Audio, s.Clusters)
		}
		if len(s.Facts) != 1 || len(s.Biomes) != 1 || !equal(s.States, []string{"SP"}) {
			t.Fatalf("campos curados não foram atualizados: %+v", s)
		}
	})
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
