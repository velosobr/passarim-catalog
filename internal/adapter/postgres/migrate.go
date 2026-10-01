package postgres

import (
	"embed"
	"errors"
	"fmt"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5" // registra o driver "pgx5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

// //go:embed coloca os arquivos .sql DENTRO do binário. Assim a imagem
// Docker não precisa copiar a pasta de migrations separadamente.
//
//go:embed migrations/*.sql
var migrationsFS embed.FS

// Migrate aplica todas as migrations pendentes. Rodar de novo é seguro:
// as que já rodaram são puladas.
func Migrate(databaseURL string) error {
	src, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return err
	}
	// O driver pgx/v5 do golang-migrate usa o esquema "pgx5://".
	url := strings.Replace(databaseURL, "postgres://", "pgx5://", 1)
	m, err := migrate.NewWithSourceInstance("iofs", src, url)
	if err != nil {
		return fmt.Errorf("preparar migrations: %w", err)
	}
	defer func() { _, _ = m.Close() }()
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("aplicar migrations: %w", err)
	}
	return nil
}
