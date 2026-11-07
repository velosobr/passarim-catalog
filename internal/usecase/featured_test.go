// Testes do caso de uso da "Ave do dia", com um repositório falso em
// memória e um relógio fixo.

package usecase_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/velosobr/passarim-catalog/internal/domain"
	"github.com/velosobr/passarim-catalog/internal/usecase"
)

// fakeFeatured imita as tabelas: as linhas gravadas, as elegíveis e as espécies.
type fakeFeatured struct {
	rows     []domain.FeaturedDay
	eligible []string
	species  map[string]domain.Species
	saves    int
}

func (f *fakeFeatured) FeaturedOn(_ context.Context, day string) (string, bool, error) {
	for _, r := range f.rows {
		if r.Day == day {
			return r.SpeciesID, true, nil
		}
	}
	return "", false, nil
}

func (f *fakeFeatured) FeaturedCycle(_ context.Context, _ string) ([]domain.FeaturedDay, error) {
	return f.rows, nil // o domínio filtra pelo ciclo; o fake devolve tudo
}

func (f *fakeFeatured) FeaturedEligible(context.Context) ([]string, error) { return f.eligible, nil }

// SaveFeatured imita o ON CONFLICT DO NOTHING: um dia já gravado fica como está.
func (f *fakeFeatured) SaveFeatured(_ context.Context, d domain.FeaturedDay) error {
	f.saves++
	for _, r := range f.rows {
		if r.Day == d.Day {
			return nil
		}
	}
	f.rows = append(f.rows, d)
	return nil
}

func (f *fakeFeatured) GetSpecies(_ context.Context, id string) (domain.Species, error) {
	if s, ok := f.species[id]; ok {
		return s, nil
	}
	return domain.Species{}, domain.ErrNotFound
}

// saoPaulo é o fuso da "Ave do dia".
var saoPaulo, _ = time.LoadLocation("America/Sao_Paulo")

// at devolve um relógio parado no instante dado, no fuso de São Paulo.
func at(year int, month time.Month, day, hour, minute, sec int) func() time.Time {
	return func() time.Time { return time.Date(year, month, day, hour, minute, sec, 0, saoPaulo) }
}

func newFeatured() *fakeFeatured {
	sabia := domain.Species{ID: "turdus-rufiventris", CommonNamePt: "Sabiá", PrimaryBiome: domain.BiomeMataAtlantica,
		Photos: []domain.Photo{{ThumbKey: "t.webp"}}, Audio: &domain.Audio{Key: "a.aac", Peaks: []float32{1}}}
	anu := domain.Species{ID: "crotophaga-ani", CommonNamePt: "Anu-preto", Photos: []domain.Photo{{ThumbKey: "anu.webp"}}}
	return &fakeFeatured{eligible: []string{"turdus-rufiventris", "crotophaga-ani"},
		species: map[string]domain.Species{sabia.ID: sabia, anu.ID: anu}}
}

// A primeira chamada do dia escolhe e grava; as outras leem a mesma ave,
// mesmo depois de ela deixar de ser elegível.
func TestGetFeatured_StableDuringTheDay(t *testing.T) {
	repo := newFeatured()
	uc := usecase.GetFeatured{Repo: repo, Now: at(2026, 11, 5, 9, 0, 0)}
	first, err := uc.Execute(context.Background(), "2026-11-05")
	if err != nil {
		t.Fatal(err)
	}
	repo.eligible = []string{"outra"}
	again, err := uc.Execute(context.Background(), "2026-11-05")
	if err != nil || again.Species.ID != first.Species.ID || repo.saves != 1 {
		t.Fatalf("primeira=%s depois=%s err=%v gravações=%d", first.Species.ID, again.Species.ID, err, repo.saves)
	}
}

// A resposta traz o resumo (com a miniatura da foto principal e o bioma
// principal) e o canto com os picos.
func TestGetFeatured_ResponseShape(t *testing.T) {
	repo := newFeatured()
	repo.eligible = []string{"turdus-rufiventris"}
	out, err := usecase.GetFeatured{Repo: repo, Now: at(2026, 11, 5, 9, 0, 0)}.Execute(context.Background(), "2026-11-05")
	if err != nil {
		t.Fatal(err)
	}
	if out.Species.ThumbnailKey != "t.webp" || out.Species.PrimaryBiome != domain.BiomeMataAtlantica || out.Audio == nil || out.Audio.Peaks[0] != 1 {
		t.Fatalf("saída: %+v audio=%+v", out.Species, out.Audio)
	}
}

// Às 23h59 em São Paulo o catalog aceita o dia seguinte (o relógio do BFF
// pode já ter virado); dois dias à frente, não.
func TestGetFeatured_AcceptsTomorrowNearMidnight(t *testing.T) {
	uc := usecase.GetFeatured{Repo: newFeatured(), Now: at(2026, 11, 5, 23, 59, 59)}
	if _, err := uc.Execute(context.Background(), "2026-11-06"); err != nil {
		t.Fatalf("amanhã deveria passar: %v", err)
	}
	if _, err := uc.Execute(context.Background(), "2026-11-04"); err != nil {
		t.Fatalf("ontem deveria passar: %v", err)
	}
	var inv *domain.InvalidArgumentError
	if _, err := uc.Execute(context.Background(), "2026-11-07"); !errors.As(err, &inv) || inv.Field != "date" {
		t.Fatalf("dois dias à frente deveria ser inválido, veio %v", err)
	}
}

// O "hoje" é o de São Paulo, não o de UTC: às 22h de São Paulo já é o dia
// seguinte em UTC, mas o dia seguinte +1 continua fora.
func TestGetFeatured_UsesSaoPauloDay(t *testing.T) {
	utcNextDay := func() time.Time { return time.Date(2026, 11, 6, 1, 0, 0, 0, time.UTC) } // 22h do dia 5 em São Paulo
	uc := usecase.GetFeatured{Repo: newFeatured(), Now: utcNextDay}
	if _, err := uc.Execute(context.Background(), "2026-11-04"); err != nil {
		t.Fatalf("ontem em São Paulo (dia 4) deveria passar: %v", err)
	}
	var inv *domain.InvalidArgumentError
	if _, err := uc.Execute(context.Background(), "2026-11-07"); !errors.As(err, &inv) {
		t.Fatalf("dia 7 deveria ser inválido em São Paulo, veio %v", err)
	}
}

func TestGetFeatured_BadFormatIsInvalid(t *testing.T) {
	uc := usecase.GetFeatured{Repo: newFeatured(), Now: at(2026, 11, 5, 9, 0, 0)}
	for _, date := range []string{"", "05/11/2026", "2026-11-5", "2026-13-01"} {
		var inv *domain.InvalidArgumentError
		if _, err := uc.Execute(context.Background(), date); !errors.As(err, &inv) {
			t.Fatalf("%q deveria ser inválido, veio %v", date, err)
		}
	}
}

func TestGetFeatured_NoEligibleIsNotFound(t *testing.T) {
	repo := newFeatured()
	repo.eligible = nil
	_, err := usecase.GetFeatured{Repo: repo, Now: at(2026, 11, 5, 9, 0, 0)}.Execute(context.Background(), "2026-11-05")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("esperava ErrNotFound, veio %v", err)
	}
}
