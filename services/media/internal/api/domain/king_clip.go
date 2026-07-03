package domain

import (
	"errors"
	"fmt"
	"time"
)

// Hill type constants for the king_clips.hill_type column. A "hill" is a
// king-of-the-hill competition scoped to a term (daily/monthly/ranked); each
// hill_type has exactly one current king clip at a time.
const (
	HillTypeDaily   = "daily"
	HillTypeMonthly = "monthly"
	HillTypeRanked  = "ranked"
)

// ErrInvalidHillType is returned when hill_type is not one of the known
// HillType* constants.
var ErrInvalidHillType = errors.New("invalid hill type")

// ErrInvalidBlinkTs is returned when blink_ts_ms is malformed or negative.
var ErrInvalidBlinkTs = errors.New("invalid blink_ts_ms: must be >= 0")

// ValidHillType reports whether s is one of the known hill types.
func ValidHillType(s string) bool {
	switch s {
	case HillTypeDaily, HillTypeMonthly, HillTypeRanked:
		return true
	default:
		return false
	}
}

// KingClip represents a stored king-of-the-hill clip with its metadata. King
// clips are a category separate from win-clips (Clip): they live in their own
// table and object-key prefix, and are NOT subject to the per-user keep-last-10
// FIFO rotation that applies to win-clips. A king clip's lifecycle is tied to
// its hill term via ExpiresAt.
type KingClip struct {
	ID        int64
	UserID    int64
	HillType  string
	ObjectKey string
	BlinkTsMs int64
	CreatedAt time.Time
	ExpiresAt time.Time
}

// BuildKingClipObjectKey returns the MinIO object key for a king clip. The
// "king-clips/" prefix is dedicated to this category and is disjoint from the
// win-clip "clips/" prefix used by BuildObjectKey.
func BuildKingClipObjectKey(hillType string, userID int64, id string) string {
	return fmt.Sprintf("king-clips/%s/%d/%s.webm", hillType, userID, id)
}
