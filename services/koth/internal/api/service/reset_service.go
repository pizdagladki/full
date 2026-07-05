package service

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/pizdagladki/full/services/koth/internal/api/domain"
	"github.com/pizdagladki/full/services/koth/internal/api/repository"
)

const kingRefDateLayout = "2006-01-02"

type resetService struct {
	repo   repository.HillRepository
	clock  Clock
	points PointsClient
	media  MediaClient
	logger *zap.Logger
}

// NewResetService returns a ResetService wired to repo, clock, and the
// points/media clients used for the final-placement reward and king-clip
// expiry.
func NewResetService(
	repo repository.HillRepository, clock Clock, points PointsClient, media MediaClient, logger *zap.Logger,
) ResetService {
	return &resetService{repo: repo, clock: clock, points: points, media: media, logger: logger}
}

func (s *resetService) CloseStaleReign(ctx context.Context, hillType domain.HillType) error {
	periodStart := domain.PeriodStart(hillType, s.clock.Now())

	closed, err := s.repo.CloseIfStale(ctx, hillType, periodStart)
	if err != nil {
		return fmt.Errorf("close stale reign: %w", err)
	}

	if closed == nil {
		// Nothing to close this tick — already reset (or freshly seeded) for
		// this period. This is what makes repeated invocations idempotent.
		return nil
	}

	reason := domain.ReasonKothDailyFinal
	if hillType == domain.HillTypeMonthly {
		reason = domain.ReasonKothMonthlyFinal
	}

	refID := fmt.Sprintf("%s:%s", hillType, periodStart.Format(kingRefDateLayout))

	err = s.points.Credit(ctx, CreditRequest{UserID: closed.UserID, Reason: reason, RefID: refID})
	if err != nil {
		// Non-blocking: the credit is logged and dropped rather than failing
		// the reset — the reign must still close and the clip must still
		// expire even if the store is unreachable (mirrors the ratings
		// PointsClient convention: log-on-error, do not block the outcome).
		s.logger.Warn("koth reset: credit final-placement reward failed",
			zap.String("hill_type", string(hillType)), zap.Int64("user_id", closed.UserID),
			zap.String("reason", reason), zap.Error(err))
	}

	if closed.ClipID != "" {
		err = s.media.ExpireKingClip(ctx, closed.ClipID)
		if err != nil {
			s.logger.Warn("koth reset: expire king clip failed",
				zap.String("hill_type", string(hillType)), zap.String("clip_id", closed.ClipID), zap.Error(err))
		}
	}

	return nil
}
