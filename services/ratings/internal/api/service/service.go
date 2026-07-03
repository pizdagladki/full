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

		// ListMatchHistory returns the player's match history, newest first,
		// paginated by limit and offset.
		ListMatchHistory(ctx context.Context, userID int64, limit, offset int) ([]domain.MatchHistoryItem, error)
	}

	// PointsClient credits points into the store's ledger. Implemented by an
	// HTTP client targeting the store service. Credits are idempotent
	// (deduped by user_id+reason+ref_id on the store side), so a failed call
	// here is safe to log and drop — a retry (later) is not lost work.
	PointsClient interface {
		Credit(ctx context.Context, req CreditRequest) error
	}
)

// CreditRequest is the body sent to the store's POST /v1/points/credit.
type CreditRequest struct {
	UserID int64  `json:"user_id"`
	Reason string `json:"reason"`
	RefID  string `json:"ref_id"`
}
