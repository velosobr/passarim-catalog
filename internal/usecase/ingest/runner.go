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
		if ctx.Err() != nil {
			// O lote acabou (tempo ou shutdown): devolvemos o job sem rodar,
			// sem gastar tentativa e sem sujar a métrica de erros.
			_ = r.finishCtx(ctx, func(c context.Context) error { return r.Queue.Release(c, job.ID) })
			continue
		}
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
		_ = r.finishCtx(ctx, func(c context.Context) error { return r.Queue.Complete(c, job.ID) })
		return
	}
	_, known := r.Handlers[job.Source]
	giveUp := job.Attempts+1 >= r.MaxAttempts || !known // sem handler, tentar de novo não adianta
	_ = r.finishCtx(ctx, func(c context.Context) error {
		return r.Queue.Fail(c, job.ID, err.Error(), domain.NextAttemptDelay(job.Attempts), giveUp)
	})
}

// finishCtx grava o resultado do job com um contexto PRÓPRIO: se o do lote já
// expirou (foi isso que fez o job falhar), registrar a falha ainda precisa funcionar,
// senão o job ficaria "running" sem contar a tentativa.
func (r Runner) finishCtx(ctx context.Context, fn func(context.Context) error) error {
	c, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	return fn(c)
}
