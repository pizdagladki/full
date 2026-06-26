package config

import "github.com/go-playground/validator/v10"

// ValidateConfig checks required config fields (HTTP addr, Postgres DSN,
// Redis addr, and all required MinIO storage fields), failing fast at startup
// when any is unset.
func ValidateConfig(cfg *Config) error {
	return validator.New().Struct(cfg)
}
