package domain

import (
	"errors"
	"time"
)

// ErrInvalidHillType is returned by ParseHillType when the raw value is
// neither "daily" nor "monthly".
var ErrInvalidHillType = errors.New("hill_type must be daily or monthly")

// HillType identifies which king-of-the-hill cadence a reign belongs to.
type HillType string

const (
	// HillTypeDaily is the daily-reset hill.
	HillTypeDaily HillType = "daily"
	// HillTypeMonthly is the monthly-reset hill.
	HillTypeMonthly HillType = "monthly"
)

// ParseHillType validates and converts a raw path param into a HillType.
// Returns ErrInvalidHillType for anything other than "daily"/"monthly".
func ParseHillType(raw string) (HillType, error) {
	switch HillType(raw) {
	case HillTypeDaily:
		return HillTypeDaily, nil
	case HillTypeMonthly:
		return HillTypeMonthly, nil
	default:
		return "", ErrInvalidHillType
	}
}

// KingReign is the domain model for a king_reigns row: a contiguous reign by
// a user over a hill_type, from StartedAt until EndedAt (nil while current).
type KingReign struct {
	ID        int64
	HillType  HillType
	UserID    int64
	ClipID    string
	BlinkTsMs int
	StartedAt time.Time
	EndedAt   *time.Time
}

// ChallengeOutcome is the decided result of a Challenge call: whether the
// challenger became king (Won), plus the resulting current king — either the
// challenger's freshly opened reign, or the unchanged incumbent when the
// challenge fell short.
type ChallengeOutcome struct {
	Won  bool
	King KingReign
}
