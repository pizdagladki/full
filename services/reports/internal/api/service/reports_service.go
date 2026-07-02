package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/pizdagladki/full/services/reports/internal/api/domain"
	"github.com/pizdagladki/full/services/reports/internal/api/repository"
)

// ErrSelfReport is returned when a player tries to report themselves.
var ErrSelfReport = errors.New("reporter and reported must be different")

const lastNMatches = 10

type reportsService struct {
	repo        repository.CheatReportsRepository
	cooldown    repository.CooldownStore
	logger      *zap.Logger
	cooldownTTL time.Duration
	threshold   int
}

// NewReportsService constructs a ReportsService backed by the given repository
// and cooldown store. cooldownTTL is the Redis key TTL applied when the
// threshold is reached.
func NewReportsService(
	repo repository.CheatReportsRepository,
	cooldown repository.CooldownStore,
	logger *zap.Logger,
	cooldownTTL time.Duration,
) ReportsService {
	return &reportsService{
		repo:        repo,
		cooldown:    cooldown,
		logger:      logger,
		cooldownTTL: cooldownTTL,
		threshold:   5,
	}
}

// ReportCheat records a cheat report and, when the reported player accumulates
// >= 5 reports across their 10 most recent matches, sets a Redis cooldown key.
func (s *reportsService) ReportCheat(ctx context.Context, reporterID, reportedID int64, matchID string) error {
	// Only check self-report when reporter_id is known (non-zero).
	if reporterID != 0 && reporterID == reportedID {
		return ErrSelfReport
	}

	report := domain.CheatReport{
		ReporterID: reporterID,
		ReportedID: reportedID,
		MatchID:    matchID,
	}

	err := s.repo.UpsertCheatReport(ctx, report)
	if err != nil {
		return fmt.Errorf("upsert cheat report: %w", err)
	}

	count, err := s.repo.CountRecentCheatReports(ctx, reportedID, lastNMatches)
	if err != nil {
		return fmt.Errorf("count recent cheat reports: %w", err)
	}

	if count >= s.threshold {
		err = s.cooldown.SetCooldown(ctx, reportedID, s.cooldownTTL)
		if err != nil {
			// Log but do not fail — the report was recorded successfully.
			s.logger.Error("set cooldown failed", zap.Int64("reported_id", reportedID), zap.Error(err))
		}
	}

	return nil
}

// GetCooldown returns the current cooldown status for a user.
func (s *reportsService) GetCooldown(ctx context.Context, userID int64) (domain.CooldownStatus, error) {
	active, seconds, err := s.cooldown.GetCooldown(ctx, userID)
	if err != nil {
		return domain.CooldownStatus{}, fmt.Errorf("get cooldown: %w", err)
	}

	return domain.CooldownStatus{
		Active:           active,
		SecondsRemaining: seconds,
	}, nil
}
