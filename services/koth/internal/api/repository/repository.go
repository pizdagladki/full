// Package repository holds the koth service data access (hand-written SQL via
// pgx, mapping rows to domain models). Repository interfaces are added here by
// downstream resource slices via the new-resource skill.
package repository

//go:generate mockgen -source=repository.go -destination=mocks/repository_mock.go -package=mocks

import (
	"context"
	"errors"
	"time"

	"github.com/pizdagladki/full/services/koth/internal/api/domain"
)

// ErrRankNotFound is returned by RankRepository.GetRank when there is no
// hill_ranks row for the given (user_id, day) pair.
var ErrRankNotFound = errors.New("rank: not found")

// ErrSessionNotFound is returned by SessionRepository when the session key is
// absent from Redis or has expired.
var ErrSessionNotFound = errors.New("session: not found or expired")

// RankRepository is the data-access contract for the hill_ranks table.
type RankRepository interface {
	// GetRank returns the caller's rank row for (userID, day). Returns
	// ErrRankNotFound when no row exists yet.
	GetRank(ctx context.Context, userID int64, day time.Time) (*domain.HillRank, error)
	// UpsertRank inserts or updates the (userID, day) row with newRank and
	// bestHoldMs. The write is downgrade-safe: an existing row is only
	// overwritten when newRank is strictly greater than the stored rank.
	UpsertRank(ctx context.Context, userID int64, day time.Time, newRank, bestHoldMs int) error
	// RankDistribution returns the accounts-per-rank distribution for day,
	// ordered by rank ascending.
	RankDistribution(ctx context.Context, day time.Time) ([]domain.RankCount, error)
}

// SessionRepository resolves Redis session keys to user IDs.
type SessionRepository interface {
	// UserIDBySession returns the user_id stored under session:<sessionID>.
	// Returns ErrSessionNotFound when the key is absent or has expired.
	UserIDBySession(ctx context.Context, sessionID string) (int64, error)
}
