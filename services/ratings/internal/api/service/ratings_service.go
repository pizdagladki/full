package service

import (
	"context"
	"errors"
	"strconv"

	"go.uber.org/zap"

	"github.com/pizdagladki/full/services/ratings/internal/api/domain"
	"github.com/pizdagladki/full/services/ratings/internal/api/repository"
)

// ErrSamePlayer is returned when winner_id equals loser_id.
// The delivery layer maps this to HTTP 400.
var ErrSamePlayer = errors.New("winner_id and loser_id must be different")

// Points-ledger credit reasons applied on a successfully-applied match result.
const (
	reasonMatchWin = "match_win"
	reasonLevelUp  = "level_up"
)

type ratingsService struct {
	repo   repository.RatingsRepository
	logger *zap.Logger
	points PointsClient
}

// NewRatingsService builds a RatingsService backed by repo. points credits
// the store's ledger after a match result is applied — a PointsClient
// failure is logged and never fails ApplyMatchResult.
func NewRatingsService(repo repository.RatingsRepository, logger *zap.Logger, points PointsClient) RatingsService {
	return &ratingsService{repo: repo, logger: logger, points: points}
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

	s.creditWinner(ctx, result)

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

func (s *ratingsService) ListMatchHistory(
	ctx context.Context, userID int64, limit, offset int,
) ([]domain.MatchHistoryItem, error) {
	items, err := s.repo.ListMatchHistory(ctx, userID, limit, offset)
	if err != nil {
		s.logger.Error("list match history", zap.Int64("user_id", userID), zap.Error(err))

		return nil, err
	}

	return items, nil
}

// creditWinner credits the store's points ledger for the winner of an
// already-applied match: a match_win credit always, plus a level_up credit
// when the winner's level band increased. Credits are idempotent on the
// store side (deduped by user_id+reason+ref_id), so a failure here is
// swallowed — logged only — and never propagated to the caller.
func (s *ratingsService) creditWinner(ctx context.Context, result domain.MatchResult) {
	matchID := strconv.FormatInt(result.MatchID, 10)

	err := s.points.Credit(ctx, CreditRequest{
		UserID: result.Winner.UserID,
		Reason: reasonMatchWin,
		RefID:  matchID,
	})
	if err != nil {
		s.logger.Error("credit match_win points",
			zap.Int64("winner_id", result.Winner.UserID), zap.Error(err))
	}

	if !result.WinnerLeveledUp {
		return
	}

	err = s.points.Credit(ctx, CreditRequest{
		UserID: result.Winner.UserID,
		Reason: reasonLevelUp,
		RefID:  matchID + ":level",
	})
	if err != nil {
		s.logger.Error("credit level_up points",
			zap.Int64("winner_id", result.Winner.UserID), zap.Error(err))
	}
}
