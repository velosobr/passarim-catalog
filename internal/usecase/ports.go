// Package usecase contém os casos de uso: o que o catálogo SABE FAZER.
// Ele depende só do domain e de interfaces ("ports"). Quem implementa as
// interfaces (PostgreSQL, por exemplo) fica na camada adapter. Assim dá
// para trocar o banco sem tocar numa linha daqui.
package usecase

import (
	"context"

	"github.com/velosobr/passarim-catalog/internal/domain"
)

// SpeciesRepository é a "porta" de saída para onde as espécies estão guardadas.
type SpeciesRepository interface {
	// ListSpecies devolve até filter.Limit espécies, na ordem (SortName, ID),
	// começando DEPOIS de filter.After (se informado).
	ListSpecies(ctx context.Context, filter ListFilter) ([]domain.SpeciesSummary, error)
	// GetSpecies devolve domain.ErrNotFound se o id não existir.
	GetSpecies(ctx context.Context, id string) (domain.Species, error)
	CountByBiome(ctx context.Context) (map[domain.Biome]int, error)
	CountByState(ctx context.Context) (map[string]int, error)
}

// ListFilter chega ao repositório já validado e normalizado.
type ListFilter struct {
	Query string       // sem acentos e minúscula; vazio = sem busca
	Biome domain.Biome // vazio = sem filtro
	State string       // UF maiúscula; vazio = sem filtro
	After *Cursor      // nil = primeira página
	Limit int          // quantos itens buscar
}
