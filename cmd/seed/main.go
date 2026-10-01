// seed: carrega as aves curadas (content/species/*.yaml) no banco.
// Pode rodar quantas vezes quiser: o resultado é sempre o mesmo (idempotente).
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/velosobr/passarim-catalog/internal/adapter/curated"
	"github.com/velosobr/passarim-catalog/internal/adapter/postgres"
)

func main() {
	dir := flag.String("dir", "content/species", "pasta com os arquivos .yaml")
	flag.Parse()
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	// Primeiro valida TODOS os arquivos; só então toca no banco.
	species, err := curated.LoadDir(*dir)
	if err != nil {
		log.Error("conteúdo curado inválido", "error", err)
		os.Exit(1)
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Error("configurar pool", "error", err)
		os.Exit(1)
	}
	defer pool.Close()
	if err := postgres.WaitForDB(ctx, pool.Ping, 30, time.Second); err != nil {
		log.Error("banco indisponível", "error", err)
		os.Exit(1)
	}
	repo := postgres.NewRepository(pool)
	for _, s := range species {
		if err := repo.UpsertCurated(ctx, s); err != nil {
			log.Error("gravar espécie", "id", s.ID, "error", err)
			os.Exit(1)
		}
	}
	log.Info("seed concluído", "especies", len(species))
}
