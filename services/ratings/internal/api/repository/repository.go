// Package repository holds the ratings service data access (hand-written SQL via
// pgx, mapping rows to domain models).
package repository

//go:generate mockgen -source=repository.go -destination=mocks/repository_mock.go -package=mocks

import (
	"context"

	"github.com/pizdagladki/full/services/ratings/internal/api/domain"
)

type (
	// RatingsRepository performs all data access for player ratings and match results.
	RatingsRepository interface {
		// ApplyMatchResult runs the full match-application transaction atomically:
		// materialize default rows for winner and loser if absent, lock them,
		// compute ELO deltas, update both ratings, insert a match_results row.
		// Returns the updated ratings and the ELO deltas for each participant.
		ApplyMatchResult(ctx context.Context, input domain.MatchInput) (domain.MatchResult, error)

		// GetRating returns the persisted rating for userID, or the defaults
		// (ELO=1000, Level=4, GamesPlayed=0) when the player has no row yet.
		// A missing row is NOT an error; it does NOT insert anything.
		GetRating(ctx context.Context, userID int64) (domain.Rating, error)

		// ListMatchHistory returns the player's match history, newest first,
		// paginated by limit and offset.
		ListMatchHistory(ctx context.Context, userID int64, limit, offset int) ([]domain.MatchHistoryItem, error)
	}
)
