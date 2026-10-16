// Package postgres implementa o repositório de espécies sobre o PostgreSQL.
//
// As consultas são SQL escrito à mão (queries.sql e queries_ingest.sql). O
// sqlc lê esse SQL e as migrations e gera o código Go em sqlcgen/, com uma
// função e uma struct de parâmetros por consulta. Assim escrevemos SQL de
// verdade e o compilador do Go ainda confere os tipos. O pgx é o driver do
// Postgres, e o pgxpool, o pool de conexões dele.
//
// Este arquivo tem a espera pelo banco na subida.
package postgres

import (
	"context"
	"fmt"
	"time"
)

// WaitForDB tenta "pingar" o banco várias vezes, esperando delay entre as
// tentativas. No docker compose o catalog pode iniciar antes do PostgreSQL
// estar pronto; sem isso o serviço morreria logo na subida.
func WaitForDB(ctx context.Context, ping func(context.Context) error, attempts int, delay time.Duration) error {
	var err error
	for i := 1; i <= attempts; i++ {
		if err = ping(ctx); err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}
	return fmt.Errorf("banco indisponível após %d tentativas: %w", attempts, err)
}
