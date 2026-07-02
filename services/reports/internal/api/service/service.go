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

	// BugReportService exposes bug-report business operations.
	BugReportService interface {
		// ReportBug records a bug report. For pc device with a non-empty
		// recording the bytes are stored in object storage first.
		ReportBug(ctx context.Context, userID int64, device, description string, recording []byte) error
	}

	// SessionService resolves a session token to a user ID.
	SessionService interface {
		// ResolveSession returns the user ID for a valid session token.
		ResolveSession(ctx context.Context, sessionID string) (int64, error)
	}

	// TelegramNotifier sends a notification via the Telegram Bot API.
	TelegramNotifier interface {
		// Notify posts a message containing the report details.
		Notify(ctx context.Context, userID int64, device, description, objectRef string) error
	}
)
