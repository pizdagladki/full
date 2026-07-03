package service

import (
	"context"
	"errors"
	"fmt"

	"go.uber.org/zap"

	"github.com/pizdagladki/full/services/store/internal/api/domain"
	"github.com/pizdagladki/full/services/store/internal/api/repository"
)

type rewardedService struct {
	repo    repository.RewardedRepository
	limiter repository.RewardedRateLimiter
	logger  *zap.Logger
}

// NewRewardedService returns a RewardedService wired to repo and limiter.
func NewRewardedService(
	repo repository.RewardedRepository,
	limiter repository.RewardedRateLimiter,
	logger *zap.Logger,
) RewardedService {
	return &rewardedService{repo: repo, limiter: limiter, logger: logger}
}

// GrantFreeDistraction fetches the product, checks it is a free distraction,
// checks the rate limit, then grants one unit into the user's inventory.
func (s *rewardedService) GrantFreeDistraction(ctx context.Context, userID, productID int64) (int, error) {
	product, err := s.repo.GetProduct(ctx, productID)
	if err != nil {
		if errors.Is(err, domain.ErrProductNotFound) {
			return 0, domain.ErrProductNotFound
		}

		return 0, fmt.Errorf("get product: %w", err)
	}

	if product.Kind != domain.KindDistraction || !product.IsFree {
		return 0, domain.ErrNotGrantable
	}

	allowed, err := s.limiter.Allow(ctx, userID)
	if err != nil {
		return 0, fmt.Errorf("check rate limit: %w", err)
	}

	if !allowed {
		return 0, domain.ErrRateLimited
	}

	quantity, err := s.repo.GrantFreeDistraction(ctx, userID, productID)
	if err != nil {
		return 0, fmt.Errorf("grant free distraction: %w", err)
	}

	return quantity, nil
}
