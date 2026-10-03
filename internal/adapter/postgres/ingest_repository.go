package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/velosobr/passarim-catalog/internal/adapter/postgres/sqlcgen"
	"github.com/velosobr/passarim-catalog/internal/domain"
	"github.com/velosobr/passarim-catalog/internal/usecase/ingest"
)

// IngestRepository implementa a fila de jobs e a gravação de mídia.
type IngestRepository struct {
	pool *pgxpool.Pool
	q    *sqlcgen.Queries
}

var (
	_ ingest.JobQueue        = (*IngestRepository)(nil)
	_ ingest.MediaRepository = (*IngestRepository)(nil)
)

func NewIngestRepository(pool *pgxpool.Pool) *IngestRepository {
	return &IngestRepository{pool: pool, q: sqlcgen.New(pool)}
}

func (r *IngestRepository) EnqueueMissing(ctx context.Context, sources []ingest.Source, refreshAfter time.Duration) (int, error) {
	names := make([]string, len(sources))
	for i, s := range sources {
		names[i] = string(s)
	}
	if err := r.q.EnqueueMissingJobs(ctx, names); err != nil {
		return 0, fmt.Errorf("enfileirar: %w", err)
	}
	if err := r.q.RescheduleStaleJobs(ctx, refreshAfter.Seconds()); err != nil {
		return 0, fmt.Errorf("reagendar: %w", err)
	}
	if err := r.q.RequeueStuckJobs(ctx); err != nil {
		return 0, fmt.Errorf("devolver jobs presos: %w", err)
	}
	n, err := r.q.CountPendingJobs(ctx)
	return int(n), err
}

func (r *IngestRepository) ClaimDue(ctx context.Context, limit int) ([]ingest.Job, error) {
	rows, err := r.q.ClaimDueJobs(ctx, toInt32(limit))
	if err != nil {
		return nil, err
	}
	jobs := make([]ingest.Job, len(rows))
	for i, row := range rows {
		jobs[i] = ingest.Job{ID: row.ID, SpeciesID: row.SpeciesID, ScientificName: row.ScientificName,
			Source: ingest.Source(row.Source), Attempts: int(row.Attempts)}
	}
	return jobs, nil
}

func (r *IngestRepository) Complete(ctx context.Context, jobID int64) error {
	return r.q.CompleteJob(ctx, jobID)
}

func (r *IngestRepository) Fail(ctx context.Context, jobID int64, cause string, retryIn time.Duration, giveUp bool) error {
	if len(cause) > 500 { // a mensagem vai para o banco: limitamos o tamanho
		cause = cause[:500]
	}
	return r.q.FailJob(ctx, sqlcgen.FailJobParams{ID: jobID, GiveUp: giveUp, LastError: cause, RetryInSecs: retryIn.Seconds()})
}

// replaceMedia apaga a mídia de um tipo e grava a nova, numa transação.
func (r *IngestRepository) replaceMedia(ctx context.Context, speciesID, kind string, rows []sqlcgen.InsertMediaParams) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // após Commit, Rollback não faz nada
	q := r.q.WithTx(tx)
	if err := q.DeleteMediaByKind(ctx, sqlcgen.DeleteMediaByKindParams{SpeciesID: speciesID, Kind: kind}); err != nil {
		return err
	}
	for _, row := range rows {
		if err := q.InsertMedia(ctx, row); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (r *IngestRepository) ReplacePhotos(ctx context.Context, speciesID string, photos []domain.Photo) error {
	rows := make([]sqlcgen.InsertMediaParams, len(photos))
	for i, p := range photos {
		rows[i] = sqlcgen.InsertMediaParams{SpeciesID: speciesID, Kind: "photo", Position: toInt32(i),
			ThumbKey: p.ThumbKey, MediumKey: p.MediumKey, LargeKey: p.LargeKey,
			Width: toInt32(p.Width), Height: toInt32(p.Height),
			Author: p.Credit.Author, License: p.Credit.License, Source: p.Credit.Source, SourceUrl: p.Credit.SourceURL}
	}
	return r.replaceMedia(ctx, speciesID, "photo", rows)
}

func (r *IngestRepository) ReplaceAudio(ctx context.Context, speciesID string, a *domain.Audio) error {
	var rows []sqlcgen.InsertMediaParams
	if a != nil {
		rows = append(rows, sqlcgen.InsertMediaParams{SpeciesID: speciesID, Kind: "audio", Position: 0,
			AudioKey: a.Key, DurationMs: toInt32(a.DurationMs),
			Author: a.Credit.Author, License: a.Credit.License, Source: a.Credit.Source, SourceUrl: a.Credit.SourceURL})
	}
	return r.replaceMedia(ctx, speciesID, "audio", rows)
}

func (r *IngestRepository) ReplaceClusters(ctx context.Context, speciesID string, clusters []domain.OccurrenceCluster) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // após Commit, Rollback não faz nada
	q := r.q.WithTx(tx)
	if err := q.DeleteClusters(ctx, speciesID); err != nil {
		return err
	}
	for _, c := range clusters {
		if err := q.InsertCluster(ctx, sqlcgen.InsertClusterParams{SpeciesID: speciesID, Lat: c.Lat, Lng: c.Lng,
			Count: toInt32(c.Count), Precision: c.Precision}); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
