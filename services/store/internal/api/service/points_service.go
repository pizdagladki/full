package service

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/pizdagladki/full/services/store/internal/api/domain"
	"github.com/pizdagladki/full/services/store/internal/api/repository"
)

type pointsService struct {
	repo    repository.PointsRepository
	cache   repository.PointsCache
	amounts map[string]int64
	logger  *zap.Logger
}

// NewPointsService returns a PointsService wired to repo, cache, the
// config-driven per-reason point amounts, and logger.
func NewPointsService(
	repo repository.PointsRepository,
	cache repository.PointsCache,
	amounts map[string]int64,
	logger *zap.Logger,
) PointsService {
	return &pointsService{repo: repo, cache: cache, amounts: amounts, logger: logger}
}

func (s *pointsService) Credit(ctx context.Context, in domain.PointsCredit) (int64, error) {
	delta := in.Delta
	if delta <= 0 {
		delta = s.amounts[in.Reason]
	}

	if in.Reason == "" || delta <= 0 {
		return 0, domain.ErrInvalidCredit
	}

	balance, _, err := s.repo.Credit(ctx, in.UserID, delta, in.Reason, in.RefID)
	if err != nil {
		return 0, fmt.Errorf("credit points: %w", err)
	}

	err = s.cache.SetBalance(ctx, in.UserID, balance)
	if err != nil {
		// Postgres remains the source of truth; a cache write failure must
		// not fail the credit.
		s.logger.Warn("set cached points balance after credit", zap.Int64("user_id", in.UserID), zap.Error(err))
	}

	return balance, nil
}

func (s *pointsService) GetBalance(ctx context.Context, userID int64) (int64, error) {
	balance, found, err := s.cache.GetBalance(ctx, userID)
	if err != nil {
		s.logger.Warn("get cached points balance", zap.Int64("user_id", userID), zap.Error(err))
	} else if found {
		return balance, nil
	}

	balance, err = s.repo.GetBalance(ctx, userID)
	if err != nil {
		return 0, fmt.Errorf("get points balance: %w", err)
	}

	setErr := s.cache.SetBalance(ctx, userID, balance)
	if setErr != nil {
		s.logger.Warn("set cached points balance after read", zap.Int64("user_id", userID), zap.Error(setErr))
	}

	return balance, nil
}
