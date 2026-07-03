// Package config loads and validates the store service configuration.
package config

import (
	"encoding/json"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config is the store service configuration.
type Config struct {
	HTTP     HTTPConfig     `yaml:"http" validate:"required"`
	Postgres PostgresConfig `yaml:"postgres" validate:"required"`
	Redis    RedisConfig    `yaml:"redis" validate:"required"`
	Session  SessionConfig  `yaml:"session"`
	Stripe   StripeConfig   `yaml:"stripe" validate:"required"`
	Points   PointsConfig   `yaml:"points"`
}

// PointsConfig holds the config-driven points-per-reason amounts (e.g.
// match_win, level_up). These are placeholders — extend the map as new
// earning reasons are added, without hardcoding amounts in Go.
type PointsConfig struct {
	Amounts map[string]int64 `yaml:"amounts"`
}

// StripeConfig holds Stripe API credentials and webhook settings.
type StripeConfig struct {
	SecretKey            string `yaml:"secret_key" validate:"required"`
	WebhookSigningSecret string `yaml:"webhook_signing_secret" validate:"required"`
}

// SessionConfig holds session cookie settings.
type SessionConfig struct {
	CookieName string `yaml:"cookie_name"`
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

const (
	defaultAddr              = ":8083"
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
		cfg, err = loadFromEnv()
		if err != nil {
			return nil, err
		}
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

// pointsAmountsEnvVar is the name of the env var carrying the points-per-reason
// amounts as a JSON object, e.g. POINTS_AMOUNTS={"match_win":10,"level_up":25}.
// Unset/empty is valid (an empty map — Credit then resolves any config-driven
// reason to a non-positive delta and returns ErrInvalidCredit unless the
// caller passes an explicit positive delta); malformed JSON fails config load.
const pointsAmountsEnvVar = "POINTS_AMOUNTS"

func loadFromEnv() (*Config, error) {
	amounts, err := parsePointsAmountsEnv(os.Getenv(pointsAmountsEnvVar))
	if err != nil {
		return nil, err
	}

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
		Stripe: StripeConfig{
			SecretKey:            os.Getenv("STRIPE_SECRET_KEY"),
			WebhookSigningSecret: os.Getenv("STRIPE_WEBHOOK_SIGNING_SECRET"),
		},
		Points: PointsConfig{
			Amounts: amounts,
		},
	}, nil
}

// parsePointsAmountsEnv parses the POINTS_AMOUNTS env var (a JSON object of
// reason -> amount, e.g. {"match_win":10,"level_up":25}). An empty string
// returns an empty (non-nil) map — points amounts are config-driven, not
// hardcoded in Go, and an absent config simply means no reason resolves to a
// positive amount until configured. Malformed JSON is a config load error.
func parsePointsAmountsEnv(raw string) (map[string]int64, error) {
	if raw == "" {
		return map[string]int64{}, nil
	}

	amounts := make(map[string]int64)

	err := json.Unmarshal([]byte(raw), &amounts)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", pointsAmountsEnvVar, err)
	}

	return amounts, nil
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
