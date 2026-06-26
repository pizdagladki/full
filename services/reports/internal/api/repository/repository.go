// Package repository holds the reports service data access (hand-written SQL
// via pgx, mapping rows to domain models). Repository interfaces are added here
// by downstream resource slices via the new-resource skill.
package repository

//go:generate mockgen -source=repository.go -destination=mocks/repository_mock.go -package=mocks

import (
	"context"
	"errors"
	"time"

	"github.com/pizdagladki/full/services/reports/internal/api/domain"
)

// ErrSessionNotFound is returned when a session key is absent or expired.
var ErrSessionNotFound = errors.New("session: not found or expired")

type (
	// CheatReportsRepository provides access to cheat_reports table.
	CheatReportsRepository interface {
		// UpsertCheatReport inserts a cheat report. If a report with the same
		// (reporter_id, match_id) already exists it does nothing (idempotent).
		UpsertCheatReport(ctx context.Context, report domain.CheatReport) error
		// CountRecentCheatReports counts how many distinct cheat reports the
		// reported user has across their lastNMatches most recent distinct matches.
		CountRecentCheatReports(ctx context.Context, reportedID int64, lastNMatches int) (int, error)
	}

	// CooldownStore manages anti-cheat cooldown flags in Redis.
	CooldownStore interface {
		// SetCooldown writes a cooldown key for the user with the given TTL.
		SetCooldown(ctx context.Context, userID int64, ttl time.Duration) error
		// GetCooldown returns whether a cooldown is active and how many seconds
		// remain on it. Returns active=false when the key is absent or has no TTL.
		GetCooldown(ctx context.Context, userID int64) (active bool, secondsRemaining int, err error)
	}

	// BugReportsRepository provides access to bug_reports table.
	BugReportsRepository interface {
		// InsertBugReport inserts a bug report row.
		InsertBugReport(ctx context.Context, report domain.BugReport) error
	}

	// SessionRepository resolves authenticated sessions stored in Redis.
	SessionRepository interface {
		// UserIDBySession looks up the user ID for a session token.
		// Returns ErrSessionNotFound when the key is absent or expired.
		UserIDBySession(ctx context.Context, sessionID string) (int64, error)
	}

	// BugRecordingStorage stores bug-report recordings in object storage.
	BugRecordingStorage interface {
		// StoreRecording writes data under key in the configured bucket.
		StoreRecording(ctx context.Context, key string, data []byte) error
	}
)
