package repository

import (
	"bytes"
	"context"
	"fmt"

	"github.com/minio/minio-go/v7"
)

type bugRecordingStorage struct {
	client    *minio.Client
	bucket    string
	keyPrefix string
}

// NewBugRecordingStorage constructs a MinIO-backed BugRecordingStorage.
// Objects are stored in bucket under keyPrefix+key.
func NewBugRecordingStorage(client *minio.Client, bucket, keyPrefix string) BugRecordingStorage {
	return &bugRecordingStorage{
		client:    client,
		bucket:    bucket,
		keyPrefix: keyPrefix,
	}
}

// StoreRecording writes data to object storage under keyPrefix+key with
// Content-Type video/webm.
func (s *bugRecordingStorage) StoreRecording(ctx context.Context, key string, data []byte) error {
	objectKey := s.keyPrefix + key

	_, err := s.client.PutObject(
		ctx,
		s.bucket,
		objectKey,
		bytes.NewReader(data),
		int64(len(data)),
		minio.PutObjectOptions{ContentType: "video/webm"},
	)
	if err != nil {
		return fmt.Errorf("put object %q: %w", objectKey, err)
	}

	return nil
}
