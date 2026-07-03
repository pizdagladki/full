// Package config loads and validates the media service configuration.
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"gopkg.in/yaml.v3"
)

// Config is the media service configuration.
type Config struct {
	HTTP      HTTPConfig      `yaml:"http"     validate:"required"`
	Postgres  PostgresConfig  `yaml:"postgres" validate:"required"`
	Storage   StorageConfig   `yaml:"storage"  validate:"required"`
	Redis     RedisConfig     `yaml:"redis"    validate:"required"`
	Session   SessionConfig   `yaml:"session"`
	Clips     ClipsConfig     `yaml:"clips"`
	KingClips KingClipsConfig `yaml:"king_clips"`
}

// HTTPConfig holds the HTTP server settings.
type HTTPConfig struct {
	Addr string `yaml:"addr" validate:"required"`
}

// PostgresConfig holds the Postgres connection settings.
type PostgresConfig struct {
	DSN string `yaml:"dsn" validate:"required"`
}

// StorageConfig holds the MinIO object-storage connection settings.
type StorageConfig struct {
	Endpoint  string `yaml:"endpoint"   validate:"required"`
	AccessKey string `yaml:"access_key" validate:"required"`
	SecretKey string `yaml:"secret_key" validate:"required"`
	Bucket    string `yaml:"bucket"     validate:"required"`
	UseSSL    bool   `yaml:"use_ssl"`
}

// RedisConfig holds the Redis connection settings.
type RedisConfig struct {
	Addr     string `yaml:"addr"     validate:"required"`
	Password string `yaml:"password"`
}

// SessionConfig holds session cookie settings.
type SessionConfig struct {
	CookieName string `yaml:"cookie_name"`
}

// ClipsConfig holds clip-upload limits.
type ClipsConfig struct {
	MaxUploadBytes    int64         `yaml:"max_upload_bytes"`
	DownloadURLTTLRaw string        `yaml:"download_url_ttl"`
	DownloadURLTTL    time.Duration `yaml:"-"`
}

// KingClipsConfig holds the per-hill king-of-the-hill clip term lengths.
// MaxUploadBytes and the download URL TTL are shared with Clips. Term lengths
// are config-driven so ops can retune daily/monthly/ranked windows without a
// code change.
type KingClipsConfig struct {
	DailyTTLRaw   string        `yaml:"daily_ttl"`
	DailyTTL      time.Duration `yaml:"-"`
	MonthlyTTLRaw string        `yaml:"monthly_ttl"`
	MonthlyTTL    time.Duration `yaml:"-"`
	RankedTTLRaw  string        `yaml:"ranked_ttl"`
	RankedTTL     time.Duration `yaml:"-"`
}

const (
	defaultAddr              = ":8082"
	defaultSessionCookieName = "session"
	defaultMaxUploadBytes    = int64(52428800) // 50 MiB
	defaultDownloadURLTTL    = "15m"
	defaultKingDailyTTL      = "24h"
	defaultKingMonthlyTTL    = "720h" // ~30 days
	defaultKingRankedTTL     = "24h"
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
	} else {
		cfg, err = loadFromFile(path)
	}

	if err != nil {
		return nil, err
	}

	err = ValidateConfig(cfg)
	if err != nil {
		return nil, err
	}

	return cfg, nil
}

func loadFromEnv() (*Config, error) {
	maxBytes, err := parseMaxUploadBytes(os.Getenv("MEDIA_MAX_UPLOAD_BYTES"))
	if err != nil {
		return nil, err
	}

	ttlRaw := getEnv("MEDIA_DOWNLOAD_URL_TTL", defaultDownloadURLTTL)

	ttl, err := time.ParseDuration(ttlRaw)
	if err != nil {
		return nil, fmt.Errorf("parse MEDIA_DOWNLOAD_URL_TTL %q: %w", ttlRaw, err)
	}

	dailyRaw := getEnv("MEDIA_KING_DAILY_TTL", defaultKingDailyTTL)

	dailyTTL, err := time.ParseDuration(dailyRaw)
	if err != nil {
		return nil, fmt.Errorf("parse MEDIA_KING_DAILY_TTL %q: %w", dailyRaw, err)
	}

	monthlyRaw := getEnv("MEDIA_KING_MONTHLY_TTL", defaultKingMonthlyTTL)

	monthlyTTL, err := time.ParseDuration(monthlyRaw)
	if err != nil {
		return nil, fmt.Errorf("parse MEDIA_KING_MONTHLY_TTL %q: %w", monthlyRaw, err)
	}

	rankedRaw := getEnv("MEDIA_KING_RANKED_TTL", defaultKingRankedTTL)

	rankedTTL, err := time.ParseDuration(rankedRaw)
	if err != nil {
		return nil, fmt.Errorf("parse MEDIA_KING_RANKED_TTL %q: %w", rankedRaw, err)
	}

	cfg := &Config{
		HTTP: HTTPConfig{Addr: getEnv("HTTP_ADDR", defaultAddr)},
		Postgres: PostgresConfig{
			DSN: os.Getenv("POSTGRES_DSN"),
		},
		Storage: StorageConfig{
			Endpoint:  os.Getenv("STORAGE_ENDPOINT"),
			AccessKey: os.Getenv("STORAGE_ACCESS_KEY"),
			SecretKey: os.Getenv("STORAGE_SECRET_KEY"),
			Bucket:    os.Getenv("STORAGE_BUCKET"),
			UseSSL:    os.Getenv("STORAGE_USE_SSL") == "true",
		},
		Redis: RedisConfig{
			Addr:     os.Getenv("REDIS_ADDR"),
			Password: os.Getenv("REDIS_PASSWORD"),
		},
		Session: SessionConfig{
			CookieName: getEnv("SESSION_COOKIE_NAME", defaultSessionCookieName),
		},
		Clips: ClipsConfig{
			MaxUploadBytes:    maxBytes,
			DownloadURLTTLRaw: ttlRaw,
			DownloadURLTTL:    ttl,
		},
		KingClips: KingClipsConfig{
			DailyTTLRaw:   dailyRaw,
			DailyTTL:      dailyTTL,
			MonthlyTTLRaw: monthlyRaw,
			MonthlyTTL:    monthlyTTL,
			RankedTTLRaw:  rankedRaw,
			RankedTTL:     rankedTTL,
		},
	}

	return cfg, nil
}

