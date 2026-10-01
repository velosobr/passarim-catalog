package grpcadapter_test

import (
	"context"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	grpcadapter "github.com/velosobr/passarim-catalog/internal/adapter/grpc"
)

func TestUnaryRequestID_KeepsIncomingID(t *testing.T) {
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("x-request-id", "abc-123"))
	var seen string
	handler := func(ctx context.Context, _ any) (any, error) { seen = grpcadapter.RequestIDFrom(ctx); return nil, nil }
	_, _ = grpcadapter.UnaryRequestID()(ctx, nil, &grpc.UnaryServerInfo{}, handler)
	if seen != "abc-123" {
		t.Fatalf("request id = %q, want abc-123", seen)
	}
}

func TestUnaryRequestID_GeneratesWhenMissing(t *testing.T) {
	var seen string
	handler := func(ctx context.Context, _ any) (any, error) { seen = grpcadapter.RequestIDFrom(ctx); return nil, nil }
	_, _ = grpcadapter.UnaryRequestID()(context.Background(), nil, &grpc.UnaryServerInfo{}, handler)
	if len(seen) != 32 {
		t.Fatalf("esperava id gerado de 32 hex, veio %q", seen)
	}
}

func TestUnaryRequestID_RejectsOversizedID(t *testing.T) {
	long := string(make([]byte, 200))
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("x-request-id", long))
	var seen string
	handler := func(ctx context.Context, _ any) (any, error) { seen = grpcadapter.RequestIDFrom(ctx); return nil, nil }
	_, _ = grpcadapter.UnaryRequestID()(ctx, nil, &grpc.UnaryServerInfo{}, handler)
	if seen == long || len(seen) != 32 {
		t.Fatalf("id gigante deveria ser trocado por um gerado, veio len=%d", len(seen))
	}
}
