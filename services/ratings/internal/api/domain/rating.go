// Package domain holds the ratings service domain models, DTOs, and pure ELO math.
package domain

import (
	"math"
	"time"
)

// Default ELO / level / games constants for a player with no history.
const (
	DefaultELO         = 1000
	DefaultLevel       = 4
	DefaultGamesPlayed = 0
)

// Rating is the persistent player rating record.
type Rating struct {
	UserID      int64
	ELO         int
	Level       int
	GamesPlayed int
}

// MatchInput carries the raw parameters of a finished match.
type MatchInput struct {
	WinnerID   int64
	LoserID    int64
	Mode       string
	DurationMS *int
}

// MatchResult is the outcome returned after applying a match: updated ratings
// and the ELO deltas for each participant.
type MatchResult struct {
	Winner      Rating
	Loser       Rating
	WinnerDelta int
	LoserDelta  int

	// MatchID is the id of the inserted match_results row — the natural match
	// identifier, used as the ref_id when crediting points for this match.
	MatchID int64

	// WinnerLeveledUp is true when the winner's new level band is strictly
	// greater than their level band before the match.
	WinnerLeveledUp bool
}

// MatchHistoryItem is one entry in a player's match history.
type MatchHistoryItem struct {
	MatchID    int64
	OpponentID int64
	Result     string // "win" or "loss"
	Mode       string
	ELODelta   int
	DurationMS *int
	CreatedAt  time.Time
}

// ─── ELO math ────────────────────────────────────────────────────────────────

// kFactor returns the K-factor for a player's CURRENT games_played count
// (before incrementing it for the current match).
//
//	games_played < 20  → 64  (calibration phase)
//	games_played ≥ 20  → 32  (established player)
func kFactor(gamesPlayed int) float64 {
	if gamesPlayed < 20 {
		return 64
	}

	return 32
}

// CalcELODeltas computes the ELO score changes for a single match outcome.
//
// Inputs:
//   - rw, rl      – winner / loser ELO ratings before the match
//   - gwPlayed    – winner's games_played before this match (determines K_w)
//   - glPlayed    – loser's  games_played before this match (determines K_l)
//
// Returns:
//   - winnerDelta – positive integer (full ELO gain)
//   - loserDelta  – negative integer (80 % of the fair loss, loser-softening asymmetry)
func CalcELODeltas(rw, rl, gwPlayed, glPlayed int) (int, int) {
	kw := kFactor(gwPlayed)
	kl := kFactor(glPlayed)

	// Expected score for the winner.
	ew := 1.0 / (1.0 + math.Pow(10, float64(rl-rw)/400.0))
	el := 1.0 - ew

	// Winner receives the full ELO gain.
	winnerDelta := int(math.Round(kw * (1 - ew)))

	// Loser is softened to 80 % of the fair loss (loserDelta is negative).
	loserDelta := int(math.Round(0.8 * kl * (0 - el)))

	return winnerDelta, loserDelta
}

// ─── Level bands ─────────────────────────────────────────────────────────────

// LevelForELO maps an ELO rating to a 10-band level (1 = floor, 10 = elite).
//
// Band table:
//
//	L1  ≤ 500
//	L2   501 –  700
//	L3   701 –  900
//	L4   901 – 1100  (default ELO 1000 → L4)
//	L5  1101 – 1300
//	L6  1301 – 1500
//	L7  1501 – 1700
//	L8  1701 – 1900
//	L9  1901 – 2000
//	L10 ≥ 2001
func LevelForELO(elo int) int {
	switch {
	case elo <= 500:
		return 1
	case elo <= 700:
		return 2
	case elo <= 900:
		return 3
	case elo <= 1100:
		return 4
	case elo <= 1300:
		return 5
	case elo <= 1500:
		return 6
	case elo <= 1700:
		return 7
	case elo <= 1900:
		return 8
	case elo <= 2000:
		return 9
	default:
		return 10
	}
}
