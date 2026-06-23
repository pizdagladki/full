// Package config loads and validates the matchmaking service configuration.
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"gopkg.in/yaml.v3"
)

// Config is the matchmaking service configuration.
type Config struct {
	HTTP        HTTPConfig        `yaml:"http"        validate:"required"`
	Redis       RedisConfig       `yaml:"redis"       validate:"required"`
	Matchmaking MatchmakingConfig `yaml:"matchmaking"`
}

// HTTPConfig holds the HTTP server settings.
type HTTPConfig struct {
	Addr string `yaml:"addr" validate:"required"`
}

// RedisConfig holds the Redis connection settings.
type RedisConfig struct {
	Addr     string `yaml:"addr"     validate:"required"`
	Password string `yaml:"password"`
}

// MatchmakingConfig holds the pairing algorithm settings.
type MatchmakingConfig struct {
	// LevelDistance is the maximum level difference D for an in-distance match.
	LevelDistance int `yaml:"level_distance"`
	// FallbackAfter is how long a player waits before the fallback (nearest-
	// regardless) pairing is used.
	FallbackAfter time.Duration `yaml:"fallback_after"`
	// SessionCookie is the name of the HTTP cookie that carries the session id.
	SessionCookie string `yaml:"session_cookie"`
}

const (
	defaultAddr          = ":8080"
	defaultLevelDist     = 3
	defaultFallbackAfter = 10 * time.Second
	defaultSessionCookie = "session"
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

	applyMatchmakingDefaults(&cfg.Matchmaking)

	err = ValidateConfig(cfg)
	if err != nil {
		return nil, err
	}

	return cfg, nil
}

func loadFromEnv() *Config {
	levelDist := defaultLevelDist
	if v := os.Getenv("MM_LEVEL_DISTANCE"); v != "" {
		n, atoiErr := strconv.Atoi(v)
		if atoiErr == nil {
			levelDist = n
		}
	}

	fallbackAfter := defaultFallbackAfter
	if v := os.Getenv("MM_FALLBACK_AFTER"); v != "" {
		d, parseErr := time.ParseDuration(v)
		if parseErr == nil {
			fallbackAfter = d
		}
	}

	sessionCookie := getEnv("MM_SESSION_COOKIE", defaultSessionCookie)

	return &Config{
		HTTP: HTTPConfig{Addr: getEnv("HTTP_ADDR", defaultAddr)},
		Redis: RedisConfig{
			Addr:     os.Getenv("REDIS_ADDR"),
			Password: os.Getenv("REDIS_PASSWORD"),
		},
		Matchmaking: MatchmakingConfig{
			LevelDistance: levelDist,
			FallbackAfter: fallbackAfter,
			SessionCookie: sessionCookie,
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

// applyMatchmakingDefaults fills zero values with sane defaults.
func applyMatchmakingDefaults(m *MatchmakingConfig) {
	if m.LevelDistance == 0 {
		m.LevelDistance = defaultLevelDist
	}

	if m.FallbackAfter == 0 {
		m.FallbackAfter = defaultFallbackAfter
	}

	if m.SessionCookie == "" {
		m.SessionCookie = defaultSessionCookie
	}
}

func getEnv(key, def string) string {
	v := os.Getenv(key)
	if v != "" {
		return v
	}

	return def
}
