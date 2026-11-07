// Testes do caso de uso de filtros.

package usecase_test

import (
	"context"
	"testing"

	"github.com/velosobr/passarim-catalog/internal/domain"
	"github.com/velosobr/passarim-catalog/internal/usecase"
)

// Biomas saem na ordem oficial e UFs em ordem alfabética, sem as zeradas.
func TestListFilters_OrdersBiomesCanonicallyAndStatesAlphabetically(t *testing.T) {
	out, err := usecase.ListFilters{Repo: &fakeRepo{}}.Execute(context.Background(), usecase.ListFiltersInput{})
	if err != nil {
		t.Fatal(err)
	}
	// Ordem dos biomas segue domain.AllBiomes (amazonia antes de pantanal).
	if len(out.Biomes) != 2 || out.Biomes[0].Biome != domain.BiomeAmazonia || out.Biomes[1].Count != 2 {
		t.Fatalf("biomas: %+v", out.Biomes)
	}
	if len(out.States) != 2 || out.States[0].State != "AM" || out.States[1].Count != 3 {
		t.Fatalf("estados: %+v", out.States)
	}
}

// As contagens por bioma principal saem na ordem oficial, sem as zeradas, e
// o kidsOnly chega ao repositório.
func TestListFilters_PrimaryBiomesAndKidsOnly(t *testing.T) {
	repo := &fakeRepo{}
	out, err := usecase.ListFilters{Repo: repo}.Execute(context.Background(), usecase.ListFiltersInput{KidsOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if !repo.lastKidsOnly {
		t.Fatal("kidsOnly não chegou ao repositório")
	}
	if len(out.PrimaryBiomes) != 2 || out.PrimaryBiomes[0].Biome != domain.BiomeCerrado || out.PrimaryBiomes[1].Biome != domain.BiomePampa {
		t.Fatalf("biomas principais: %+v", out.PrimaryBiomes)
	}
}
