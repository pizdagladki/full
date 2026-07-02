package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/pizdagladki/full/services/koth/internal/api/domain"
	"github.com/pizdagladki/full/services/koth/internal/api/repository"
)

type rankService struct {
	repo       repository.RankRepository
	clock      Clock
	thresholds []int
}

// NewRankService returns a RankService wired to repo, clock, and the
// ascending rank thresholds (in ms) loaded from config.
func NewRankService(repo repository.RankRepository, clock Clock, thresholds []int) RankService {
	return &rankService{repo: repo, clock: clock, thresholds: thresholds}
}

func (s *rankService) SubmitAttempt(ctx context.Context, userID int64, heldMs int) (domain.AttemptResult, error) {
	if heldMs <= 0 {
		return domain.AttemptResult{}, domain.ErrInvalidHoldMs
	}

	day := s.clock.Now().UTC()

	achieved := domain.ComputeRank(heldMs, s.thresholds)

	currentRank := 0

	existing, err := s.repo.GetRank(ctx, userID, day)
	if err != nil {
		if !errors.Is(err, repository.ErrRankNotFound) {
			return domain.AttemptResult{}, fmt.Errorf("submit attempt: get rank: %w", err)
		}
	} else {
		currentRank = existing.Rank
	}

	newlyReached := false

	if achieved > currentRank {
		err = s.repo.UpsertRank(ctx, userID, day, achieved, heldMs)
		if err != nil {
			return domain.AttemptResult{}, fmt.Errorf("submit attempt: upsert rank: %w", err)
		}

		newlyReached = true
		currentRank = achieved
	}

	return domain.AttemptResult{
		AchievedRank: achieved,
		CurrentRank:  currentRank,
		NewlyReached: newlyReached,
	}, nil
}

func (s *rankService) Me(ctx context.Context, userID int64) (domain.MeResult, error) {
	day := s.clock.Now().UTC()

	currentRank := 0

	existing, err := s.repo.GetRank(ctx, userID, day)
	if err != nil {
		if !errors.Is(err, repository.ErrRankNotFound) {
			return domain.MeResult{}, fmt.Errorf("me: get rank: %w", err)
		}
	} else {
		currentRank = existing.Rank
	}

	return domain.MeResult{
		CurrentRank:  currentRank,
		NextTargetMs: domain.NextTargetMs(currentRank, s.thresholds),
	}, nil
}

func (s *rankService) Leaderboard(ctx context.Context) ([]domain.RankCount, error) {
	day := s.clock.Now().UTC()

	counts, err := s.repo.RankDistribution(ctx, day)
	if err != nil {
		return nil, fmt.Errorf("leaderboard: %w", err)
	}

	return counts, nil
}
