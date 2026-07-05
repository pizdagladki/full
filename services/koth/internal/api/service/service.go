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

type (
	// Clock is an injectable time source so tests can control wall time (and
	// thus which "day" ranked attempts land on).
	Clock interface {
		// Now returns the current time.
		Now() time.Time
	}

	// RankService is the business-logic contract for the ranked hill.
	RankService interface {
		// SubmitAttempt records a hold-time attempt for userID. Returns
		// domain.ErrInvalidHoldMs when heldMs is not positive. When the
		// achieved rank exceeds the player's current rank for today, the
		// stored rank is updated and AttemptResult.NewlyReached is true.
		SubmitAttempt(ctx context.Context, userID int64, heldMs int) (domain.AttemptResult, error)
		// Me returns the caller's current rank and the next rank's threshold
		// for today. Defaults to rank 0 (target = the rank-1 threshold) when
		// the player has no row today.
		Me(ctx context.Context, userID int64) (domain.MeResult, error)
		// Leaderboard returns the accounts-per-rank distribution for today.
		Leaderboard(ctx context.Context) ([]domain.RankCount, error)
	}

	// HillService is the business-logic contract for the daily/monthly
	// king-of-the-hill resource.
	HillService interface {
		// CurrentKing parses+validates hillType and returns the current king
		// for it. Returns domain.ErrInvalidHillType for a bad hillType, or
		// the repository.ErrHillNotFound sentinel when the hill needs
		// seeding.
		CurrentKing(ctx context.Context, hillType string) (domain.KingReign, error)
		// Challenge parses+validates hillType, then decides the challenge:
		// the challenger becomes king when survivedMs >= the current king's
		// blink_ts_ms. Returns domain.ErrInvalidHillType for a bad hillType,
		// or the repository.ErrHillNotFound sentinel when the hill needs
		// seeding.
		Challenge(
			ctx context.Context, hillType string, userID int64, survivedMs int, newClipID string,
		) (domain.ChallengeOutcome, error)
	}

	// SessionService resolves a session cookie value to a user ID.
	SessionService interface {
		// ResolveSession returns the user_id stored under the session cookie
		// value. Returns the repository.ErrSessionNotFound sentinel when
		// absent/expired.
		ResolveSession(ctx context.Context, sessionID string) (int64, error)
	}

	// PointsClient credits points into the store's ledger. Implemented by an
	// HTTP client targeting the store service. Credits are idempotent
	// (deduped by user_id+reason+ref_id on the store side), so a failed call
	// here is safe to log and drop — including by the reset job, which
	// treats it as non-blocking.
	PointsClient interface {
		Credit(ctx context.Context, req CreditRequest) error
	}
)

// CreditRequest is the body sent to the store's POST /v1/points/credit.
// Unlike ratings' CreditRequest, koth carries an explicit Delta: the store
// resolves the amount from Reason alone only when Delta is 0, but koth always
// sends its own configured amount so the "less-than-PvP" guard is enforced
// entirely by koth's own config.
type CreditRequest struct {
	UserID int64  `json:"user_id"`
	Reason string `json:"reason"`
	RefID  string `json:"ref_id"`
	Delta  int64  `json:"delta"`
}

// MediaClient expires (deletes) a king clip in the media service once its
// reign has closed. Implemented by an HTTP client targeting the media
// service's king-clip DELETE contract (see media's king_clip_handler.go,
// introduced in #97).
type MediaClient interface {
	ExpireKingClip(ctx context.Context, clipID string) error
}

// ResetService runs the day/month rollover reset for the daily/monthly
// king-of-the-hill reigns, off the request hot path (invoked by a scheduled
// worker — see internal/app/worker_reset.go).
type ResetService interface {
	// CloseStaleReign closes hillType's current reign if the day/month
	// boundary has rolled over past it, credits the final-placement reward,
	// and expires the reign's king clip. A no-op (nil error, no client
	// calls) when there is nothing to close this tick — this is what makes
	// repeated invocations for the same period idempotent.
	CloseStaleReign(ctx context.Context, hillType domain.HillType) error
}
