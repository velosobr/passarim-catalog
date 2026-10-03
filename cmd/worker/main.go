// worker: busca fotos, cantos e avistamentos nas fontes externas e grava no
// catálogo. Como na API, aqui só "ligamos os fios": nenhuma regra de negócio.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"golang.org/x/time/rate"

	"github.com/velosobr/passarim-catalog/internal/adapter/media"
	"github.com/velosobr/passarim-catalog/internal/adapter/postgres"
	"github.com/velosobr/passarim-catalog/internal/adapter/s3store"
	"github.com/velosobr/passarim-catalog/internal/adapter/safehttp"
	"github.com/velosobr/passarim-catalog/internal/adapter/sources/gbif"
	"github.com/velosobr/passarim-catalog/internal/adapter/sources/inaturalist"
	"github.com/velosobr/passarim-catalog/internal/adapter/sources/xenocanto"
	"github.com/velosobr/passarim-catalog/internal/config"
	"github.com/velosobr/passarim-catalog/internal/domain"
	"github.com/velosobr/passarim-catalog/internal/usecase/ingest"
)

const batchTimeout = 2 * time.Minute

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(log); err != nil {
		log.Error("worker encerrou com erro", "error", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	cfg, err := config.LoadWorker(os.Getenv)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("configurar pool: %w", err)
	}
	defer pool.Close()
	if err := postgres.WaitForDB(ctx, pool.Ping, 30, time.Second); err != nil {
		return err
	}
	// O worker também aplica as migrations, para poder subir antes da API.
	if err := postgres.Migrate(cfg.DatabaseURL); err != nil {
		return err
	}
	store, err := s3store.New(ctx, s3store.Config{Endpoint: cfg.S3Endpoint, AccessKey: cfg.S3AccessKey,
		SecretKey: cfg.S3SecretKey, Bucket: cfg.S3Bucket, UseSSL: cfg.S3UseSSL})
	if err != nil {
		return err
	}

	jobs := promauto.NewCounterVec(prometheus.CounterOpts{Name: "passarim_ingest_jobs_total",
		Help: "Jobs de ingestão processados, por fonte e resultado."}, []string{"source", "result"})
	blocked := promauto.NewCounterVec(prometheus.CounterOpts{Name: "passarim_downloads_blocked_total",
		Help: "Downloads bloqueados pelas regras anti-SSRF."}, []string{"reason"})

	downloader := safehttp.New(safehttp.Options{
		AllowedHosts: []string{"inaturalist-open-data.s3.amazonaws.com", "static.inaturalist.org", "xeno-canto.org", "www.xeno-canto.org"},
		UserAgent:    cfg.UserAgent,
		// Só o texto antes de ":" vira label: evita cardinalidade infinita (hosts, IPs).
		Blocked: func(reason string) {
			label, _, _ := strings.Cut(reason, ":")
			blocked.WithLabelValues(label).Inc()
		},
	})
	ffmpeg := media.NewFFmpeg(cfg.FFmpegPath)
	repo := postgres.NewIngestRepository(pool)
	policy := domain.LicensePolicy{AllowNC: cfg.AllowNC}
	oneSec := func() *rate.Limiter { return rate.NewLimiter(rate.Every(time.Second), 1) }

	runner := ingest.Runner{
		Queue: repo, BatchSize: cfg.BatchSize, MaxAttempts: 5, RefreshAfter: 30 * 24 * time.Hour,
		Handlers: map[ingest.Source]ingest.Handler{
			ingest.SourceINaturalist: ingest.IngestPhotos{
				Source:     inaturalist.New("https://api.inaturalist.org", cfg.UserAgent, oneSec()),
				Downloader: downloader, Images: ffmpeg, Store: store, Repo: repo, Policy: policy, MaxPhotos: 5},
			ingest.SourceXenoCanto: ingest.IngestAudio{
				Source:     xenocanto.New("https://xeno-canto.org", cfg.XenoCantoAPIKey, cfg.UserAgent, oneSec()),
				Downloader: downloader, Audio: ffmpeg, Store: store, Repo: repo, Policy: policy, MaxDuration: 30 * time.Second},
			ingest.SourceGBIF: ingest.IngestOccurrences{
				Source: gbif.New("https://api.gbif.org", rate.NewLimiter(rate.Every(500*time.Millisecond), 1)),
				Repo:   repo, MaxPoints: 900, CellDeg: 1.0},
		},
		Observe: func(s ingest.Source, ok bool) {
			result := "ok"
			if !ok {
				result = "error"
			}
			jobs.WithLabelValues(string(s), result).Inc()
		},
	}

	mux := http.NewServeMux()
	mux.Handle("GET /metrics", promhttp.Handler())
	httpServer := &http.Server{Addr: cfg.MetricsAddr, Handler: mux, ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second}
	errCh := make(chan error, 1)
	go func() {
		if err := httpServer.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()
	log.Info("worker no ar", "metrics", cfg.MetricsAddr, "interval", cfg.Interval.String(), "batch", cfg.BatchSize)

	loopDone := make(chan struct{})
	go func() { defer close(loopDone); loop(ctx, log, runner, cfg) }()

	select {
	case <-ctx.Done():
		log.Info("sinal recebido, encerrando com calma (graceful shutdown)")
	case err := <-errCh:
		stop()
		<-loopDone
		return err
	}
	<-loopDone // espera o lote em andamento terminar
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return httpServer.Shutdown(shutdownCtx)
}

// loop roda lotes até o contexto ser cancelado. Um lote cheio é seguido de
// outro imediatamente (há mais trabalho); senão espera o próximo tick.
func loop(ctx context.Context, log *slog.Logger, runner ingest.Runner, cfg config.WorkerConfig) {
	ticker := time.NewTicker(cfg.Interval)
	defer ticker.Stop()
	for {
		n := runBatch(ctx, log, runner)
		if n >= cfg.BatchSize && ctx.Err() == nil {
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// runBatch usa um contexto que NÃO é cancelado pelo shutdown (só pelo
// timeout): um job nunca fica pela metade só porque recebemos SIGTERM.
func runBatch(ctx context.Context, log *slog.Logger, runner ingest.Runner) int {
	batchCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), batchTimeout)
	defer cancel()
	n, err := runner.RunOnce(batchCtx)
	if err != nil {
		log.Error("rodada falhou", "error", err)
		return 0
	}
	log.Info("rodada concluída", "processed", n)
	return n
}
