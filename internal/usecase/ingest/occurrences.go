// Caso de uso dos avistamentos (GBIF): busca e agrupa em grade para o mapa.

package ingest

import (
	"context"
	"fmt"

	"github.com/velosobr/passarim-catalog/internal/domain"
)

// IngestOccurrences busca avistamentos e grava um resumo agrupado em grade.
type IngestOccurrences struct {
	Source    OccurrenceSource
	Repo      MediaRepository
	MaxPoints int
	CellDeg   float64
}

// Run busca até MaxPoints avistamentos (900 no worker), agrupa em células de
// CellDeg graus (1° no worker) e substitui os grupos da espécie.
func (u IngestOccurrences) Run(ctx context.Context, job Job) error {
	points, err := u.Source.FindOccurrences(ctx, job.ScientificName, u.MaxPoints)
	if err != nil {
		return fmt.Errorf("buscar ocorrências: %w", err)
	}
	// Zero pontos grava lista vazia: a espécie realmente não tem avistamentos.
	return u.Repo.ReplaceClusters(ctx, job.SpeciesID, domain.ClusterPoints(points, u.CellDeg))
}
