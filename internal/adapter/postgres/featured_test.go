// Testes de integração da "Ave do dia" contra um PostgreSQL de verdade: a
// consulta das elegíveis, a tabela featured_day e duas chamadas ao mesmo
// tempo.

package postgres_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/velosobr/passarim-catalog/internal/domain"
	"github.com/velosobr/passarim-catalog/internal/usecase"
)

// addMedia grava a foto principal (com a categoria dada; "" = NULL, como as
// fotos de antes da 0004) e, se audio, um canto.
func addMedia(t *testing.T, pool *pgxpool.Pool, id, category, sourcePhotoID string, audio bool) {
	t.Helper()
	ctx := context.Background()
	_, err := pool.Exec(ctx, `INSERT INTO media (species_id, kind, position, thumb_key, author, license, source, source_url, category, source_photo_id)
		VALUES ($1, 'photo', 0, 't.webp', 'A', 'CC-BY', 'inaturalist', 'https://x', NULLIF($2, ''), NULLIF($3, ''))`, id, category, sourcePhotoID)
	if err != nil {
		t.Fatal(err)
	}
	if audio {
		_, err = pool.Exec(ctx, `INSERT INTO media (species_id, kind, position, audio_key, author, license, source, source_url)
			VALUES ($1, 'audio', 0, 'a.aac', 'A', 'CC-BY', 'xeno-canto', 'https://x')`, id)
		if err != nil {
			t.Fatal(err)
		}
	}
}

// Elegível = conteúdo Kids + foto principal de ave sozinha (ou do override,
// com qualquer categoria) + canto. Fotos sem categoria (antes da 0004), sem
// canto ou sem Kids ficam de fora.
func TestFeaturedEligible(t *testing.T) {
	repo, pool := newRepo(t)
	ctx := context.Background()
	b := []domain.Biome{domain.BiomeCerrado}
	seed(t, repo,
		withKids(sp("Turdus rufiventris", "Sabiá", b)), // elegível
		withKids(sp("Crotophaga ani", "Anu", b)),       // bando, mas override aponta a foto: elegível
		withKids(sp("Pitangus sulphuratus", "Bem", b)), // sem canto
		sp("Furnarius rufus", "João", b),               // sem Kids
		withKids(sp("Cariama cristata", "Seriema", b)), // foto antiga, sem categoria
		withKids(sp("Ramphastos toco", "Tucano", b)))   // sem foto nenhuma
	addMedia(t, pool, "turdus-rufiventris", "single_bird", "1", true)
	addMedia(t, pool, "crotophaga-ani", "flock", "77", true)
	_, _ = pool.Exec(ctx, `INSERT INTO photo_override (species_id, primary_observation_id, primary_photo_id) VALUES ('crotophaga-ani', '5', '77')`)
	addMedia(t, pool, "pitangus-sulphuratus", "single_bird", "2", false)
	addMedia(t, pool, "furnarius-rufus", "single_bird", "3", true)
	addMedia(t, pool, "cariama-cristata", "", "", true)
	got, err := repo.FeaturedEligible(ctx)
	if err != nil || fmt.Sprint(got) != "[crotophaga-ani turdus-rufiventris]" {
		t.Fatalf("elegíveis: %v %v", got, err)
	}
}

// Gravar o mesmo dia duas vezes mantém a primeira; o ciclo devolve as
// linhas do ciclo da linha mais recente antes do dia pedido.
func TestFeaturedDayTable(t *testing.T) {
	repo, _ := newRepo(t)
	ctx := context.Background()
	b := []domain.Biome{domain.BiomeCerrado}
	seed(t, repo, sp("Turdus rufiventris", "Sabiá", b), sp("Crotophaga ani", "Anu", b))
	save := func(day, id string, cycle int) {
		if err := repo.SaveFeatured(ctx, domain.FeaturedDay{Day: day, SpeciesID: id, Cycle: cycle}); err != nil {
			t.Fatal(err)
		}
	}
	save("2026-11-01", "turdus-rufiventris", 1)
	save("2026-11-02", "crotophaga-ani", 2)
	save("2026-11-02", "turdus-rufiventris", 2) // conflito: fica a primeira
	if id, found, _ := repo.FeaturedOn(ctx, "2026-11-02"); !found || id != "crotophaga-ani" {
		t.Fatalf("dia 2: %s %v", id, found)
	}
	if _, found, _ := repo.FeaturedOn(ctx, "2026-11-03"); found {
		t.Fatal("dia 3 não deveria existir")
	}
	rows, err := repo.FeaturedCycle(ctx, "2026-11-03")
	if err != nil || len(rows) != 1 || rows[0].Day != "2026-11-02" || rows[0].Cycle != 2 {
		t.Fatalf("ciclo antes do dia 3: %+v %v", rows, err)
	}
	if rows, _ := repo.FeaturedCycle(ctx, "2026-11-01"); len(rows) != 0 {
		t.Fatalf("antes do primeiro dia não há ciclo: %+v", rows)
	}
}

// Duas chamadas ao mesmo tempo no mesmo dia ficam com a mesma ave e uma linha só.
func TestGetFeatured_ConcurrentCalls(t *testing.T) {
	repo, pool := newRepo(t)
	ctx := context.Background()
	b := []domain.Biome{domain.BiomeCerrado}
	for _, s := range []domain.Species{withKids(sp("Turdus rufiventris", "Sabiá", b)), withKids(sp("Crotophaga ani", "Anu", b)),
		withKids(sp("Pitangus sulphuratus", "Bem", b))} {
		seed(t, repo, s)
		addMedia(t, pool, s.ID, "single_bird", s.ID, true)
	}
	uc := usecase.GetFeatured{Repo: repo, Now: func() time.Time { return time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC) }}
	var wg sync.WaitGroup
	got := make([]string, 8)
	for i := range got {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out, err := uc.Execute(ctx, "2026-11-05")
			if err != nil {
				t.Error(err)
				return
			}
			got[i] = out.Species.ID
		}()
	}
	wg.Wait()
	for _, id := range got {
		if id != got[0] {
			t.Fatalf("aves diferentes no mesmo dia: %v", got)
		}
	}
	var n int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM featured_day`).Scan(&n)
	if n != 1 {
		t.Fatalf("esperava 1 linha, veio %d", n)
	}
}
