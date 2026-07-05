// Package config loads and validates the ratings service configuration.
package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config is the ratings service configuration.
type Config struct {
	HTTP     HTTPConfig     `yaml:"http" validate:"required"`
	Postgres PostgresConfig `yaml:"postgres" validate:"required"`
	Redis    RedisConfig    `yaml:"redis" validate:"required"`
	Store    StoreConfig    `yaml:"store" validate:"required"`
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

// StoreConfig holds the settings for calling the store service, targeted by
// the PointsClient to credit match_win / level_up points.
type StoreConfig struct {
	// BaseURL is the base URL of the store service.
	BaseURL string `yaml:"base_url" validate:"required"`
	// InternalToken authenticates ratings to the store's protected
	// POST /v1/points/credit as "Authorization: Bearer <token>". An unset
	// token is valid config (not required): store fails closed on its side,
	// so an empty token simply results in the store rejecting the request
	// with 401, surfaced as an error by PointsClient.
	InternalToken string `yaml:"internal_token"`
}

const defaultAddr = ":8080"

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
		Store: StoreConfig{
			BaseURL:       os.Getenv("STORE_BASE_URL"),
			InternalToken: os.Getenv("INTERNAL_API_TOKEN"),
		},
	}
}

func loadFromFile(path string) (*Config, error) {
	b, err := os.ReadFile(path) //nolint:gosec // path is app-controlled (cmd/config.yaml), not user input
	if err != nil {
		return nil, fmt.Errorf("read config %q: %w", path, err)
	}

	cfg := &Config{HTTP: HTTPConfig{Addr: defaultAddr}}

	err = yaml.Unmarshal(b, cfg)
	if err != nil {
		return nil, fmt.Errorf("parse config %q: %w", path, err)
	}

	if cfg.HTTP.Addr == "" {
		cfg.HTTP.Addr = defaultAddr
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
