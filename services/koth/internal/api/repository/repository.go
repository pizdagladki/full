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

// ErrHillNotFound is returned by HillRepository when there is no current
// (ended_at IS NULL) king_reigns row for a hill_type — the hill needs seeding.
var ErrHillNotFound = errors.New("hill: no current reign")

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

// HillRepository is the data-access contract for the king_reigns table.
type HillRepository interface {
	// CurrentKing returns the current (ended_at IS NULL) reign for hillType.
	// Returns ErrHillNotFound when the hill has never been seeded.
	CurrentKing(ctx context.Context, hillType domain.HillType) (*domain.KingReign, error)
	// Challenge compares survivedMs against the current king's blink_ts_ms
	// for hillType. If survivedMs >= the current king's blink_ts_ms, the
	// current reign is closed and a new reign is opened for userID with
	// newClipID and blink_ts_ms = survivedMs (the challenger's own blink
	// point); otherwise the reign is left unchanged. The whole compare +
	// close + open sequence is atomic and serialized per hill_type (a
	// Postgres advisory transaction lock keyed on hillType, taken before
	// re-reading the current king fresh), so concurrent challenges against
	// the same hill_type cannot both transfer the crown. Returns
	// ErrHillNotFound when the hill has never been seeded.
	Challenge(
		ctx context.Context, hillType domain.HillType, userID int64, survivedMs int, newClipID string,
	) (domain.ChallengeOutcome, error)
}

// SessionRepository resolves Redis session keys to user IDs.
type SessionRepository interface {
	// UserIDBySession returns the user_id stored under session:<sessionID>.
	// Returns ErrSessionNotFound when the key is absent or has expired.
	UserIDBySession(ctx context.Context, sessionID string) (int64, error)
}
