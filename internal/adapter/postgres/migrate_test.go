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
	// A 0005 veio depois: primeiro descemos até a 4 para testar a 0004 sozinha.
	if err := m.Migrate(4); err != nil {
		t.Fatalf("descer até a 0004: %v", err)
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

// A 0005 (frente B) sobe, desce e sobe de novo: o down apaga a tabela da
// "Ave do dia" e as colunas novas de species e media.
func TestMigration0005UpDownUp(t *testing.T) {
	_, pool := newRepo(t)
	ctx := context.Background()
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
	if v, _, _ := m.Version(); v != 5 {
		t.Fatalf("esperava a versão 5, veio %d", v)
	}
	if err := m.Steps(-1); err != nil {
		t.Fatalf("descer a 0005: %v", err)
	}
	var n int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns
		WHERE (table_name = 'species' AND column_name IN ('primary_biome', 'kids'))
		   OR (table_name = 'media' AND column_name = 'peaks')
		   OR table_name = 'featured_day'`).Scan(&n)
	if n != 0 {
		t.Fatalf("o down deveria apagar a tabela e as 3 colunas, sobraram %d colunas", n)
	}
	if err := m.Up(); err != nil {
		t.Fatalf("subir de novo: %v", err)
	}
}

// O CHECK de media.peaks aceita a lista vazia e a de 64 valores, e recusa
// qualquer outro tamanho.
func TestMigration0005_PeaksCheck(t *testing.T) {
	repo, pool := newRepo(t)
	ctx := context.Background()
	seed(t, repo, sp("Turdus rufiventris", "Sabiá-laranjeira", nil))
	insert := func(peaks string) error {
		_, err := pool.Exec(ctx, `INSERT INTO media (species_id, kind, position, audio_key, author, license, source, source_url, peaks)
			VALUES ('turdus-rufiventris', 'audio', 0, 'a.aac', 'A', 'CC-BY', 'xeno-canto', 'https://x', `+peaks+`)`)
		_, _ = pool.Exec(ctx, `DELETE FROM media`)
		return err
	}
	if err := insert(`'{}'`); err != nil {
		t.Fatalf("lista vazia recusada: %v", err)
	}
	if err := insert(`array_fill(0.5::real, ARRAY[64])`); err != nil {
		t.Fatalf("64 valores recusados: %v", err)
	}
	if err := insert(`'{0.1,0.2,0.3}'`); err == nil {
		t.Fatal("3 valores deveriam violar o CHECK")
	}
}
