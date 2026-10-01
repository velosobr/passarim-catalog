package usecase_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/velosobr/passarim-catalog/internal/domain"
	"github.com/velosobr/passarim-catalog/internal/usecase"
)

func TestListSpecies_DefaultAndMaxPageSize(t *testing.T) {
	repo := &fakeRepo{species: birds(80)}
	uc := usecase.ListSpecies{Repo: repo}

	out, err := uc.Execute(context.Background(), usecase.ListSpeciesInput{})
	if err != nil || len(out.Species) != usecase.DefaultPageSize {
		t.Fatalf("padrão: len=%d err=%v", len(out.Species), err)
	}
	out, _ = uc.Execute(context.Background(), usecase.ListSpeciesInput{PageSize: 500})
	if len(out.Species) != usecase.MaxPageSize {
		t.Fatalf("máximo: len=%d, want %d", len(out.Species), usecase.MaxPageSize)
	}
}

func TestListSpecies_PaginatesWithoutGapsOrRepeats(t *testing.T) {
	repo := &fakeRepo{species: birds(45)}
	uc := usecase.ListSpecies{Repo: repo}
	seen := map[string]bool{}
	token := ""
	for page := 0; page < 10; page++ {
		out, err := uc.Execute(context.Background(), usecase.ListSpeciesInput{PageSize: 20, PageToken: token})
		if err != nil {
			t.Fatal(err)
		}
		for _, s := range out.Species {
			if seen[s.ID] {
				t.Fatalf("espécie repetida entre páginas: %s", s.ID)
			}
			seen[s.ID] = true
		}
		if out.NextPageToken == "" {
			break
		}
		token = out.NextPageToken
	}
	if len(seen) != 45 {
		t.Fatalf("viu %d espécies, want 45", len(seen))
	}
}

func TestListSpecies_LastPageHasNoToken(t *testing.T) {
	uc := usecase.ListSpecies{Repo: &fakeRepo{species: birds(20)}}
	out, _ := uc.Execute(context.Background(), usecase.ListSpeciesInput{PageSize: 20})
	if out.NextPageToken != "" {
		t.Fatal("exatamente 20 itens e page_size 20: não deveria haver próxima página")
	}
}

func TestListSpecies_NormalizesQueryAndFilters(t *testing.T) {
	repo := &fakeRepo{}
	uc := usecase.ListSpecies{Repo: repo}
	_, err := uc.Execute(context.Background(), usecase.ListSpeciesInput{Query: "  SABIÁ ", Biome: "cerrado", State: "go"})
	if err != nil {
		t.Fatal(err)
	}
	q := repo.lastQuery
	if q.Query != "sabia" || q.Biome != domain.BiomeCerrado || q.State != "GO" || q.Limit != usecase.DefaultPageSize+1 {
		t.Fatalf("filtro repassado errado: %+v", q)
	}
}

// Review Focus #4: o limite é em CARACTERES (runas), não em bytes.
// "á" ocupa 2 bytes em UTF-8; 100 deles = 200 bytes, mas são 100 caracteres.
func TestListSpecies_QueryLengthCountsRunes(t *testing.T) {
	uc := usecase.ListSpecies{Repo: &fakeRepo{}}
	if _, err := uc.Execute(context.Background(), usecase.ListSpeciesInput{Query: strings.Repeat("á", 100)}); err != nil {
		t.Fatalf("100 caracteres deveria ser aceito: %v", err)
	}
	_, err := uc.Execute(context.Background(), usecase.ListSpeciesInput{Query: strings.Repeat("🐦", 101)})
	assertInvalid(t, err, "query")
}

func TestListSpecies_InvalidInputs(t *testing.T) {
	uc := usecase.ListSpecies{Repo: &fakeRepo{}}
	cases := map[string]usecase.ListSpeciesInput{
		"biome":      {Biome: "deserto"},
		"state":      {State: "XX"},
		"page_size":  {PageSize: -1},
		"page_token": {PageToken: "lixo!"},
	}
	for field, in := range cases {
		_, err := uc.Execute(context.Background(), in)
		assertInvalid(t, err, field)
	}
}

// Review Focus #2 da revisão final: o cursor precisa encodar o sort_name
// DEVOLVIDO pelo repositório (o que o banco realmente gravou e comparou),
// nunca um recálculo de domain.NormalizeForSearch(CommonNamePt) feito aqui.
func TestListSpecies_CursorUsesRepositorySortNameNotRecomputed(t *testing.T) {
	repo := &fakeRepo{
		species:          birds(2),
		sortNameOverride: map[string]string{"genus-saa": "zzz-valor-gravado-diferente"},
	}
	uc := usecase.ListSpecies{Repo: repo}
	out, err := uc.Execute(context.Background(), usecase.ListSpeciesInput{PageSize: 1})
	if err != nil {
		t.Fatal(err)
	}
	if out.NextPageToken == "" {
		t.Fatal("esperava próxima página (repo tem 1 item a mais que a página pedida só pra garantir token): ajuste o teste")
	}
	c, err := usecase.DecodeCursor(out.NextPageToken)
	if err != nil {
		t.Fatal(err)
	}
	if c.SortName != "zzz-valor-gravado-diferente" {
		t.Fatalf("cursor.SortName = %q, deveria ser o valor devolvido pelo repo (não recalculado)", c.SortName)
	}
}

func TestListSpecies_RepositoryErrorPropagates(t *testing.T) {
	boom := errors.New("banco caiu")
	_, err := usecase.ListSpecies{Repo: &fakeRepo{err: boom}}.Execute(context.Background(), usecase.ListSpeciesInput{})
	if !errors.Is(err, boom) {
		t.Fatalf("esperava o erro do repositório, veio %v", err)
	}
}

func assertInvalid(t *testing.T, err error, field string) {
	t.Helper()
	var inv *domain.InvalidArgumentError
	if !errors.As(err, &inv) || inv.Field != field {
		t.Fatalf("esperava InvalidArgumentError(%s), veio %v", field, err)
	}
}
