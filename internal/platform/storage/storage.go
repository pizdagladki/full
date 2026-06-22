// Package storage provides the shared MinIO (S3-compatible) object-storage
// client used by services that need it. It arrives with the first service that
// needs it and must not be duplicated inside a service.
package storage

import (
	"context"
	"errors"
	"fmt"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// Config holds the MinIO connection and bucket settings.
type Config struct {
	Endpoint  string
	AccessKey string
	SecretKey string
	Bucket    string
	UseSSL    bool
}

// New creates a MinIO client for the given config, verifies connectivity, and
// ensures the target bucket exists (creating it if absent). The client is not
// returned alongside an error — on any failure the caller receives nil, err.
func New(ctx context.Context, cfg Config) (*minio.Client, error) {
	client, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
	})
	if err != nil {
		// minio.New only returns an error for malformed endpoints; the error
		// may embed the raw endpoint, so wrap with a static prefix.
		return nil, errors.New("create minio client: invalid endpoint or options")
	}

	exists, err := client.BucketExists(ctx, cfg.Bucket)
	if err != nil {
		return nil, fmt.Errorf("check bucket existence: %w", err)
	}

	if !exists {
		if err = client.MakeBucket(ctx, cfg.Bucket, minio.MakeBucketOptions{}); err != nil {
			return nil, fmt.Errorf("create bucket %q: %w", cfg.Bucket, err)
		}
	}

	return client, nil
}
