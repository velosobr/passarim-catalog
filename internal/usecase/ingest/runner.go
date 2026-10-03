package ingest

import (
	"context"
	"fmt"
	"time"

	"github.com/velosobr/passarim-catalog/internal/domain"
)

// Handler executa um job de uma fonte (IngestPhotos, IngestAudio...).
type Handler interface {
	Run(ctx context.Context, job Job) error
}

// Runner liga a fila aos casos de uso.
type Runner struct {
	Queue        JobQueue
	Handlers     map[Source]Handler
	BatchSize    int
	MaxAttempts  int
	RefreshAfter time.Duration
	Observe      func(source Source, ok bool) // métrica; opcional
}

// RunOnce enfileira o que falta, pega um lote e executa. Devolve quantos jobs
// foram pegos. O erro de UM job nunca interrompe os outros: ele só é
// registrado na fila (com backoff). O erro devolvido é de infraestrutura.
func (r Runner) RunOnce(ctx context.Context) (int, error) {
	if _, err := r.Queue.EnqueueMissing(ctx, AllSources, r.RefreshAfter); err != nil {
		return 0, fmt.Errorf("enfileirar: %w", err)
	}
	jobs, err := r.Queue.ClaimDue(ctx, r.BatchSize)
	if err != nil {
		return 0, fmt.Errorf("pegar jobs: %w", err)
	}
	for _, job := range jobs {
		r.runJob(ctx, job)
	}
	return len(jobs), nil
}

func (r Runner) runJob(ctx context.Context, job Job) {
	var err error
	if h, ok := r.Handlers[job.Source]; ok {
		err = h.Run(ctx, job)
	} else {
		err = fmt.Errorf("fonte sem handler: %q", job.Source)
	}
	if r.Observe != nil {
		r.Observe(job.Source, err == nil)
	}
	if err == nil {
		_ = r.Queue.Complete(ctx, job.ID)
		return
	}
	_, known := r.Handlers[job.Source]
	giveUp := job.Attempts+1 >= r.MaxAttempts || !known // sem handler, tentar de novo não adianta
	_ = r.Queue.Fail(ctx, job.ID, err.Error(), domain.NextAttemptDelay(job.Attempts), giveUp)
}
