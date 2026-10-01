package usecase_test

import (
	"context"
	"testing"

	"github.com/velosobr/passarim-catalog/internal/domain"
	"github.com/velosobr/passarim-catalog/internal/usecase"
)

func TestListFilters_OrdersBiomesCanonicallyAndStatesAlphabetically(t *testing.T) {
	out, err := usecase.ListFilters{Repo: &fakeRepo{}}.Execute(context.Background())
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
