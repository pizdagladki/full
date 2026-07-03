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

// KingClipService is the business-logic contract for king-of-the-hill clip
// operations. King clips are a category separate from win-clips (ClipService):
// they are not subject to the per-user keep-last-10 FIFO rotation, and their
// lifecycle is tied to a hill term (ExpiresAt) instead.
type KingClipService interface {
	// Upload validates, stores, and records a new king clip for userID on the
	// given hill. Returns domain.ErrInvalidHillType, domain.ErrInvalidContentType,
	// domain.ErrTooLarge, or domain.ErrInvalidBlinkTs on bad input. On success the
	// prior king clip(s) for that hill_type are superseded (object + metadata
	// removed, best-effort).
	Upload(
		ctx context.Context, userID int64, hillType string, blinkTsMs int64,
		contentType string, size int64, r io.Reader,
	) (domain.KingClip, error)

	// CurrentURL returns a pre-signed download URL and the blink_ts_ms for the
	// current (latest non-expired) king clip on hillType. Returns
	// domain.ErrInvalidHillType on an unknown hill type, and
	// repository.ErrKingClipNotFound when no king clip is currently live.
	CurrentURL(ctx context.Context, hillType string) (url string, blinkTsMs int64, err error)

	// Delete removes the king clip identified by id, owned by userID (object +
	// metadata). Returns repository.ErrKingClipNotFound when the clip doesn't
	// exist or belongs to a different user.
	Delete(ctx context.Context, userID, id int64) error
}
