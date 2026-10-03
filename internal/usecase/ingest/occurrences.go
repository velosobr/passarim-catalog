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

func (u IngestOccurrences) Run(ctx context.Context, job Job) error {
	points, err := u.Source.FindOccurrences(ctx, job.ScientificName, u.MaxPoints)
	if err != nil {
		return fmt.Errorf("buscar ocorrências: %w", err)
	}
	// Zero pontos grava lista vazia: a espécie realmente não tem avistamentos.
	return u.Repo.ReplaceClusters(ctx, job.SpeciesID, domain.ClusterPoints(points, u.CellDeg))
}
