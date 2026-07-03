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

// ErrKingClipNotFound is returned by KingClipRepository when the requested
// king clip does not exist (or, for GetCurrent, none is currently live).
var ErrKingClipNotFound = errors.New("king clip: not found")

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

	// UpdateConversion sets the mp4_object_key and conversion_status for a clip.
	UpdateConversion(ctx context.Context, id int64, mp4Key, status string) error

	// ClaimConversion atomically transitions a clip from 'none' or 'failed' to
	// 'pending' with the given mp4Key. Returns true if exactly one row was
	// claimed (i.e. the status was 'none' or 'failed' before the call). Returns
	// false without error when another worker already holds the pending state.
	ClaimConversion(ctx context.Context, id int64, mp4Key string) (bool, error)
}

// SessionRepository resolves Redis session keys to user IDs.
type SessionRepository interface {
	// UserIDBySession returns the user_id stored under session:<sessionID>.
	// Returns ErrSessionNotFound when the key is absent or has expired.
	UserIDBySession(ctx context.Context, sessionID string) (int64, error)
}

// KingClipRepository is the data-access contract for the king_clips table.
// King clips are a category separate from the clips table (win-clips): they
// are never touched by ClipRepository.DeleteOldestBeyondLimit, and clips
// uploads never evict rows here — the two categories are fully independent.
type KingClipRepository interface {
	// Create inserts a new king clip row and returns it with the generated id
	// and created_at populated.
	Create(ctx context.Context, clip domain.KingClip) (domain.KingClip, error)

	// GetCurrent returns the latest non-expired king clip for hillType.
	// Returns ErrKingClipNotFound when none is currently live.
	GetCurrent(ctx context.Context, hillType string) (domain.KingClip, error)

	// GetByID returns the king clip with the given id or ErrKingClipNotFound
	// when absent.
	GetByID(ctx context.Context, id int64) (domain.KingClip, error)

	// Delete removes the king clip row with the given id and returns its
	// object_key. Returns ErrKingClipNotFound when absent.
	Delete(ctx context.Context, id int64) (objectKey string, err error)

	// DeleteSupersededByHill deletes all king clips for hillType other than
	// keepID and returns their object_key values. Used to evict the prior
	// king clip(s) for a hill when a new one is uploaded.
	DeleteSupersededByHill(ctx context.Context, hillType string, keepID int64) ([]string, error)

	// DeleteExpired deletes all king clips whose expires_at has passed and
	// returns their object_key values.
	DeleteExpired(ctx context.Context) ([]string, error)
}
