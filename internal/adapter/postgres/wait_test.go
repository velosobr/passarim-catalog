package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/velosobr/passarim-catalog/internal/adapter/postgres"
)

// Review Focus #6: o banco pode demorar a subir; o catalog deve esperar.
func TestWaitForDB_RetriesUntilReady(t *testing.T) {
	calls := 0
	ping := func(context.Context) error {
		calls++
		if calls < 3 {
			return errors.New("connection refused")
		}
		return nil
	}
	if err := postgres.WaitForDB(context.Background(), ping, 5, time.Millisecond); err != nil {
		t.Fatalf("deveria conectar na 3ª tentativa: %v", err)
	}
	if calls != 3 {
		t.Fatalf("calls = %d, want 3", calls)
	}
}

func TestWaitForDB_GivesUp(t *testing.T) {
	ping := func(context.Context) error { return errors.New("connection refused") }
	err := postgres.WaitForDB(context.Background(), ping, 3, time.Millisecond)
	if err == nil {
		t.Fatal("deveria desistir depois de 3 tentativas")
	}
}
