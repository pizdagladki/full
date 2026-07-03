// Package config loads and validates the koth service configuration.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config is the koth service configuration.
type Config struct {
	HTTP     HTTPConfig     `yaml:"http"     validate:"required"`
	Postgres PostgresConfig `yaml:"postgres" validate:"required"`
	Redis    RedisConfig    `yaml:"redis"    validate:"required"`
	Session  SessionConfig  `yaml:"session"`
	Ranked   RankedConfig   `yaml:"ranked"   validate:"required"`
	Store    StoreConfig    `yaml:"store"    validate:"required"`
	Media    MediaConfig    `yaml:"media"    validate:"required"`
	Reset    ResetConfig    `yaml:"reset"    validate:"required"`
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

// StoreConfig holds the store service's base URL, used to POST the
// daily/monthly final-placement reward credit.
type StoreConfig struct {
	BaseURL string `yaml:"base_url" validate:"required"`
}

// MediaConfig holds the media service's base URL, used to DELETE (expire)
// the king clip of a closed reign.
type MediaConfig struct {
	BaseURL string `yaml:"base_url" validate:"required"`
}

// ResetConfig holds the daily/monthly reset worker's poll cadence: the day
// and month boundaries themselves are calendar-based, but how often the
// worker checks for a rolled-over boundary is config-driven.
type ResetConfig struct {
	CheckInterval time.Duration `yaml:"check_interval" validate:"required"`
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
		Store: StoreConfig{
			BaseURL: os.Getenv("STORE_BASE_URL"),
		},
		Media: MediaConfig{
			BaseURL: os.Getenv("MEDIA_BASE_URL"),
		},
		Reset: ResetConfig{
			CheckInterval: parseDuration(os.Getenv("RESET_CHECK_INTERVAL")),
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

// parseDuration parses raw (e.g. "1m") into a time.Duration. An empty or
// malformed input yields the zero Duration, which fails ValidateConfig's
// "required" check and aborts startup rather than silently defaulting the
// reset worker's poll cadence.
func parseDuration(raw string) time.Duration {
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0
	}

	return d
}
