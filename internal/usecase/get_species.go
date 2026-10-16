// Caso de uso da tela de detalhe.

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

// Execute devolve a espécie pelo id, ou domain.ErrNotFound. Um id vazio
// (mesmo só com espaços) é recusado antes de ir ao banco.
func (u GetSpecies) Execute(ctx context.Context, id string) (domain.Species, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return domain.Species{}, &domain.InvalidArgumentError{Field: "id", Reason: "obrigatório"}
	}
	// Devolvemos o erro do repositório sem embrulhar: não há contexto útil a
	// acrescentar, e quem chama reconhece domain.ErrNotFound com errors.Is.
	return u.Repo.GetSpecies(ctx, id)
}
