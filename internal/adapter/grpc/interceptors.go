package grpcadapter

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// Interceptors são o "middleware" do gRPC: código que roda antes/depois
// de TODO método, sem repetir em cada um.

type ctxKey struct{}

const requestIDHeader = "x-request-id"

// RequestIDFrom devolve o id da requisição guardado no contexto.
func RequestIDFrom(ctx context.Context) string {
	id, _ := ctx.Value(ctxKey{}).(string)
	return id
}

func newRequestID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b) // crypto/rand não falha em sistemas suportados
	return hex.EncodeToString(b)
}

// UnaryRequestID reaproveita o x-request-id enviado pelo BFF (para seguir a
// mesma requisição nos logs dos dois serviços) ou gera um novo. Ids enormes
// são descartados: não confiamos cegamente em dados de entrada.
func UnaryRequestID() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		id := ""
		if md, ok := metadata.FromIncomingContext(ctx); ok {
			if v := md.Get(requestIDHeader); len(v) > 0 && len(v[0]) > 0 && len(v[0]) <= 64 {
				id = v[0]
			}
		}
		if id == "" {
			id = newRequestID()
		}
		_ = grpc.SetHeader(ctx, metadata.Pairs(requestIDHeader, id))
		return handler(context.WithValue(ctx, ctxKey{}, id), req)
	}
}

// UnaryLogging registra uma linha por chamada: método, código e duração.
func UnaryLogging(log *slog.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		start := time.Now()
		resp, err := handler(ctx, req)
		log.InfoContext(ctx, "grpc",
			"method", info.FullMethod, "code", status.Code(err).String(),
			"duration_ms", time.Since(start).Milliseconds(), "request_id", RequestIDFrom(ctx))
		return resp, err
	}
}
