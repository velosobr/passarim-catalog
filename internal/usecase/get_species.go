package usecase

import (
	"context"
	"strings"

	"github.com/velosobr/passarim-catalog/internal/domain"
)

// GetSpecies busca todos os dados de uma espécie.
type GetSpecies struct {
	Repo SpeciesRepository
}

func (u GetSpecies) Execute(ctx context.Context, id string) (domain.Species, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return domain.Species{}, &domain.InvalidArgumentError{Field: "id", Reason: "obrigatório"}
	}
	// Não "embrulhamos" o erro aqui de propósito: quem chama precisa
	// reconhecer domain.ErrNotFound com errors.Is (e reconhece mesmo assim).
	return u.Repo.GetSpecies(ctx, id)
}
