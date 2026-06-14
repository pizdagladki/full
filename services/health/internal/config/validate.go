package config

import "github.com/go-playground/validator/v10"

// Validate checks required config fields, failing fast at startup.
func Validate(cfg *Config) error {
	return validator.New().Struct(cfg)
}
