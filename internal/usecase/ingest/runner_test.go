package ingest_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/velosobr/passarim-catalog/internal/usecase/ingest"
)

type handlerFunc func(context.Context, ingest.Job) error

func (f handlerFunc) Run(ctx context.Context, j ingest.Job) error { return f(ctx, j) }

type observed struct {
	source ingest.Source
	ok     bool
}

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
