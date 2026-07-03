// Package config loads and validates the signaling service configuration.
package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config is the signaling service configuration.
type Config struct {
	HTTP           HTTPConfig      `yaml:"http"             validate:"required"`
	Redis          RedisConfig     `yaml:"redis"            validate:"required"`
	Signaling      SignalingConfig `yaml:"signaling"`
	RatingsBaseURL string          `yaml:"ratings_base_url" validate:"required"`
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

// signalingRaw is the YAML-decoded form; duration fields are kept as plain strings
// so gopkg.in/yaml.v3 can decode "30m" without error (yaml.v3 treats
// time.Duration as int64, not a duration string).
type signalingRaw struct {
	SessionCookie           string `yaml:"session_cookie"`
	RoomTTLStr              string `yaml:"room_ttl"`
	RoomCodeTTLStr          string `yaml:"room_code_ttl"`
	KeepaliveIntervalStr    string `yaml:"keepalive_interval"`
	KeepalivePingTimeoutStr string `yaml:"keepalive_ping_timeout"`
	ConfirmationBufferStr   string `yaml:"confirmation_buffer"`
}

// SignalingConfig holds signaling relay settings.
type SignalingConfig struct {
	// SessionCookie is the name of the HTTP cookie that carries the session id.
	SessionCookie string
	// RoomTTL is the Redis TTL applied to room member sets on every Join.
	RoomTTL time.Duration
	// RoomCodeTTL is the Redis TTL applied to a private-room invite code
	// (roomcode:<code> → roomID) on creation. Env: SIG_ROOM_CODE_TTL.
	// YAML: room_code_ttl. Default 15m.
	RoomCodeTTL time.Duration
	// KeepaliveInterval is how often the server pings each connected peer.
	// Zero disables keepalives (tests only).
	KeepaliveInterval time.Duration
	// KeepalivePingTimeout is the deadline for each individual Ping.
	KeepalivePingTimeout time.Duration
	// ConfirmationBuffer is the wait period after the first blink/face_lost
	// report before the outcome is finalized. Default 150ms (range 100–200ms).
	// Env: SIG_CONFIRMATION_BUFFER. YAML: confirmation_buffer.
	ConfirmationBuffer time.Duration
}

// rawConfig is the intermediate YAML-decoded config before post-processing.
type rawConfig struct {
	HTTP           HTTPConfig   `yaml:"http"             validate:"required"`
	Redis          RedisConfig  `yaml:"redis"            validate:"required"`
	Signaling      signalingRaw `yaml:"signaling"`
	RatingsBaseURL string       `yaml:"ratings_base_url"`
}

const (
	defaultAddr                 = ":8081"
	defaultSessionCookie        = "session"
	defaultRoomTTL              = 30 * time.Minute
	defaultRoomCodeTTL          = 15 * time.Minute
	defaultKeepaliveInterval    = 30 * time.Second
	defaultKeepalivePingTimeout = 10 * time.Second
	defaultConfirmationBuffer   = 150 * time.Millisecond
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

	applySignalingDefaults(&cfg.Signaling)

	err = ValidateConfig(cfg)
	if err != nil {
		return nil, err
	}

	return cfg, nil
}

func loadFromEnv() *Config {
	roomTTL := defaultRoomTTL

	if v := os.Getenv("SIG_ROOM_TTL"); v != "" {
		d, parseErr := time.ParseDuration(v)
		if parseErr == nil {
			roomTTL = d
		}
	}

	roomCodeTTL := defaultRoomCodeTTL

	if v := os.Getenv("SIG_ROOM_CODE_TTL"); v != "" {
		d, parseErr := time.ParseDuration(v)
		if parseErr == nil {
			roomCodeTTL = d
		}
	}

	keepaliveInterval := defaultKeepaliveInterval

	if v := os.Getenv("SIG_KEEPALIVE_INTERVAL"); v != "" {
		d, parseErr := time.ParseDuration(v)
		if parseErr == nil {
			keepaliveInterval = d
		}
	}

	keepalivePingTimeout := defaultKeepalivePingTimeout

	if v := os.Getenv("SIG_KEEPALIVE_PING_TIMEOUT"); v != "" {
		d, parseErr := time.ParseDuration(v)
		if parseErr == nil {
			keepalivePingTimeout = d
		}
	}

	confirmationBuffer := defaultConfirmationBuffer

	if v := os.Getenv("SIG_CONFIRMATION_BUFFER"); v != "" {
		d, parseErr := time.ParseDuration(v)
		if parseErr == nil {
			confirmationBuffer = d
		}
	}

	return &Config{
		HTTP: HTTPConfig{Addr: getEnv("HTTP_ADDR", defaultAddr)},
		Redis: RedisConfig{
			Addr:     os.Getenv("REDIS_ADDR"),
			Password: os.Getenv("REDIS_PASSWORD"),
		},
		Signaling: SignalingConfig{
			SessionCookie:        getEnv("SIG_SESSION_COOKIE", defaultSessionCookie),
			RoomTTL:              roomTTL,
			RoomCodeTTL:          roomCodeTTL,
			KeepaliveInterval:    keepaliveInterval,
			KeepalivePingTimeout: keepalivePingTimeout,
			ConfirmationBuffer:   confirmationBuffer,
		},
		RatingsBaseURL: os.Getenv("RATINGS_BASE_URL"),
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

	var roomTTL time.Duration

	if raw.Signaling.RoomTTLStr != "" {
		roomTTL, err = time.ParseDuration(raw.Signaling.RoomTTLStr)
		if err != nil {
			return nil, fmt.Errorf("parse signaling.room_ttl %q: %w", raw.Signaling.RoomTTLStr, err)
		}
	}

	var roomCodeTTL time.Duration

	if raw.Signaling.RoomCodeTTLStr != "" {
		roomCodeTTL, err = time.ParseDuration(raw.Signaling.RoomCodeTTLStr)
		if err != nil {
			return nil, fmt.Errorf("parse signaling.room_code_ttl %q: %w", raw.Signaling.RoomCodeTTLStr, err)
		}
	}

	var keepaliveInterval time.Duration

	if raw.Signaling.KeepaliveIntervalStr != "" {
		keepaliveInterval, err = time.ParseDuration(raw.Signaling.KeepaliveIntervalStr)
		if err != nil {
			return nil, fmt.Errorf("parse signaling.keepalive_interval %q: %w", raw.Signaling.KeepaliveIntervalStr, err)
		}
	}

	var keepalivePingTimeout time.Duration

	if raw.Signaling.KeepalivePingTimeoutStr != "" {
		keepalivePingTimeout, err = time.ParseDuration(raw.Signaling.KeepalivePingTimeoutStr)
		if err != nil {
			return nil, fmt.Errorf("parse signaling.keepalive_ping_timeout %q: %w", raw.Signaling.KeepalivePingTimeoutStr, err)
		}
	}

	var confirmationBuffer time.Duration

	if raw.Signaling.ConfirmationBufferStr != "" {
		confirmationBuffer, err = time.ParseDuration(raw.Signaling.ConfirmationBufferStr)
		if err != nil {
			return nil, fmt.Errorf("parse signaling.confirmation_buffer %q: %w", raw.Signaling.ConfirmationBufferStr, err)
		}
	}

	return &Config{
		HTTP:  raw.HTTP,
		Redis: raw.Redis,
		Signaling: SignalingConfig{
			SessionCookie:        raw.Signaling.SessionCookie,
			RoomTTL:              roomTTL,
			RoomCodeTTL:          roomCodeTTL,
			KeepaliveInterval:    keepaliveInterval,
			KeepalivePingTimeout: keepalivePingTimeout,
			ConfirmationBuffer:   confirmationBuffer,
		},
		RatingsBaseURL: raw.RatingsBaseURL,
	}, nil
}

// applySignalingDefaults fills zero values with sane defaults.
func applySignalingDefaults(s *SignalingConfig) {
	if s.SessionCookie == "" {
		s.SessionCookie = defaultSessionCookie
	}

	if s.RoomTTL == 0 {
		s.RoomTTL = defaultRoomTTL
	}

	if s.RoomCodeTTL == 0 {
		s.RoomCodeTTL = defaultRoomCodeTTL
	}

	if s.KeepaliveInterval == 0 {
		s.KeepaliveInterval = defaultKeepaliveInterval
	}

	if s.KeepalivePingTimeout == 0 {
		s.KeepalivePingTimeout = defaultKeepalivePingTimeout
	}

	if s.ConfirmationBuffer == 0 {
		s.ConfirmationBuffer = defaultConfirmationBuffer
	}
}

func getEnv(key, def string) string {
	v := os.Getenv(key)
	if v != "" {
		return v
	}

	return def
}
