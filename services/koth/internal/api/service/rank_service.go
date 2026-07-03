package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/pizdagladki/full/services/koth/internal/api/domain"
	"github.com/pizdagladki/full/services/koth/internal/api/repository"
)

// reasonKothRank is the points-ledger credit reason applied when a player
// reaches a newly-reached ranked-hill rank.
const reasonKothRank = "koth_rank"

type rankService struct {
	repo       repository.RankRepository
	clock      Clock
	thresholds []int
	points     PointsClient
	rankAmount int64
	logger     *zap.Logger
}

// NewRankService returns a RankService wired to repo, clock, and the
// ascending rank thresholds (in ms) loaded from config. points credits the
// store's ledger when a player reaches a newly-reached rank — a PointsClient
// failure is logged and never fails SubmitAttempt. rankAmount is the
// config-driven koth_rank award amount (always strictly less than PvP's
// match_win).
func NewRankService(
	repo repository.RankRepository, clock Clock, thresholds []int,
	points PointsClient, rankAmount int64, logger *zap.Logger,
) RankService {
	return &rankService{
		repo: repo, clock: clock, thresholds: thresholds,
		points: points, rankAmount: rankAmount, logger: logger,
	}
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

		s.creditRankUp(ctx, userID, day, achieved)
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

// creditRankUp credits the store's points ledger for a newly-reached rank.
// RefID is a day+achieved-rank composite — stable for the same user+day+rank
// so a re-processed identical event yields the same ref_id; it does not need
// to bake in userID since the store's dedup key is (user_id, reason,
// ref_id). A PointsClient failure is logged and swallowed — never
// propagated — so it never blocks the already-recorded rank-up.
func (s *rankService) creditRankUp(ctx context.Context, userID int64, day time.Time, achieved int) {
	err := s.points.Credit(ctx, CreditRequest{
		UserID: userID,
		Reason: reasonKothRank,
		RefID:  fmt.Sprintf("%s:%d", day.Format("2006-01-02"), achieved),
		Delta:  s.rankAmount,
	})
	if err != nil {
		s.logger.Error("credit koth_rank points", zap.Int64("user_id", userID), zap.Error(err))
	}
}
