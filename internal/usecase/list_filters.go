// Caso de uso dos filtros: biomas e UFs com a quantidade de espécies.

package usecase

import (
	"context"
	"fmt"
	"sort"

	"github.com/velosobr/passarim-catalog/internal/domain"
)

// BiomeCount e StateCount são uma opção de filtro com a contagem.
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
	Biomes        []BiomeCount
	States        []StateCount
	PrimaryBiomes []BiomeCount // por bioma principal, na ordem oficial
}

// ListFiltersInput é o pedido dos filtros. KidsOnly = contar só as espécies
// com conteúdo Kids (a jornada Kids).
type ListFiltersInput struct {
	KidsOnly bool
}

// ListFilters monta as opções de filtro da tela Explorar.
type ListFilters struct {
	Repo SpeciesRepository
}

// Execute busca as três contagens, tira as zeradas e ordena: biomas na
// ordem oficial, UFs em ordem alfabética. Os erros ganham contexto com %w,
// que embrulha o erro original sem perdê-lo.
func (u ListFilters) Execute(ctx context.Context, in ListFiltersInput) (Filters, error) {
	byBiome, err := u.Repo.CountByBiome(ctx, in.KidsOnly)
	if err != nil {
		return Filters{}, fmt.Errorf("contar por bioma: %w", err)
	}
	byState, err := u.Repo.CountByState(ctx, in.KidsOnly)
	if err != nil {
		return Filters{}, fmt.Errorf("contar por estado: %w", err)
	}
	byPrimary, err := u.Repo.CountByPrimaryBiome(ctx, in.KidsOnly)
	if err != nil {
		return Filters{}, fmt.Errorf("contar por bioma principal: %w", err)
	}

	out := Filters{Biomes: inOfficialOrder(byBiome), PrimaryBiomes: inOfficialOrder(byPrimary)}
	for uf, n := range byState {
		if n > 0 {
			out.States = append(out.States, StateCount{State: uf, Count: n})
		}
	}
	sort.Slice(out.States, func(i, j int) bool { return out.States[i].State < out.States[j].State })
	return out, nil
}

// inOfficialOrder devolve os biomas com contagem maior que zero na ordem
// "oficial" (fixa), não alfabética nem aleatória: mapas em Go não têm ordem
// garantida, então nunca iteramos o mapa direto.
func inOfficialOrder(counts map[domain.Biome]int) []BiomeCount {
	var out []BiomeCount
	for _, b := range domain.AllBiomes() {
		if n := counts[b]; n > 0 {
			out = append(out, BiomeCount{Biome: b, Count: n})
		}
	}
	return out
}
