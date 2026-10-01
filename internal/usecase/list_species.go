package usecase

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/velosobr/passarim-catalog/internal/domain"
)

// Limites da listagem (proteção contra abuso — OWASP API4).
const (
	DefaultPageSize = 20
	MaxPageSize     = 50
	MaxQueryLen     = 100 // em caracteres, não bytes
)

// ListSpeciesInput é o pedido "cru", como veio de fora.
type ListSpeciesInput struct {
	Query     string
	Biome     string
	State     string
	PageSize  int
	PageToken string
}

// ListSpeciesOutput traz a página e o token para pedir a próxima ("" = acabou).
type ListSpeciesOutput struct {
	Species       []domain.SpeciesSummary
	NextPageToken string
}

// ListSpecies lista espécies com busca, filtros e paginação.
type ListSpecies struct {
	Repo SpeciesRepository
}

func (u ListSpecies) Execute(ctx context.Context, in ListSpeciesInput) (ListSpeciesOutput, error) {
	filter, pageSize, err := buildFilter(in)
	if err != nil {
		return ListSpeciesOutput{}, err
	}

	// Truque clássico: pedimos UM item a mais do que a página. Se ele vier,
	// sabemos que existe próxima página sem fazer uma segunda consulta (COUNT).
	filter.Limit = pageSize + 1
	items, err := u.Repo.ListSpecies(ctx, filter)
	if err != nil {
		return ListSpeciesOutput{}, fmt.Errorf("listar espécies: %w", err)
	}

	out := ListSpeciesOutput{Species: items}
	if len(items) > pageSize {
		out.Species = items[:pageSize]
		last := out.Species[pageSize-1]
		out.NextPageToken = EncodeCursor(Cursor{SortName: domain.NormalizeForSearch(last.CommonNamePt), ID: last.ID})
	}
	return out, nil
}

// buildFilter valida e normaliza a entrada. Fica separado para Execute ler como uma história.
func buildFilter(in ListSpeciesInput) (ListFilter, int, error) {
	var f ListFilter

	query := strings.TrimSpace(in.Query)
	if utf8.RuneCountInString(query) > MaxQueryLen {
		return f, 0, &domain.InvalidArgumentError{Field: "query", Reason: fmt.Sprintf("máximo de %d caracteres", MaxQueryLen)}
	}
	f.Query = domain.NormalizeForSearch(query)

	if in.Biome != "" {
		b, err := domain.ParseBiome(in.Biome)
		if err != nil {
			return f, 0, err
		}
		f.Biome = b
	}
	if in.State != "" {
		uf, err := domain.NormalizeUF(in.State)
		if err != nil {
			return f, 0, err
		}
		f.State = uf
	}

	pageSize := in.PageSize
	switch {
	case pageSize < 0:
		return f, 0, &domain.InvalidArgumentError{Field: "page_size", Reason: "não pode ser negativo"}
	case pageSize == 0:
		pageSize = DefaultPageSize
	case pageSize > MaxPageSize:
		pageSize = MaxPageSize
	}

	if in.PageToken != "" {
		c, err := DecodeCursor(in.PageToken)
		if err != nil {
			return f, 0, err
		}
		f.After = &c
	}
	return f, pageSize, nil
}
