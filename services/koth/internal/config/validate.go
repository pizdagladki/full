package config

import (
	"errors"

	"github.com/go-playground/validator/v10"
)

// ErrThresholdsNotAscending is returned when Ranked.Thresholds is not
// strictly ascending.
var ErrThresholdsNotAscending = errors.New("ranked.thresholds must be strictly ascending")

// ErrPointsAmountNotLessThanPvP is returned when a configured KotH award
// amount (win or rank) is not strictly less than the PvP match_win amount —
// KotH points must always stay below PvP, per spec.
var ErrPointsAmountNotLessThanPvP = errors.New(
	"points.win_amount and points.rank_amount must be strictly less than points.pvp_win_amount",
)

// ValidateConfig checks required config fields (HTTP addr, Postgres DSN, Redis
// addr, ranked thresholds, points amounts), failing fast at startup when any
// is unset or malformed.
func ValidateConfig(cfg *Config) error {
	err := validator.New().Struct(cfg)
	if err != nil {
		return err
	}

	err = validateThresholdsAscending(cfg.Ranked.Thresholds)
	if err != nil {
		return err
	}

	return validatePointsLessThanPvP(cfg.Points)
}

// validatePointsLessThanPvP reports ErrPointsAmountNotLessThanPvP unless both
// the win and rank award amounts are strictly less than the PvP match_win
// amount.
func validatePointsLessThanPvP(points PointsConfig) error {
	if points.WinAmount >= points.PvPWinAmount || points.RankAmount >= points.PvPWinAmount {
		return ErrPointsAmountNotLessThanPvP
	}

	return nil
}

// validateThresholdsAscending reports an error unless thresholds is strictly
// ascending (each entry greater than the previous one).
func validateThresholdsAscending(thresholds []int) error {
	for i := 1; i < len(thresholds); i++ {
		if thresholds[i] <= thresholds[i-1] {
			return ErrThresholdsNotAscending
		}
	}

	return nil
}
