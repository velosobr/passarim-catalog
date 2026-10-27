// O laço da fila: enfileira o que falta, pega um lote e executa cada job com
// o handler da fonte certa, registrando sucesso ou falha.

package ingest

import (
	"context"
	"errors"
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
	// MaxPerRound limita quantos jobs de uma fonte rodam por rodada. O job de
	// fotos é longo (40 downloads, o curador, o ffmpeg): com 1 por rodada,
	// ele não disputa o prazo da rodada com outro igual. Os que passam do
	// limite voltam para a fila sem gastar tentativa e saem na rodada seguinte.
	MaxPerRound map[Source]int
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
	perSource := map[Source]int{}
	for _, job := range jobs {
		limit, limited := r.MaxPerRound[job.Source]
		if ctx.Err() != nil || (limited && perSource[job.Source] >= limit) {
			// O lote acabou (tempo ou shutdown), ou a fonte já rodou o seu
			// limite nesta rodada: devolvemos o job sem rodar, sem gastar
			// tentativa e sem sujar a métrica de erros.
			_ = r.finishCtx(ctx, func(c context.Context) error { return r.Queue.Release(c, job.ID) })
			continue
		}
		perSource[job.Source]++
		r.runJob(ctx, job)
	}
	return len(jobs), nil
}

// runJob executa um job e grava o resultado na fila. Curador indisponível
// adia o job (sem gastar tentativa). Na falha, a próxima tentativa é
// agendada com backoff exponencial; depois de MaxAttempts (ou se a fonte não
// tem handler), o job desiste.
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
	if errors.Is(err, ErrCuratorUnavailable) {
		// Curador fora do ar: adiar, não falhar. A espera cresce com os
		// adiamentos (1, 2, 4... min), não com as tentativas.
		_ = r.finishCtx(ctx, func(c context.Context) error {
			return r.Queue.Defer(c, job.ID, err.Error(), domain.NextAttemptDelay(job.Deferrals))
		})
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
