// Package usecase contém os casos de uso: o que o catálogo SABE FAZER.
// Ele depende só do domain e de interfaces ("ports"). Quem implementa as
// interfaces (PostgreSQL, por exemplo) fica na camada adapter. Assim dá
// para trocar o banco sem tocar numa linha daqui.
//
// Este arquivo define a porta do repositório e o filtro que ela recebe. Em
// Go, uma interface é satisfeita sem declaração: o repositório Postgres
// implementa SpeciesRepository só por ter estes métodos, e os testes usam
// um repositório em memória (fake_repo_test.go) do mesmo jeito.
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
	// CountByBiome e CountByState contam as espécies de cada bioma e de cada
	// UF; CountByPrimaryBiome conta por bioma principal ("mundo" do Kids).
	// Com kidsOnly, só entram as espécies com conteúdo Kids.
	CountByBiome(ctx context.Context, kidsOnly bool) (map[domain.Biome]int, error)
	CountByState(ctx context.Context, kidsOnly bool) (map[string]int, error)
	CountByPrimaryBiome(ctx context.Context, kidsOnly bool) (map[domain.Biome]int, error)
}

// ListFilter chega ao repositório já validado e normalizado.
type ListFilter struct {
	Query        string       // sem acentos e minúscula; vazio = sem busca
	Biome        domain.Biome // vazio = sem filtro
	State        string       // UF maiúscula; vazio = sem filtro
	KidsOnly     bool         // true = só espécies com conteúdo Kids
	PrimaryBiome domain.Biome // vazio = sem filtro pelo bioma principal
	After        *Cursor      // nil = primeira página
	Limit        int          // quantos itens buscar
}

// FeaturedRepository é a porta da "Ave do dia": a tabela featured_day, a
// lista das elegíveis e a leitura da espécie escolhida.
type FeaturedRepository interface {
	// FeaturedOn devolve a ave já gravada para o dia (found=false se não há).
	FeaturedOn(ctx context.Context, day string) (speciesID string, found bool, err error)
	// FeaturedCycle devolve as linhas do ciclo da linha mais recente antes do
	// dia, mais as dos dias depois dele (amanhã pedido antes de hoje).
	FeaturedCycle(ctx context.Context, day string) ([]domain.FeaturedDay, error)
	// FeaturedEligible devolve os ids das espécies que podem ser escolhidas:
	// com conteúdo Kids, foto principal de ave sozinha (ou do override) e canto.
	FeaturedEligible(ctx context.Context) ([]string, error)
	// SaveFeatured grava a escolha; um dia já gravado fica como está.
	SaveFeatured(ctx context.Context, d domain.FeaturedDay) error
	// GetSpecies devolve domain.ErrNotFound se o id não existir.
	GetSpecies(ctx context.Context, id string) (domain.Species, error)
}
