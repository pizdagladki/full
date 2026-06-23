// Package domain holds the matchmaking service domain models, DTOs, and pure
// decision functions. No I/O — only in-memory types and logic.
package domain

import (
	"errors"
	"time"
)

// Sentinel validation errors.
var (
	ErrInvalidLevel = errors.New("level must be between 1 and 10")
	ErrInvalidMode  = errors.New("mode must be a non-empty string of at most 64 characters")
	ErrUnknownType  = errors.New("unknown message type")
)

const (
	maxModeLen = 64
	minLevel   = 1
	maxLevel   = 10
)

// Player represents a waiting queue entry.
type Player struct {
	UserID     int64
	Mode       string
	Level      int
	EnqueuedAt time.Time
}

// InboundMessage is the discriminator envelope for client→server WS messages.
type InboundMessage struct {
	Type  string `json:"type"`
	Mode  string `json:"mode,omitempty"`
	Level int    `json:"level,omitempty"`
}

// MatchedMessage is the server→client notification on successful pairing.
type MatchedMessage struct {
	Type     string `json:"type"`
	RoomID   string `json:"room_id"`
	Opponent int64  `json:"opponent"`
}

// ValidateLevel returns ErrInvalidLevel when level is out of [1,10].
func ValidateLevel(level int) error {
	if level < minLevel || level > maxLevel {
		return ErrInvalidLevel
	}

	return nil
}

// ValidateMode returns ErrInvalidMode when mode is empty or exceeds maxModeLen.
func ValidateMode(mode string) error {
	if mode == "" || len(mode) > maxModeLen {
		return ErrInvalidMode
	}

	return nil
}

// ValidateJoin validates both mode and level together.
func ValidateJoin(mode string, level int) error {
	err := ValidateMode(mode)
	if err != nil {
		return err
	}

	return ValidateLevel(level)
}

// NearestWithinDistance returns the best matching opponent for candidate from
// waiting, where "best" means the smallest |level difference| that is <= maxDist.
//
// Tie-break: earliest EnqueuedAt; then lowest UserID (deterministic).
// Returns nil when no player is within maxDist.
func NearestWithinDistance(candidate Player, waiting []Player, maxDist int) *Player {
	var best *Player

	for i := range waiting {
		w := &waiting[i]
		if w.UserID == candidate.UserID {
			continue
		}

		diff := abs(candidate.Level - w.Level)
		if diff > maxDist {
			continue
		}

		if best == nil || isBetter(w, best, candidate) {
			best = w
		}
	}

	return best
}

// NearestRegardless returns the closest opponent by level (ignoring distance),
// applying the same tie-break. Returns nil when waiting is empty or contains
// only the candidate itself.
func NearestRegardless(candidate Player, waiting []Player) *Player {
	var best *Player

	for i := range waiting {
		w := &waiting[i]
		if w.UserID == candidate.UserID {
			continue
		}

		if best == nil || isBetter(w, best, candidate) {
			best = w
		}
	}

	return best
}

// PastFallbackDeadline reports whether a player that joined at enqueuedAt has
// been waiting longer than fallbackAfter as of now.
func PastFallbackDeadline(now, enqueuedAt time.Time, fallbackAfter time.Duration) bool {
	return now.Sub(enqueuedAt) >= fallbackAfter
}

// isBetter reports whether challenger is a better opponent for candidate than
// current. "Better" means a smaller level distance, then earlier EnqueuedAt,
// then lower UserID.
func isBetter(challenger, current *Player, candidate Player) bool {
	diffC := abs(candidate.Level - challenger.Level)
	diffB := abs(candidate.Level - current.Level)

	if diffC != diffB {
		return diffC < diffB
	}

	if !challenger.EnqueuedAt.Equal(current.EnqueuedAt) {
		return challenger.EnqueuedAt.Before(current.EnqueuedAt)
	}

	return challenger.UserID < current.UserID
}

func abs(x int) int {
	if x < 0 {
		return -x
	}

	return x
}
