// catalog-api: ponto de entrada do serviço. Aqui só "ligamos os fios":
// lemos a config, criamos as dependências e as entregamos umas às outras.
// Nenhuma regra de negócio mora no main.
//
// O catalog tem três programas (ADR-0005, pré-ingestão): esta API gRPC, que
// só LÊ o banco e responde ao BFF; o worker, que busca dados nas fontes
// externas e grava; e o seed, que carrega o conteúdo curado. Separados, a
// API nunca fica lenta por causa de uma busca no iNaturalist, e cada um pode
// ser reiniciado sem derrubar os outros. O banco pertence ao catalog: nenhum
// outro serviço o acessa (ADR-0004).
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	grpcprom "github.com/grpc-ecosystem/go-grpc-middleware/providers/prometheus"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	catalogv1 "github.com/velosobr/passarim-proto/gen/go/passarim/catalog/v1"

	grpcadapter "github.com/velosobr/passarim-catalog/internal/adapter/grpc"
	"github.com/velosobr/passarim-catalog/internal/adapter/postgres"
	"github.com/velosobr/passarim-catalog/internal/config"
	"github.com/velosobr/passarim-catalog/internal/usecase"
)

// main cria o logger JSON e decide entre o healthcheck e a API de verdade.
func main() {
	// Logs em JSON: fáceis de buscar e filtrar em qualquer ferramenta.
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	// "catalog-api healthcheck": usado pelo Docker. A imagem distroless não
	// tem shell nem curl, então o próprio binário faz a checagem.
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		os.Exit(healthcheck())
	}
	if err := run(log); err != nil {
		log.Error("catalog-api encerrou com erro", "error", err)
		os.Exit(1)
	}
}

// run monta a API e só retorna quando ela termina. A ordem: configuração →
// sinais de desligamento → pool de conexões com o Postgres → espera o banco
// responder → migrations (se ligadas) → repositório e casos de uso →
// servidor gRPC com interceptors, saúde e métricas → servidor de métricas →
// espera o sinal de parar → desligamento gracioso.
func run(log *slog.Logger) error {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}

	// Contexto cancelado ao receber SIGINT (Ctrl+C) ou SIGTERM (docker stop).
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Um pool mantém várias conexões abertas com o Postgres e as reaproveita
	// entre as chamadas: abrir uma conexão nova a cada consulta seria lento.
	// WaitForDB tenta o ping até 30 vezes, uma por segundo, porque no docker
	// compose o banco pode subir depois da API.
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("configurar pool: %w", err)
	}
	defer pool.Close()
	if err := postgres.WaitForDB(ctx, pool.Ping, 30, time.Second); err != nil {
		return err
	}
	if cfg.MigrateOnStart {
		if err := postgres.Migrate(cfg.DatabaseURL); err != nil {
			return err
		}
		log.Info("migrations aplicadas")
	}

	// Injeção de dependências "à mão": repositório → casos de uso → servidor.
	repo := postgres.NewRepository(pool)
	srv := grpcadapter.NewServer(
		usecase.ListSpecies{Repo: repo}, usecase.GetSpecies{Repo: repo}, usecase.ListFilters{Repo: repo}, log)

	// Métricas do Prometheus para cada chamada gRPC (contagem e duração por
	// método e código de resposta).
	metrics := grpcprom.NewServerMetrics(grpcprom.WithServerHandlingTimeHistogram())
	prometheus.MustRegister(metrics)

	// Interceptors são os "middlewares" do gRPC: rodam em volta de cada chamada,
	// na ordem da lista. Primeiro pegamos o request_id, depois medimos e, por
	// fim, registramos o log da chamada.
	grpcServer := grpc.NewServer(grpc.ChainUnaryInterceptor(
		grpcadapter.UnaryRequestID(), metrics.UnaryServerInterceptor(), grpcadapter.UnaryLogging(log)))
	catalogv1.RegisterCatalogServiceServer(grpcServer, srv)
	// O serviço de saúde padrão do gRPC responde "SERVING" enquanto a API está
	// no ar; o healthcheck do Docker o consulta. Reflection (desligada por
	// padrão; GRPC_REFLECTION=true liga, para desenvolvimento) deixa
	// ferramentas como o grpcurl descobrirem os métodos.
	healthSrv := health.NewServer()
	healthpb.RegisterHealthServer(grpcServer, healthSrv)
	if cfg.Reflection {
		reflection.Register(grpcServer)
	}
	metrics.InitializeMetrics(grpcServer)

	// Servidor HTTP auxiliar: métricas para o Prometheus.
	mux := http.NewServeMux()
	mux.Handle("GET /metrics", promhttp.Handler())
	httpServer := &http.Server{Addr: cfg.MetricsAddr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}

	// Os dois servidores rodam em goroutines. O canal errCh recebe o erro de
	// qualquer um que cair; o select abaixo espera o sinal de parar ou essa queda.
	lis, err := net.Listen("tcp", cfg.GRPCAddr)
	if err != nil {
		return err
	}
	errCh := make(chan error, 2)
	go func() { errCh <- grpcServer.Serve(lis) }()
	go func() {
		if err := httpServer.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()
	log.Info("catalog-api no ar", "grpc", cfg.GRPCAddr, "metrics", cfg.MetricsAddr)

	select {
	case <-ctx.Done():
		log.Info("sinal recebido, encerrando com calma (graceful shutdown)")
	case err := <-errCh:
		return err
	}

	// Graceful shutdown: para de aceitar chamadas novas e espera as em
	// andamento terminarem (até 10 s). Depois disso, força o fim.
	healthSrv.Shutdown()
	done := make(chan struct{})
	go func() { grpcServer.GracefulStop(); close(done) }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		grpcServer.Stop()
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return httpServer.Shutdown(shutdownCtx)
}

// healthcheck chama o serviço padrão de saúde do gRPC no próprio container.
func healthcheck() int {
	addr := os.Getenv("GRPC_ADDR")
	if addr == "" {
		addr = ":50051"
	}
	conn, err := grpc.NewClient("localhost"+addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return 1
	}
	defer func() { _ = conn.Close() }()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	resp, err := healthpb.NewHealthClient(conn).Check(ctx, &healthpb.HealthCheckRequest{})
	if err != nil || resp.GetStatus() != healthpb.HealthCheckResponse_SERVING {
		return 1
	}
	return 0
}
