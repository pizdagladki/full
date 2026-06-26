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
	Ratings     RatingsConfig     `yaml:"ratings"     validate:"required"`
}

// RatingsConfig holds the ratings service connection settings.
type RatingsConfig struct {
	BaseURL string `yaml:"base_url" validate:"required"`
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

// matchmakingRaw is the YAML-decoded form; FallbackAfterStr is kept as a
// plain string so gopkg.in/yaml.v3 can decode "10s" without error (yaml.v3
// treats time.Duration as int64, not a duration string, so round-tripping
// "10s" through a time.Duration field fails).
type matchmakingRaw struct {
	LevelDistance    int    `yaml:"level_distance"`
	FallbackAfterStr string `yaml:"fallback_after"`
	SessionCookie    string `yaml:"session_cookie"`
}

// MatchmakingConfig holds the pairing algorithm settings after any string
// fields have been parsed to their native types.
type MatchmakingConfig struct {
	// LevelDistance is the maximum level difference D for an in-distance match.
	LevelDistance int
	// FallbackAfter is how long a player waits before the fallback (nearest-
	// regardless) pairing is used.
	FallbackAfter time.Duration
	// SessionCookie is the name of the HTTP cookie that carries the session id.
	SessionCookie string
}

// rawConfig is the intermediate YAML-decoded config before post-processing.
type rawConfig struct {
	HTTP        HTTPConfig     `yaml:"http"        validate:"required"`
	Redis       RedisConfig    `yaml:"redis"       validate:"required"`
	Matchmaking matchmakingRaw `yaml:"matchmaking"`
	Ratings     RatingsConfig  `yaml:"ratings"`
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
		Ratings: RatingsConfig{BaseURL: os.Getenv("RATINGS_BASE_URL")},
	}
}

func loadFromFile(path string) (*Config, error) {
	b, err := os.ReadFile(path) //nolint:gosec // path is app-controlled (cmd/config.yaml), not user input
	if err != nil {
		return nil, fmt.Errorf("read config %q: %w", path, err)
	}

	raw := &rawConfig{HTTP: HTTPConfig{Addr: defaultAddr}}

	err = yaml.Unmarshal(b, raw)
	if err != nil {
		return nil, fmt.Errorf("parse config %q: %w", path, err)
	}

	if raw.HTTP.Addr == "" {
		raw.HTTP.Addr = defaultAddr
	}

	// Parse the duration string from YAML (e.g. "10s").
	var fallbackAfter time.Duration
	if raw.Matchmaking.FallbackAfterStr != "" {
		fallbackAfter, err = time.ParseDuration(raw.Matchmaking.FallbackAfterStr)
		if err != nil {
			return nil, fmt.Errorf("parse matchmaking.fallback_after %q: %w", raw.Matchmaking.FallbackAfterStr, err)
		}
	}

	return &Config{
		HTTP:  raw.HTTP,
		Redis: raw.Redis,
		Matchmaking: MatchmakingConfig{
			LevelDistance: raw.Matchmaking.LevelDistance,
			FallbackAfter: fallbackAfter,
			SessionCookie: raw.Matchmaking.SessionCookie,
		},
		Ratings: raw.Ratings,
	}, nil
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
