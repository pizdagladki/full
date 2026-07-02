// Package config loads and validates the koth service configuration.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config is the koth service configuration.
type Config struct {
	HTTP     HTTPConfig     `yaml:"http"     validate:"required"`
	Postgres PostgresConfig `yaml:"postgres" validate:"required"`
	Redis    RedisConfig    `yaml:"redis"    validate:"required"`
	Session  SessionConfig  `yaml:"session"`
	Ranked   RankedConfig   `yaml:"ranked"   validate:"required"`
}

// HTTPConfig holds the HTTP server settings.
type HTTPConfig struct {
	Addr string `yaml:"addr" validate:"required"`
}

// PostgresConfig holds the Postgres connection settings.
type PostgresConfig struct {
	DSN string `yaml:"dsn" validate:"required"`
}

// RedisConfig holds the Redis connection settings.
type RedisConfig struct {
	Addr     string `yaml:"addr" validate:"required"`
	Password string `yaml:"password"`
}

// SessionConfig holds session cookie settings.
type SessionConfig struct {
	CookieName string `yaml:"cookie_name"`
}

// RankedConfig holds the ranked-hill hold-time rank thresholds (in
// milliseconds), config-driven placeholders rather than hardcoded values.
// Thresholds must be non-empty and strictly ascending — see validate.go.
type RankedConfig struct {
	Thresholds []int `yaml:"thresholds" validate:"required,min=1"`
}

const (
	defaultAddr              = ":8080"
	defaultSessionCookieName = "session"
)

// Load reads the config from environment variables when IS_DOCKER is set,
// otherwise from the YAML file at path, then validates it.
func Load(path string) (*Config, error) {
	var (
		cfg *Config
		err error
	)

	if os.Getenv("IS_DOCKER") != "" {
		cfg = loadFromEnv()
	} else {
		cfg, err = loadFromFile(path)
		if err != nil {
			return nil, err
		}
	}

	err = ValidateConfig(cfg)
	if err != nil {
		return nil, err
	}

	return cfg, nil
}

func loadFromEnv() *Config {
	return &Config{
		HTTP: HTTPConfig{Addr: getEnv("HTTP_ADDR", defaultAddr)},
		Postgres: PostgresConfig{
			DSN: os.Getenv("POSTGRES_DSN"),
		},
		Redis: RedisConfig{
			Addr:     os.Getenv("REDIS_ADDR"),
			Password: os.Getenv("REDIS_PASSWORD"),
		},
		Session: SessionConfig{
			CookieName: getEnv("SESSION_COOKIE_NAME", defaultSessionCookieName),
		},
		Ranked: RankedConfig{
			Thresholds: parseThresholds(os.Getenv("RANKED_THRESHOLDS_MS")),
		},
	}
}

func loadFromFile(path string) (*Config, error) {
	b, err := os.ReadFile(path) //nolint:gosec // path is app-controlled (cmd/config.yaml), not user input
	if err != nil {
		return nil, fmt.Errorf("read config %q: %w", path, err)
	}

	cfg := &Config{
		HTTP:    HTTPConfig{Addr: defaultAddr},
		Session: SessionConfig{CookieName: defaultSessionCookieName},
	}

	err = yaml.Unmarshal(b, cfg)
	if err != nil {
		return nil, fmt.Errorf("parse config %q: %w", path, err)
	}

	if cfg.HTTP.Addr == "" {
		cfg.HTTP.Addr = defaultAddr
	}

	if cfg.Session.CookieName == "" {
		cfg.Session.CookieName = defaultSessionCookieName
	}

	return cfg, nil
}

func getEnv(key, def string) string {
	v := os.Getenv(key)
	if v != "" {
		return v
	}

	return def
}

// parseThresholds parses a comma-separated list of ascending millisecond
// thresholds (e.g. "5000,15000,30000"). Malformed or empty entries are
// skipped; an empty/unset input yields an empty (invalid) slice, which fails
// ValidateConfig and aborts startup rather than silently hardcoding ranks.
func parseThresholds(raw string) []int {
	if raw == "" {
		return nil
	}

	parts := strings.Split(raw, ",")
	thresholds := make([]int, 0, len(parts))

	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}

		v, err := strconv.Atoi(p)
		if err != nil {
			continue
		}

		thresholds = append(thresholds, v)
	}

	return thresholds
}
