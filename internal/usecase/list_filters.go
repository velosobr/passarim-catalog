package usecase

import (
	"context"
	"fmt"
	"sort"

	"github.com/velosobr/passarim-catalog/internal/domain"
)

type BiomeCount struct {
	Biome domain.Biome
	Count int
}

type StateCount struct {
	State string
	Count int
}

// Filters lista só biomas/estados que têm pelo menos uma ave.
type Filters struct {
	Biomes []BiomeCount
	States []StateCount
}

// ListFilters monta as opções de filtro da tela Explorar.
type ListFilters struct {
	Repo SpeciesRepository
}

func (u ListFilters) Execute(ctx context.Context) (Filters, error) {
	byBiome, err := u.Repo.CountByBiome(ctx)
	if err != nil {
		return Filters{}, fmt.Errorf("contar por bioma: %w", err)
	}
	byState, err := u.Repo.CountByState(ctx)
	if err != nil {
		return Filters{}, fmt.Errorf("contar por estado: %w", err)
	}

	var out Filters
	// Biomas em ordem "oficial" (fixa), não alfabética nem aleatória:
	// mapas em Go não têm ordem garantida, então nunca iteramos o mapa direto.
	for _, b := range domain.AllBiomes() {
		if n := byBiome[b]; n > 0 {
			out.Biomes = append(out.Biomes, BiomeCount{Biome: b, Count: n})
		}
	}
	for uf, n := range byState {
		if n > 0 {
			out.States = append(out.States, StateCount{State: uf, Count: n})
		}
	}
	sort.Slice(out.States, func(i, j int) bool { return out.States[i].State < out.States[j].State })
	return out, nil
}
