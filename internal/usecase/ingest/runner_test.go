// Testes do laço da fila.

package ingest_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/velosobr/passarim-catalog/internal/usecase/ingest"
)

// handlerFunc deixa uma função qualquer servir de Handler.
type handlerFunc func(context.Context, ingest.Job) error

func (f handlerFunc) Run(ctx context.Context, j ingest.Job) error { return f(ctx, j) }

// observed registra o que foi passado ao Observe (a métrica).
type observed struct {
	source ingest.Source
	ok     bool
}

// O job que dá certo é concluído; o que falha é reagendado com backoff.
func TestRunner_CompletesSuccessfulAndRetriesFailed(t *testing.T) {
	q := &fakeQueue{jobs: []ingest.Job{
		{ID: 1, Source: ingest.SourceINaturalist},
		{ID: 2, Source: ingest.SourceGBIF},
	}}
	var obs []observed
	r := ingest.Runner{Queue: q, BatchSize: 5, MaxAttempts: 5, RefreshAfter: time.Hour,
		Handlers: map[ingest.Source]ingest.Handler{
			ingest.SourceINaturalist: handlerFunc(func(context.Context, ingest.Job) error { return nil }),
			ingest.SourceGBIF:        handlerFunc(func(context.Context, ingest.Job) error { return errors.New("gbif fora do ar") }),
		},
		Observe: func(s ingest.Source, ok bool) { obs = append(obs, observed{s, ok}) }}
	n, err := r.RunOnce(context.Background())
	if err != nil || n != 2 {
		t.Fatalf("processed=%d err=%v", n, err)
	}
	if len(q.completed) != 1 || q.completed[0] != 1 {
		t.Fatalf("completed: %v", q.completed)
	}
	if len(q.failed) != 1 || q.failed[0] != (failCall{2, "gbif fora do ar", time.Minute, false}) {
		t.Fatalf("failed: %+v", q.failed)
	}
	if len(obs) != 2 || obs[0] != (observed{ingest.SourceINaturalist, true}) || obs[1] != (observed{ingest.SourceGBIF, false}) {
		t.Fatalf("observe: %+v", obs)
	}
}

// Na última tentativa, o job desiste.
func TestRunner_GivesUpAfterMaxAttempts(t *testing.T) {
	q := &fakeQueue{jobs: []ingest.Job{{ID: 1, Source: ingest.SourceGBIF, Attempts: 4}}}
	r := ingest.Runner{Queue: q, BatchSize: 5, MaxAttempts: 5,
		Handlers: map[ingest.Source]ingest.Handler{ingest.SourceGBIF: handlerFunc(func(context.Context, ingest.Job) error { return errors.New("x") })}}
	if _, err := r.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(q.failed) != 1 || !q.failed[0].giveUp {
		t.Fatalf("5ª falha deveria desistir: %+v", q.failed)
	}
}

// Um job de uma fonte sem handler falha de vez, sem derrubar o worker.
func TestRunner_UnknownSourceFailsJobWithoutPanic(t *testing.T) {
	q := &fakeQueue{jobs: []ingest.Job{{ID: 1, Source: "desconhecida"}}}
	r := ingest.Runner{Queue: q, BatchSize: 5, MaxAttempts: 5, Handlers: map[ingest.Source]ingest.Handler{}}
	if _, err := r.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(q.failed) != 1 || !q.failed[0].giveUp {
		t.Fatalf("fonte sem handler deveria falhar de vez: %+v", q.failed)
	}
}

// Se o contexto do lote expira durante um job, Fail precisa
// funcionar mesmo assim, e os jobs que nem começaram são devolvidos sem
// gastar tentativa nem poluir a métrica.
func TestRunner_ExpiredBatchStillRecordsFailureAndReleasesUnstartedJobs(t *testing.T) {
	q := &fakeQueue{jobs: []ingest.Job{{ID: 1, Source: ingest.SourceGBIF}, {ID: 2, Source: ingest.SourceGBIF}, {ID: 3, Source: ingest.SourceGBIF}}}
	ctx, cancel := context.WithCancel(context.Background())
	var observed int
	r := ingest.Runner{Queue: q, BatchSize: 5, MaxAttempts: 5,
		Handlers: map[ingest.Source]ingest.Handler{ingest.SourceGBIF: handlerFunc(func(ctx context.Context, j ingest.Job) error {
			cancel() // o lote estoura durante o job 1
			return ctx.Err()
		})},
		Observe: func(ingest.Source, bool) { observed++ }}
	if _, err := r.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	if len(q.failed) != 1 || q.failed[0].id != 1 {
		t.Fatalf("job 1 deveria ter a falha registrada: %+v", q.failed)
	}
	for _, e := range q.ctxErrOnFinish {
		if e != nil {
			t.Fatalf("Fail/Complete rodou com contexto já expirado: %v", e)
		}
	}
	if len(q.released) != 2 || q.released[0] != 2 || q.released[1] != 3 {
		t.Fatalf("jobs 2 e 3 deveriam ser devolvidos: %+v", q.released)
	}
	if observed != 1 {
		t.Fatalf("Observe deveria contar só o job executado, veio %d", observed)
	}
}

// Curador fora do ar: o job é adiado (sem gastar tentativa), não falha. A
// espera cresce com os adiamentos anteriores (backoff).
func TestRunner_DefersCuratorUnavailable(t *testing.T) {
	q := &fakeQueue{jobs: []ingest.Job{{ID: 1, Source: ingest.SourceINaturalist, Attempts: 4, Deferrals: 2}}}
	r := ingest.Runner{Queue: q, BatchSize: 5, MaxAttempts: 5,
		Handlers: map[ingest.Source]ingest.Handler{ingest.SourceINaturalist: handlerFunc(func(context.Context, ingest.Job) error {
			return fmt.Errorf("classificar: %w", ingest.ErrCuratorUnavailable)
		})}}
	if _, err := r.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(q.failed) != 0 || len(q.deferred) != 1 || q.deferred[0].retryIn != 4*time.Minute {
		t.Fatalf("esperava adiar 4 min sem falhar: failed=%+v deferred=%+v", q.failed, q.deferred)
	}
}

// No máximo 1 job de fotos por rodada: os outros voltam para a fila sem rodar.
func TestRunner_OnePhotoJobPerRound(t *testing.T) {
	q := &fakeQueue{jobs: []ingest.Job{
		{ID: 1, Source: ingest.SourceINaturalist}, {ID: 2, Source: ingest.SourceGBIF}, {ID: 3, Source: ingest.SourceINaturalist},
	}}
	var ran []int64
	h := handlerFunc(func(_ context.Context, j ingest.Job) error { ran = append(ran, j.ID); return nil })
	r := ingest.Runner{Queue: q, BatchSize: 5, MaxAttempts: 5, MaxPerRound: map[ingest.Source]int{ingest.SourceINaturalist: 1},
		Handlers: map[ingest.Source]ingest.Handler{ingest.SourceINaturalist: h, ingest.SourceGBIF: h}}
	n, err := r.RunOnce(context.Background())
	if err != nil || n != 3 {
		t.Fatalf("n=%d err=%v (o lote pegou 3)", n, err)
	}
	if fmt.Sprint(ran) != "[1 2]" || fmt.Sprint(q.released) != "[3]" {
		t.Fatalf("rodou %v, devolveu %v", ran, q.released)
	}
}
