// Package service holds the reports service business logic (orchestrating
// repositories and external integrations). Service interfaces are added here by
// downstream resource slices via the new-resource skill.
package service

//go:generate mockgen -source=service.go -destination=mocks/service_mock.go -package=mocks

import (
	"context"

	"github.com/pizdagladki/full/services/reports/internal/api/domain"
)

type (
	// ReportsService exposes cheat-report business operations.
	ReportsService interface {
		// ReportCheat records a cheat report. Returns ErrSelfReport when
		// reporter_id == reported_id.
		ReportCheat(ctx context.Context, reporterID, reportedID int64, matchID string) error
		// GetCooldown returns the current cooldown state for a user.
		GetCooldown(ctx context.Context, userID int64) (domain.CooldownStatus, error)
	}
)