func loadFromFile(path string) (*Config, error) {
	b, err := os.ReadFile(path) //nolint:gosec // path is app-controlled (cmd/config.yaml), not user input
	if err != nil {
		return nil, fmt.Errorf("read config %q: %w", path, err)
	}

	cfg := &Config{
		HTTP:    HTTPConfig{Addr: defaultAddr},
		Session: SessionConfig{CookieName: defaultSessionCookieName},
		Clips: ClipsConfig{
			MaxUploadBytes:    defaultMaxUploadBytes,
			DownloadURLTTLRaw: defaultDownloadURLTTL,
		},
		KingClips: KingClipsConfig{
			DailyTTLRaw:   defaultKingDailyTTL,
			MonthlyTTLRaw: defaultKingMonthlyTTL,
			RankedTTLRaw:  defaultKingRankedTTL,
		},
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

	if cfg.Clips.MaxUploadBytes == 0 {
		cfg.Clips.MaxUploadBytes = defaultMaxUploadBytes
	}

	ttlRaw := cfg.Clips.DownloadURLTTLRaw
	if ttlRaw == "" {
		ttlRaw = defaultDownloadURLTTL
		cfg.Clips.DownloadURLTTLRaw = ttlRaw
	}

	ttl, err := time.ParseDuration(ttlRaw)
	if err != nil {
		return nil, fmt.Errorf("parse clips.download_url_ttl %q: %w", ttlRaw, err)
	}

	cfg.Clips.DownloadURLTTL = ttl

	if cfg.KingClips.DailyTTLRaw == "" {
		cfg.KingClips.DailyTTLRaw = defaultKingDailyTTL
	}

	cfg.KingClips.DailyTTL, err = time.ParseDuration(cfg.KingClips.DailyTTLRaw)
	if err != nil {
		return nil, fmt.Errorf("parse king_clips.daily_ttl %q: %w", cfg.KingClips.DailyTTLRaw, err)
	}

	if cfg.KingClips.MonthlyTTLRaw == "" {
		cfg.KingClips.MonthlyTTLRaw = defaultKingMonthlyTTL
	}

	cfg.KingClips.MonthlyTTL, err = time.ParseDuration(cfg.KingClips.MonthlyTTLRaw)
	if err != nil {
		return nil, fmt.Errorf("parse king_clips.monthly_ttl %q: %w", cfg.KingClips.MonthlyTTLRaw, err)
	}

	if cfg.KingClips.RankedTTLRaw == "" {
		cfg.KingClips.RankedTTLRaw = defaultKingRankedTTL
	}

	cfg.KingClips.RankedTTL, err = time.ParseDuration(cfg.KingClips.RankedTTLRaw)
	if err != nil {
		return nil, fmt.Errorf("parse king_clips.ranked_ttl %q: %w", cfg.KingClips.RankedTTLRaw, err)
	}

	return cfg, nil
}

// parseMaxUploadBytes parses the raw value from the environment variable for
// MEDIA_MAX_UPLOAD_BYTES. An empty string returns the default.
func parseMaxUploadBytes(raw string) (int64, error) {
	if raw == "" {
		return defaultMaxUploadBytes, nil
	}

	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse MEDIA_MAX_UPLOAD_BYTES %q: %w", raw, err)
	}

	return v, nil
}

func getEnv(key, def string) string {
	v := os.Getenv(key)
	if v != "" {
		return v
	}

	return def
}
