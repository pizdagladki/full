// Package service holds the ratings service business logic.
package service

//go:generate mockgen -source=service.go -destination=mocks/service_mock.go -package=mocks

import (
	"context"

	"github.com/pizdagladki/full/services/ratings/internal/api/domain"
)

type (
	// RatingsService orchestrates ELO application and rating retrieval.
	RatingsService interface {
		// ApplyMatchResult validates the input and delegates the transactional
		// update to the repository. Returns 400-class ErrSamePlayer when
		// winner_id == loser_id.
		ApplyMatchResult(ctx context.Context, input domain.MatchInput) (domain.MatchResult, error)

		// GetRating returns the stored rating for userID, or the defaults when
		// the player has no history yet.
		GetRating(ctx context.Context, userID int64) (domain.Rating, error)
	}
)
