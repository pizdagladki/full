package service

import (
	"context"
	"fmt"
	"strconv"

	"go.uber.org/zap"

	"github.com/pizdagladki/full/services/koth/internal/api/domain"
	"github.com/pizdagladki/full/services/koth/internal/api/repository"
)

// reasonKothWin is the points-ledger credit reason applied when a player
// takes a daily/monthly crown.
const reasonKothWin = "koth_win"

type hillService struct {
	repo      repository.HillRepository
	points    PointsClient
	winAmount int64
	logger    *zap.Logger
}

// NewHillService returns a HillService wired to repo. points credits the
// store's ledger when a challenger takes the crown — a PointsClient failure
// is logged and never fails Challenge. winAmount is the config-driven
// koth_win award amount (always strictly less than PvP's match_win).
func NewHillService(
	repo repository.HillRepository, points PointsClient, winAmount int64, logger *zap.Logger,
) HillService {
	return &hillService{repo: repo, points: points, winAmount: winAmount, logger: logger}
}

func (s *hillService) CurrentKing(ctx context.Context, hillType string) (domain.KingReign, error) {
	ht, err := domain.ParseHillType(hillType)
	if err != nil {
		return domain.KingReign{}, err
	}

	king, err := s.repo.CurrentKing(ctx, ht)
	if err != nil {
		return domain.KingReign{}, err
	}

	return *king, nil
}

func (s *hillService) Challenge(
	ctx context.Context, hillType string, userID int64, survivedMs int, newClipID string,
) (domain.ChallengeOutcome, error) {
	ht, err := domain.ParseHillType(hillType)
	if err != nil {
		return domain.ChallengeOutcome{}, err
	}

	outcome, err := s.repo.Challenge(ctx, ht, userID, survivedMs, newClipID)
	if err != nil {
		return domain.ChallengeOutcome{}, fmt.Errorf("challenge: %w", err)
	}

	if outcome.Won {
		s.creditWin(ctx, userID, outcome.King)
	}

	return outcome, nil
}

// creditWin credits the store's points ledger for a taken crown. RefID is the
// reign's own ID — stable per new reign, so a re-processed identical event
// yields the same ref_id (idempotent by ref_id on the store side). A
// PointsClient failure is logged and swallowed — never propagated — so it
// never blocks the already-recorded crown/challenge outcome.
func (s *hillService) creditWin(ctx context.Context, userID int64, king domain.KingReign) {
	err := s.points.Credit(ctx, CreditRequest{
		UserID: userID,
		Reason: reasonKothWin,
		RefID:  strconv.FormatInt(king.ID, 10),
		Delta:  s.winAmount,
	})
	if err != nil {
		s.logger.Error("credit koth_win points", zap.Int64("user_id", userID), zap.Error(err))
	}
}
