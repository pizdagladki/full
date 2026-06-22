// Package config loads and validates the auth service configuration.
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"gopkg.in/yaml.v3"
)

// Config is the auth service configuration.
type Config struct {
	HTTP        HTTPConfig        `yaml:"http"        validate:"required"`
	Postgres    PostgresConfig    `yaml:"postgres"    validate:"required"`
	Redis       RedisConfig       `yaml:"redis"       validate:"required"`
	GoogleOAuth GoogleOAuthConfig `yaml:"google_oauth" validate:"required"`
	Session     SessionConfig     `yaml:"session"     validate:"required"`
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

// GoogleOAuthConfig holds the Google OAuth 2.0 client credentials.
type GoogleOAuthConfig struct {
	ClientID     string `yaml:"client_id"     validate:"required"`
	ClientSecret string `yaml:"client_secret"  validate:"required"`
	RedirectURL  string `yaml:"redirect_url"   validate:"required"`
}

// SessionConfig holds the session cookie settings.
type SessionConfig struct {
	// Name is the cookie name (e.g. "session").
	Name string `yaml:"name" validate:"required"`
	// TTL is the session lifetime (e.g. "24h").
	TTL time.Duration `yaml:"ttl" validate:"required"`
	// Secure sets the Secure attribute on the session cookie. Should be true in
	// production (HTTPS) and false for local HTTP dev.
	Secure bool `yaml:"secure"`
}

const defaultAddr = ":8080"

const (
	defaultSessionName = "session"
	defaultSessionTTL  = 24 * time.Hour
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
	sessionTTL := defaultSessionTTL

	if s := os.Getenv("SESSION_TTL"); s != "" {
		sessionTTL = parseSessionTTL(s)
	}

	secure := false
	if s := os.Getenv("SESSION_COOKIE_SECURE"); s == "true" || s == "1" {
		secure = true
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
		GoogleOAuth: GoogleOAuthConfig{
			ClientID:     os.Getenv("GOOGLE_OAUTH_CLIENT_ID"),
			ClientSecret: os.Getenv("GOOGLE_OAUTH_CLIENT_SECRET"),
			RedirectURL:  os.Getenv("GOOGLE_OAUTH_REDIRECT_URL"),
		},
		Session: SessionConfig{
			Name:   getEnv("SESSION_COOKIE_NAME", defaultSessionName),
			TTL:    sessionTTL,
			Secure: secure,
		},
	}
}

func loadFromFile(path string) (*Config, error) {
	b, err := os.ReadFile(path) //nolint:gosec // path is app-controlled (cmd/config.yaml), not user input
	if err != nil {
		return nil, fmt.Errorf("read config %q: %w", path, err)
	}

	cfg := &Config{
		HTTP: HTTPConfig{Addr: defaultAddr},
		Session: SessionConfig{
			Name: defaultSessionName,
			TTL:  defaultSessionTTL,
		},
	}

	err = yaml.Unmarshal(b, cfg)
	if err != nil {
		return nil, fmt.Errorf("parse config %q: %w", path, err)
	}

	if cfg.HTTP.Addr == "" {
		cfg.HTTP.Addr = defaultAddr
	}

	if cfg.Session.Name == "" {
		cfg.Session.Name = defaultSessionName
	}

	if cfg.Session.TTL == 0 {
		cfg.Session.TTL = defaultSessionTTL
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

// parseSessionTTL parses s as either an integer number of seconds or a Go
// duration string (e.g. "24h"). It returns defaultSessionTTL on any error.
func parseSessionTTL(s string) time.Duration {
	// Try integer seconds first (common in Docker .env files).
	secs, err := strconv.ParseInt(s, 10, 64)
	if err == nil {
		return time.Duration(secs) * time.Second
	}

	d, err := time.ParseDuration(s)
	if err == nil {
		return d
	}

	return defaultSessionTTL
}
