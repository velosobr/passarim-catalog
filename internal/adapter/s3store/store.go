// Package s3store grava arquivos num object storage compatível com S3
// (SeaweedFS no ambiente local, Cloudflare R2 em produção — ADR-0006/0012).
// O minio-go é só um CLIENTE S3 genérico; funciona com qualquer servidor S3.
package s3store

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"github.com/velosobr/passarim-catalog/internal/usecase/ingest"
)

type Config struct {
	Endpoint  string // host:porta, sem "http://"
	AccessKey string
	SecretKey string
	Bucket    string
	UseSSL    bool
}

type Store struct {
	client *minio.Client
	bucket string
}

var _ ingest.MediaStore = (*Store)(nil)

func New(ctx context.Context, cfg Config) (*Store, error) {
	client, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("cliente s3: %w", err)
	}
	exists, err := client.BucketExists(ctx, cfg.Bucket)
	if err != nil {
		return nil, fmt.Errorf("verificar bucket: %w", err)
	}
	if !exists {
		if err := client.MakeBucket(ctx, cfg.Bucket, minio.MakeBucketOptions{}); err != nil {
			return nil, fmt.Errorf("criar bucket: %w", err)
		}
	}
	return &Store{client: client, bucket: cfg.Bucket}, nil
}

// Put grava (ou substitui) um arquivo. O Content-Type é guardado junto,
// para a CDN servir o arquivo com o tipo certo.
func (s *Store) Put(ctx context.Context, key, contentType string, data []byte) error {
	_, err := s.client.PutObject(ctx, s.bucket, key, bytes.NewReader(data), int64(len(data)),
		minio.PutObjectOptions{ContentType: contentType})
	return err
}

func (s *Store) Get(ctx context.Context, key string) ([]byte, error) {
	obj, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}
	defer obj.Close()
	return io.ReadAll(obj)
}
