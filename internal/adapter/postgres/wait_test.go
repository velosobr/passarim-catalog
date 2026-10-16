// Testes da espera pelo banco, com um ping falso.

package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/velosobr/passarim-catalog/internal/adapter/postgres"
)

// O banco pode demorar a subir; o catalog deve esperar.
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

// Se o banco nunca responde, WaitForDB desiste depois das tentativas e
// devolve o último erro.
func TestWaitForDB_GivesUp(t *testing.T) {
	ping := func(context.Context) error { return errors.New("connection refused") }
	err := postgres.WaitForDB(context.Background(), ping, 3, time.Millisecond)
	if err == nil {
		t.Fatal("deveria desistir depois de 3 tentativas")
	}
}
