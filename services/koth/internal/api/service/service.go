// Package service holds the koth service business logic (orchestrating
// repositories and external integrations). Service interfaces are added here
// by downstream resource slices via the new-resource skill.
package service

//go:generate mockgen -source=service.go -destination=mocks/service_mock.go -package=mocks

import (
	"context"
	"time"

	"github.com/pizdagladki/full/services/koth/internal/api/domain"
)

// Clock is an injectable time source so tests can control wall time (and thus
// which "day" ranked attempts land on).
type Clock interface {
	// Now returns the current time.
	Now() time.Time
}

// RankService is the business-logic contract for the ranked hill.
type RankService interface {
	// SubmitAttempt records a hold-time attempt for userID. Returns
	// domain.ErrInvalidHoldMs when heldMs is not positive. When the achieved
	// rank exceeds the player's current rank for today, the stored rank is
	// updated and AttemptResult.NewlyReached is true.
	SubmitAttempt(ctx context.Context, userID int64, heldMs int) (domain.AttemptResult, error)
	// Me returns the caller's current rank and the next rank's threshold for
	// today. Defaults to rank 0 (target = the rank-1 threshold) when the
	// player has no row today.
	Me(ctx context.Context, userID int64) (domain.MeResult, error)
	// Leaderboard returns the accounts-per-rank distribution for today.
	Leaderboard(ctx context.Context) ([]domain.RankCount, error)
}

// SessionService resolves a session cookie value to a user ID.
type SessionService interface {
	// ResolveSession returns the user_id stored under the session cookie value.
	// Returns the repository.ErrSessionNotFound sentinel when absent/expired.
	ResolveSession(ctx context.Context, sessionID string) (int64, error)
}
