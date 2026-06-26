package service

import (
	"context"
	"errors"

	"go.uber.org/zap"

	"github.com/pizdagladki/full/services/ratings/internal/api/domain"
	"github.com/pizdagladki/full/services/ratings/internal/api/repository"
)

// ErrSamePlayer is returned when winner_id equals loser_id.
// The delivery layer maps this to HTTP 400.
var ErrSamePlayer = errors.New("winner_id and loser_id must be different")

type ratingsService struct {
	repo   repository.RatingsRepository
	logger *zap.Logger
}

// NewRatingsService builds a RatingsService backed by repo.
func NewRatingsService(repo repository.RatingsRepository, logger *zap.Logger) RatingsService {
	return &ratingsService{repo: repo, logger: logger}
}

func (s *ratingsService) ApplyMatchResult(ctx context.Context, input domain.MatchInput) (domain.MatchResult, error) {
	if input.WinnerID == input.LoserID {
		return domain.MatchResult{}, ErrSamePlayer
	}

	result, err := s.repo.ApplyMatchResult(ctx, input)
	if err != nil {
		s.logger.Error("apply match result", zap.Error(err))

		return domain.MatchResult{}, err
	}

	return result, nil
}

func (s *ratingsService) GetRating(ctx context.Context, userID int64) (domain.Rating, error) {
	rating, err := s.repo.GetRating(ctx, userID)
	if err != nil {
		s.logger.Error("get rating", zap.Int64("user_id", userID), zap.Error(err))

		return domain.Rating{}, err
	}

	return rating, nil
}
