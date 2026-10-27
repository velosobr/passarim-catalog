// worker: busca fotos, cantos e avistamentos nas fontes externas e grava no
// catálogo. Como na API, aqui só "ligamos os fios": nenhuma regra de negócio.
//
// O worker trabalha em rodadas: pega um lote de jobs da fila (no Postgres),
// executa cada um com o handler da fonte certa (iNaturalist para fotos,
// xeno-canto para cantos, GBIF para avistamentos) e volta a esperar. Nada
// disso acontece quando o app pede uma ave: o app só lê o que o worker já
// gravou (ADR-0005).
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

	"github.com/velosobr/passarim-catalog/internal/adapter/curator"
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

// batchTimeout é o tempo de uma rodada sem job de fotos. A rodada inteira
// ganha também o prazo do job de fotos (cfg.PhotoJobTimeout), porque ele
// pode ser o último a rodar: somar garante o prazo inteiro a ele.
const batchTimeout = 2 * time.Minute

// main sobe o worker e sai com 1 se ele terminar com erro.
func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(log); err != nil {
		log.Error("worker encerrou com erro", "error", err)
		os.Exit(1)
	}
}

// run monta o worker: configuração, banco (com as migrations), object
// storage, métricas, o downloader seguro, o ffmpeg e os três handlers da
// fila; depois sobe o servidor de métricas e roda o laço de rodadas até o
// sinal de parar.
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
	// O object storage (SeaweedFS, compatível com S3) guarda os arquivos de
	// mídia já convertidos (ADR-0012).
	store, err := s3store.New(ctx, s3store.Config{Endpoint: cfg.S3Endpoint, AccessKey: cfg.S3AccessKey,
		SecretKey: cfg.S3SecretKey, Bucket: cfg.S3Bucket, UseSSL: cfg.S3UseSSL})
	if err != nil {
		return err
	}

	// promauto cria a métrica e já a registra no registro padrão do Prometheus.
	jobs := promauto.NewCounterVec(prometheus.CounterOpts{Name: "passarim_ingest_jobs_total",
		Help: "Jobs de ingestão processados, por fonte e resultado."}, []string{"source", "result"})
	blocked := promauto.NewCounterVec(prometheus.CounterOpts{Name: "passarim_downloads_blocked_total",
		Help: "Downloads bloqueados pelas regras anti-SSRF."}, []string{"reason"})

	// O downloader só baixa dos hosts conhecidos das fontes e recusa endereços
	// internos (proteção contra SSRF, veja o pacote safehttp).
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
	// O curador (passarim-photo-curator) diz o que cada foto mostra. Só ele
	// e o worker estão na rede passarim-curation do compose.
	photoCurator, err := curator.New(cfg.CuratorAddr, 30*time.Second)
	if err != nil {
		return err
	}
	defer func() { _ = photoCurator.Close() }()
	repo := postgres.NewIngestRepository(pool)
	policy := domain.LicensePolicy{AllowNC: cfg.AllowNC}
	// Cada fonte tem um rate limiter: no máximo uma chamada por segundo (meio
	// segundo no GBIF), para respeitar os limites das APIs públicas.
	oneSec := func() *rate.Limiter { return rate.NewLimiter(rate.Every(time.Second), 1) }

	// O Runner é o laço da fila. RefreshAfter (30 dias) é de quanto em quanto
	// tempo cada espécie é buscada de novo, e MaxAttempts (5) quantas vezes um
	// job que falha é tentado antes de desistir. Cada handler recebe as peças
	// de que precisa: a fonte, o downloader, o conversor, o storage e o banco.
	// Limites: galeria de até 6 fotos escolhidas pelo curador entre até 40
	// candidatas, e um job de fotos por rodada; canto cortado em 30 s e até
	// 900 avistamentos agrupados em células de 1°.
	runner := ingest.Runner{
		Queue: repo, BatchSize: cfg.BatchSize, MaxAttempts: 5, RefreshAfter: 30 * 24 * time.Hour,
		MaxPerRound: map[ingest.Source]int{ingest.SourceINaturalist: 1},
		Handlers: map[ingest.Source]ingest.Handler{
			ingest.SourceINaturalist: ingest.IngestPhotos{
				Source:     inaturalist.New("https://api.inaturalist.org", cfg.UserAgent, oneSec()),
				Downloader: downloader, Images: ffmpeg, Curator: photoCurator, Store: store, Repo: repo, Overrides: repo,
				Policy: policy, MaxPhotos: 6, MaxCandidates: 40, Observations: 20,
				LowConfidence: cfg.LowConfidence, Timeout: cfg.PhotoJobTimeout},
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

	// O laço roda numa goroutine; fechar loopDone avisa que ele terminou.
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
		n := runBatch(ctx, log, runner, batchTimeout+cfg.PhotoJobTimeout)
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
// prazo da rodada, timeout = batchTimeout + PHOTO_JOB_TIMEOUT): um SIGTERM
// deixa o lote em andamento terminar, desde que o orquestrador espere esse
// tempo antes de matar o processo (stop_grace_period de 8m30s no compose).
func runBatch(ctx context.Context, log *slog.Logger, runner ingest.Runner, timeout time.Duration) int {
	batchCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), timeout)
	defer cancel()
	n, err := runner.RunOnce(batchCtx)
	if err != nil {
		log.Error("rodada falhou", "error", err)
		return 0
	}
	log.Info("rodada concluída", "processed", n)
	return n
}
