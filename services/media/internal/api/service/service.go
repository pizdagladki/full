// Package service holds the media service business logic (orchestrating
// repositories, MinIO object storage operations, and ffmpeg WebM→MP4 conversion
// via os/exec). Service interfaces are added here by downstream resource slices
// via the new-resource skill.
package service

//go:generate mockgen -source=service.go -destination=mocks/service_mock.go -package=mocks

import (
	"context"
	"io"
	"time"

	"github.com/pizdagladki/full/services/media/internal/api/domain"
)

// ObjectStore is the contract for MinIO object-storage operations used by
// ClipService.
type ObjectStore interface {
	// Put stores the content from r at key with the given size and content type.
	Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error

	// PresignedGetURL returns a pre-signed GET URL for key valid for ttl.
	PresignedGetURL(ctx context.Context, key string, ttl time.Duration) (string, error)

	// Remove deletes the object at key.
	Remove(ctx context.Context, key string) error

	// Get retrieves the object at key and returns a ReadCloser for its content.
	Get(ctx context.Context, key string) (io.ReadCloser, error)
}

// FFmpegRunner converts a WebM file at inputPath to an MP4 at outputPath.
type FFmpegRunner interface {
	Convert(ctx context.Context, inputPath, outputPath string) error
}

// ClipService is the business-logic contract for win-clip operations.
type ClipService interface {
	// Upload validates, stores, and records a new clip for userID.
	// Returns ErrInvalidContentType or ErrTooLarge on bad input.
	Upload(ctx context.Context, userID int64, contentType string, size int64, r io.Reader) (domain.Clip, error)

	// List returns the caller's clips, newest first.
	List(ctx context.Context, userID int64) ([]domain.Clip, error)

	// DownloadURL returns a pre-signed download URL for clipID owned by userID.
	// Returns ErrClipNotFound when the clip doesn't exist or belongs to a different user.
	DownloadURL(ctx context.Context, userID, clipID int64) (string, error)

	// RequestConvert triggers async WebM→MP4 conversion for clipID owned by userID.
	// Returns the current conversion status ("pending", "done"). Idempotent when
	// already done. Returns ErrClipNotFound when the clip doesn't exist or belongs
	// to a different user.
	RequestConvert(ctx context.Context, userID, clipID int64) (string, error)

	// GetMP4URL returns a presigned download URL for the MP4 of clipID owned by
	// userID. Returns ErrConversionNotDone when not yet converted,
	// ErrConversionFailed when ffmpeg previously failed, ErrClipNotFound otherwise.
	GetMP4URL(ctx context.Context, userID, clipID int64) (string, error)
}

// SessionService resolves a session cookie value to a user ID.
type SessionService interface {
	// ResolveSession returns the user_id stored under the session cookie value.
	// Returns the repository.ErrSessionNotFound sentinel when absent/expired.
	ResolveSession(ctx context.Context, sessionID string) (int64, error)
}
