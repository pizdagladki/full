package repository

import (
	"context"
	"io"
	"net/url"
	"time"

	"github.com/minio/minio-go/v7"
)

// minioObjectStore implements service.ObjectStore using a *minio.Client.
// It is a thin adapter with no business logic.
type minioObjectStore struct {
	client *minio.Client
	bucket string
}

// NewMinioObjectStore returns a service.ObjectStore backed by a MinIO client.
// This is intentionally not exported as service.ObjectStore to avoid an import
// cycle; the caller (app layer) assigns it to the interface via the service
// constructor.
//
//nolint:revive // unexported type returned intentionally; caller (app layer) assigns to service.ObjectStore
func NewMinioObjectStore(client *minio.Client, bucket string) *minioObjectStore {
	return &minioObjectStore{client: client, bucket: bucket}
}

// Put uploads the contents of r to the given key in the bucket.
func (s *minioObjectStore) Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error {
	_, err := s.client.PutObject(ctx, s.bucket, key, r, size, minio.PutObjectOptions{
		ContentType: contentType,
	})

	return err
}

// PresignedGetURL returns a pre-signed GET URL for the given key valid for ttl.
// The URL carries response-content-disposition=attachment so that browsers
// download the file rather than rendering it inline, preventing any
// HTML/XSS-via-object-URL concern when the link is opened directly.
func (s *minioObjectStore) PresignedGetURL(ctx context.Context, key string, ttl time.Duration) (string, error) {
	reqParams := url.Values{}
	reqParams.Set("response-content-disposition", "attachment")

	u, err := s.client.PresignedGetObject(ctx, s.bucket, key, ttl, reqParams)
	if err != nil {
		return "", err
	}

	return u.String(), nil
}

// Remove deletes the object at key from the bucket.
func (s *minioObjectStore) Remove(ctx context.Context, key string) error {
	return s.client.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{})
}
