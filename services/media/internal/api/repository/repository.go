// Package repository holds the media service data access (hand-written SQL via
// pgx, mapping rows to domain models, and MinIO object operations for win-clip
// upload/download). Repository interfaces are added here by downstream resource
// slices via the new-resource skill.
package repository

//go:generate mockgen -source=repository.go -destination=mocks/repository_mock.go -package=mocks

import (
	"context"
	"errors"

	"github.com/pizdagladki/full/services/media/internal/api/domain"
)

// ErrClipNotFound is returned by ClipRepository when the requested clip does
// not exist.
var ErrClipNotFound = errors.New("clip: not found")

// ErrSessionNotFound is returned by SessionRepository when the session key is
// absent from Redis or has expired.
var ErrSessionNotFound = errors.New("session: not found or expired")

// ClipRepository is the data-access contract for the clips table.
type ClipRepository interface {
	// Create inserts a new clip row and returns it with the generated id and
	// created_at populated.
	Create(ctx context.Context, clip domain.Clip) (domain.Clip, error)

	// ListByUser returns all clips belonging to userID ordered by created_at
	// DESC, id DESC (newest first).
	ListByUser(ctx context.Context, userID int64) ([]domain.Clip, error)

	// GetByID returns the clip with the given id or ErrClipNotFound when
	// absent.
	GetByID(ctx context.Context, id int64) (domain.Clip, error)

	// DeleteOldestBeyondLimit deletes the rows beyond the newest limit clips
	// for userID and returns their object_key values. If the user has at most
	// limit clips, nothing is deleted and an empty slice is returned.
	DeleteOldestBeyondLimit(ctx context.Context, userID int64, limit int) ([]string, error)
}

// SessionRepository resolves Redis session keys to user IDs.
type SessionRepository interface {
	// UserIDBySession returns the user_id stored under session:<sessionID>.
	// Returns ErrSessionNotFound when the key is absent or has expired.
	UserIDBySession(ctx context.Context, sessionID string) (int64, error)
}
