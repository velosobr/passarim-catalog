// Testa que a última migration sobe, desce e sobe de novo sem erro: o
// .down.sql precisa desfazer exatamente o .up.sql.

package postgres_test

import (
	"context"
	"strings"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"

	"github.com/velosobr/passarim-catalog/internal/adapter/postgres"
)

func TestMigration0004UpDownUp(t *testing.T) {
	_, pool := newRepo(t) // já aplica todas as migrations
	url := pool.Config().ConnString()
	src, err := iofs.New(postgres.MigrationsFS, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	m, err := migrate.NewWithSourceInstance("iofs", src, strings.Replace(url, "postgres://", "pgx5://", 1))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = m.Close() }()
	if v, _, _ := m.Version(); v != 4 {
		t.Fatalf("esperava a versão 4, veio %d", v)
	}
	if err := m.Steps(-1); err != nil {
		t.Fatalf("descer a 0004: %v", err)
	}
	var n int
	_ = pool.QueryRow(context.Background(),
		"SELECT count(*) FROM information_schema.tables WHERE table_name IN ('photo_override', 'curation_report')").Scan(&n)
	if n != 0 {
		t.Fatalf("o down deveria apagar as 2 tabelas, sobraram %d", n)
	}
	if err := m.Up(); err != nil {
		t.Fatalf("subir de novo: %v", err)
	}
}
