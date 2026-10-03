package s3store_test

import (
	"context"
	"testing"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/velosobr/passarim-catalog/internal/adapter/s3store"
)

func TestStore_PutAndGet(t *testing.T) {
	if testing.Short() {
		t.Skip("integração")
	}
	ctx := context.Background()
	ctr, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "chrislusf/seaweedfs:4.48",
			Cmd:          []string{"server", "-s3", "-dir=/data"},
			Env:          map[string]string{"AWS_ACCESS_KEY_ID": "test", "AWS_SECRET_ACCESS_KEY": "test-secret"},
			ExposedPorts: []string{"8333/tcp"},
			WaitingFor:   wait.ForHTTP("/healthz").WithPort("8333/tcp"),
		},
		Started: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ctr.Terminate(context.Background()) })
	endpoint, err := ctr.PortEndpoint(ctx, "8333/tcp", "")
	if err != nil {
		t.Fatal(err)
	}

	st, err := s3store.New(ctx, s3store.Config{Endpoint: endpoint, AccessKey: "test", SecretKey: "test-secret", Bucket: "passarim-media"})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Put(ctx, "species/x/photo-0-thumb.webp", "image/webp", []byte("RIFF....WEBP")); err != nil {
		t.Fatal(err)
	}
	got, err := st.Get(ctx, "species/x/photo-0-thumb.webp")
	if err != nil || string(got) != "RIFF....WEBP" {
		t.Fatalf("get: %q %v", got, err)
	}
	// Criar de novo (bucket já existe) não pode falhar.
	if _, err := s3store.New(ctx, s3store.Config{Endpoint: endpoint, AccessKey: "test", SecretKey: "test-secret", Bucket: "passarim-media"}); err != nil {
		t.Fatalf("New com bucket existente: %v", err)
	}
}
