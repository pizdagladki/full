package config

import (
	"errors"

	"github.com/go-playground/validator/v10"
)

// ErrThresholdsNotAscending is returned when Ranked.Thresholds is not
// strictly ascending.
var ErrThresholdsNotAscending = errors.New("ranked.thresholds must be strictly ascending")

// ValidateConfig checks required config fields (HTTP addr, Postgres DSN, Redis
// addr, ranked thresholds), failing fast at startup when any is unset or
// malformed.
func ValidateConfig(cfg *Config) error {
	err := validator.New().Struct(cfg)
	if err != nil {
		return err
	}

	return validateThresholdsAscending(cfg.Ranked.Thresholds)
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
