// Package domain holds the koth service domain models, DTOs, and enums.
package domain

import (
	"errors"
	"time"
)

// ErrInvalidHoldMs is returned when a submitted held_ms is not positive.
var ErrInvalidHoldMs = errors.New("held_ms must be positive")

// HillRank is the domain model for a user's ranked-hill standing on a given
// day (hill_ranks table).
type HillRank struct {
	UserID     int64
	Day        time.Time
	Rank       int
	BestHoldMs int
}

// RankCount is a single bucket of the rank-distribution leaderboard: how many
// accounts sit at Rank today.
type RankCount struct {
	Rank  int
	Count int
}

// AttemptResult is the outcome of submitting a hold-time attempt.
type AttemptResult struct {
	AchievedRank int
	CurrentRank  int
	NewlyReached bool
}

// MeResult is the caller's current standing for today plus the threshold for
// the next rank.
type MeResult struct {
	CurrentRank  int
	NextTargetMs int
}

// ComputeRank maps a held-time (in ms) to the rank achieved given ascending
// thresholds. thresholds must be sorted ascending. The achieved rank is the
// count of thresholds that are <= heldMs: holding at least thresholds[0]
// yields rank 1, at least thresholds[1] yields rank 2, and so on. Holding
// less than thresholds[0] yields rank 0.
func ComputeRank(heldMs int, thresholds []int) int {
	rank := 0

	for _, threshold := range thresholds {
		if heldMs >= threshold {
			rank++
		} else {
			break
		}
	}

	return rank
}

// NextTargetMs returns the threshold (in ms) the player needs to reach to
// advance from currentRank to currentRank+1. When currentRank is already at
// or beyond the highest configured rank, there is no higher target and
// NextTargetMs returns 0.
func NextTargetMs(currentRank int, thresholds []int) int {
	if currentRank < 0 || currentRank >= len(thresholds) {
		return 0
	}

	return thresholds[currentRank]
}
