package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoad(t *testing.T) {
	const (
		validDSN       = "postgres://app:app@localhost:5432/app?sslmode=disable"
		validEndpoint  = "localhost:9000"
		validKey       = "minioadmin"
		validBucket    = "media"
		validRedisAddr = "localhost:6379"
	)

	minioEnv := map[string]string{
		"STORAGE_ENDPOINT":   validEndpoint,
		"STORAGE_ACCESS_KEY": validKey,
		"STORAGE_SECRET_KEY": validKey,
		"STORAGE_BUCKET":     validBucket,
	}

	baseEnv := merge(map[string]string{
		"POSTGRES_DSN": validDSN,
		"REDIS_ADDR":   validRedisAddr,
	}, minioEnv)

	validYAML := "http:\n  addr: \":7070\"\n" +
		"postgres:\n  dsn: \"" + validDSN + "\"\n" +
		"storage:\n  endpoint: \"" + validEndpoint + "\"\n  access_key: \"" + validKey + "\"\n  secret_key: \"" + validKey + "\"\n  bucket: \"" + validBucket + "\"\n" +
		"redis:\n  addr: \"" + validRedisAddr + "\"\n"

	tests := []struct {
		name               string
		isDocker           bool
		env                map[string]string
		setup              func(t *testing.T) string // returns the config path
		wantAddr           string
		wantRedisAddr      string
		wantCookieName     string
		wantMaxBytes       int64
		wantTTL            time.Duration
		wantKingDailyTTL   time.Duration
		wantKingMonthlyTTL time.Duration
		wantKingRankedTTL  time.Duration
		wantErr            bool
	}{
		{
			name:               "env mode all set with explicit HTTP_ADDR",
			isDocker:           true,
			env:                merge(map[string]string{"HTTP_ADDR": ":9090"}, baseEnv),
			wantAddr:           ":9090",
			wantRedisAddr:      validRedisAddr,
			wantCookieName:     defaultSessionCookieName,
			wantMaxBytes:       defaultMaxUploadBytes,
			wantTTL:            15 * time.Minute,
			wantKingDailyTTL:   24 * time.Hour,
			wantKingMonthlyTTL: 720 * time.Hour,
			wantKingRankedTTL:  24 * time.Hour,
		},
		{
			name:               "env mode default HTTP addr",
			isDocker:           true,
			env:                baseEnv,
			wantAddr:           ":8082",
			wantRedisAddr:      validRedisAddr,
			wantCookieName:     defaultSessionCookieName,
			wantMaxBytes:       defaultMaxUploadBytes,
			wantTTL:            15 * time.Minute,
			wantKingDailyTTL:   24 * time.Hour,
			wantKingMonthlyTTL: 720 * time.Hour,
			wantKingRankedTTL:  24 * time.Hour,
		},
		{
			// criterion: 4 — king clip terms are config-driven per hill (daily
			// ~24h, monthly ~30d, ranked ~24h), overridable via env vars.
			name:     "env mode custom king clip TTLs",
			isDocker: true,
			env: merge(map[string]string{
				"MEDIA_KING_DAILY_TTL":   "12h",
				"MEDIA_KING_MONTHLY_TTL": "360h",
				"MEDIA_KING_RANKED_TTL":  "6h",
			}, baseEnv),
			wantAddr:           ":8082",
			wantRedisAddr:      validRedisAddr,
			wantCookieName:     defaultSessionCookieName,
			wantMaxBytes:       defaultMaxUploadBytes,
			wantTTL:            15 * time.Minute,
			wantKingDailyTTL:   12 * time.Hour,
			wantKingMonthlyTTL: 360 * time.Hour,
			wantKingRankedTTL:  6 * time.Hour,
		},
		{
			name:     "env mode invalid king daily ttl fails",
			isDocker: true,
			env: merge(map[string]string{
				"MEDIA_KING_DAILY_TTL": "not-a-duration",
			}, baseEnv),
			wantErr: true,
		},
		{
			name:     "env mode missing Postgres DSN fails validation",
			isDocker: true,
			env:      merge(map[string]string{"REDIS_ADDR": validRedisAddr}, minioEnv),
			wantErr:  true,
		},
		{
			name:     "env mode missing Redis addr fails validation",
			isDocker: true,
			env:      merge(map[string]string{"POSTGRES_DSN": validDSN}, minioEnv),
			wantErr:  true,
		},
		{
			name:     "env mode missing storage endpoint fails validation",
			isDocker: true,
			env: map[string]string{
				"POSTGRES_DSN":       validDSN,
				"REDIS_ADDR":         validRedisAddr,
				"STORAGE_ACCESS_KEY": validKey,
				"STORAGE_SECRET_KEY": validKey,
				"STORAGE_BUCKET":     validBucket,
			},
			wantErr: true,
		},
		{
			name:     "env mode invalid download url ttl fails",
			isDocker: true,
			env: merge(map[string]string{
				"MEDIA_DOWNLOAD_URL_TTL": "not-a-duration",
			}, baseEnv),
			wantErr: true,
		},
		{
			name:     "env mode invalid max upload bytes fails",
			isDocker: true,
			env: merge(map[string]string{
				"MEDIA_MAX_UPLOAD_BYTES": "notanumber",
			}, baseEnv),
			wantErr: true,
		},
		{
			name:     "env mode custom cookie name and ttl",
			isDocker: true,
			env: merge(map[string]string{
				"SESSION_COOKIE_NAME":    "my_session",
				"MEDIA_DOWNLOAD_URL_TTL": "30m",
				"MEDIA_MAX_UPLOAD_BYTES": "1048576",
			}, baseEnv),
			wantAddr:       ":8082",
			wantRedisAddr:  validRedisAddr,
			wantCookieName: "my_session",
			wantMaxBytes:   1048576,
			wantTTL:        30 * time.Minute,
		},
		{
			// criterion: 4 — in file mode, missing king_clips section falls back
			// to the config-driven defaults (daily ~24h, monthly ~30d, ranked ~24h).
			name: "file mode reads full yaml",
			setup: func(t *testing.T) string {
				return writeTempConfig(t, validYAML)
			},
			wantAddr:           ":7070",
			wantRedisAddr:      validRedisAddr,
			wantCookieName:     defaultSessionCookieName,
			wantMaxBytes:       defaultMaxUploadBytes,
			wantTTL:            15 * time.Minute,
			wantKingDailyTTL:   24 * time.Hour,
			wantKingMonthlyTTL: 720 * time.Hour,
			wantKingRankedTTL:  24 * time.Hour,
		},
		{
			// criterion: 4 — file mode king clip TTLs are parsed from yaml.
			name: "file mode custom king clip TTLs",
			setup: func(t *testing.T) string {
				return writeTempConfig(t, validYAML+
					"king_clips:\n  daily_ttl: \"12h\"\n  monthly_ttl: \"360h\"\n  ranked_ttl: \"6h\"\n")
			},
			wantAddr:           ":7070",
			wantRedisAddr:      validRedisAddr,
			wantCookieName:     defaultSessionCookieName,
			wantMaxBytes:       defaultMaxUploadBytes,
			wantTTL:            15 * time.Minute,
			wantKingDailyTTL:   12 * time.Hour,
			wantKingMonthlyTTL: 360 * time.Hour,
			wantKingRankedTTL:  6 * time.Hour,
		},
		{
			name: "file mode invalid king daily_ttl fails",
			setup: func(t *testing.T) string {
				return writeTempConfig(t, validYAML+"king_clips:\n  daily_ttl: \"bad-duration\"\n")
			},
			wantErr: true,
		},
		{
			name: "file mode empty addr falls back to default",
			setup: func(t *testing.T) string {
				return writeTempConfig(t,
					"postgres:\n  dsn: \""+validDSN+"\"\n"+
						"storage:\n  endpoint: \""+validEndpoint+"\"\n  access_key: \""+validKey+"\"\n  secret_key: \""+validKey+"\"\n  bucket: \""+validBucket+"\"\n"+
						"redis:\n  addr: \""+validRedisAddr+"\"\n")
			},
			wantAddr:       ":8082",
			wantCookieName: defaultSessionCookieName,
			wantMaxBytes:   defaultMaxUploadBytes,
			wantTTL:        15 * time.Minute,
		},
		{
			name: "file mode missing required Postgres fails validation",
			setup: func(t *testing.T) string {
				return writeTempConfig(t,
					"storage:\n  endpoint: \""+validEndpoint+"\"\n  access_key: \""+validKey+"\"\n  secret_key: \""+validKey+"\"\n  bucket: \""+validBucket+"\"\n"+
						"redis:\n  addr: \""+validRedisAddr+"\"\n")
			},
			wantErr: true,
		},
		{
			name: "file mode missing Redis addr fails validation",
			setup: func(t *testing.T) string {
				return writeTempConfig(t,
					"postgres:\n  dsn: \""+validDSN+"\"\n"+
						"storage:\n  endpoint: \""+validEndpoint+"\"\n  access_key: \""+validKey+"\"\n  secret_key: \""+validKey+"\"\n  bucket: \""+validBucket+"\"\n")
			},
			wantErr: true,
		},
		{
			name: "file mode invalid download_url_ttl fails",
			setup: func(t *testing.T) string {
				return writeTempConfig(t,
					"postgres:\n  dsn: \""+validDSN+"\"\n"+
						"storage:\n  endpoint: \""+validEndpoint+"\"\n  access_key: \""+validKey+"\"\n  secret_key: \""+validKey+"\"\n  bucket: \""+validBucket+"\"\n"+
						"redis:\n  addr: \""+validRedisAddr+"\"\n"+
						"clips:\n  download_url_ttl: \"bad-duration\"\n")
			},
			wantErr: true,
		},
		{
			name: "file mode missing file errors",
			setup: func(t *testing.T) string {
				return filepath.Join(t.TempDir(), "missing.yaml")
			},
			wantErr: true,
		},
		{
			name: "file mode invalid yaml errors",
			setup: func(t *testing.T) string {
				return writeTempConfig(t, "http: [unterminated\n")
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.isDocker {
				t.Setenv("IS_DOCKER", "1")
			}
			for k, v := range tt.env {
				t.Setenv(k, v)
			}

			path := ""
			if tt.setup != nil {
				path = tt.setup(t)
			}

			cfg, err := Load(path)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Load() error = nil, want error")
				}

				return
			}
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if cfg.HTTP.Addr != tt.wantAddr {
				t.Errorf("addr = %q, want %q", cfg.HTTP.Addr, tt.wantAddr)
			}
			if tt.wantRedisAddr != "" && cfg.Redis.Addr != tt.wantRedisAddr {
				t.Errorf("redis.addr = %q, want %q", cfg.Redis.Addr, tt.wantRedisAddr)
			}
			if tt.wantCookieName != "" && cfg.Session.CookieName != tt.wantCookieName {
				t.Errorf("session.cookie_name = %q, want %q", cfg.Session.CookieName, tt.wantCookieName)
			}
			if tt.wantMaxBytes != 0 && cfg.Clips.MaxUploadBytes != tt.wantMaxBytes {
				t.Errorf("clips.max_upload_bytes = %d, want %d", cfg.Clips.MaxUploadBytes, tt.wantMaxBytes)
			}
			if tt.wantTTL != 0 && cfg.Clips.DownloadURLTTL != tt.wantTTL {
				t.Errorf("clips.download_url_ttl = %v, want %v", cfg.Clips.DownloadURLTTL, tt.wantTTL)
			}
			if tt.wantKingDailyTTL != 0 && cfg.KingClips.DailyTTL != tt.wantKingDailyTTL {
				t.Errorf("king_clips.daily_ttl = %v, want %v", cfg.KingClips.DailyTTL, tt.wantKingDailyTTL)
			}
			if tt.wantKingMonthlyTTL != 0 && cfg.KingClips.MonthlyTTL != tt.wantKingMonthlyTTL {
				t.Errorf("king_clips.monthly_ttl = %v, want %v", cfg.KingClips.MonthlyTTL, tt.wantKingMonthlyTTL)
			}
			if tt.wantKingRankedTTL != 0 && cfg.KingClips.RankedTTL != tt.wantKingRankedTTL {
				t.Errorf("king_clips.ranked_ttl = %v, want %v", cfg.KingClips.RankedTTL, tt.wantKingRankedTTL)
			}
		})
	}
}

func TestLoad_InternalConfig(t *testing.T) {
	const (
		validDSN       = "postgres://app:app@localhost:5432/app?sslmode=disable"
		validEndpoint  = "localhost:9000"
		validKey       = "minioadmin"
		validBucket    = "media"
		validRedisAddr = "localhost:6379"
	)

	minioEnv := map[string]string{
		"STORAGE_ENDPOINT":   validEndpoint,
		"STORAGE_ACCESS_KEY": validKey,
		"STORAGE_SECRET_KEY": validKey,
		"STORAGE_BUCKET":     validBucket,
	}

	baseEnv := merge(map[string]string{
		"POSTGRES_DSN": validDSN,
		"REDIS_ADDR":   validRedisAddr,
	}, minioEnv)

	validYAML := "http:\n  addr: \":7070\"\n" +
		"postgres:\n  dsn: \"" + validDSN + "\"\n" +
		"storage:\n  endpoint: \"" + validEndpoint + "\"\n  access_key: \"" + validKey + "\"\n  secret_key: \"" + validKey + "\"\n  bucket: \"" + validBucket + "\"\n" +
		"redis:\n  addr: \"" + validRedisAddr + "\"\n"

	tests := []struct {
		name         string
		isDocker     bool
		env          map[string]string
		setup        func(t *testing.T) string
		wantAPIToken string
		wantErr      bool
	}{
		{
			// criterion: 1 — media config gains Internal.APIToken read from env
			// INTERNAL_API_TOKEN in Docker/env mode.
			name:         "env mode reads INTERNAL_API_TOKEN into Internal.APIToken",
			isDocker:     true,
			env:          merge(map[string]string{"INTERNAL_API_TOKEN": "s2s-secret-token"}, baseEnv),
			wantAPIToken: "s2s-secret-token",
		},
		{
			// criterion: 1 — an unset INTERNAL_API_TOKEN is valid (not
			// validate:"required") and yields an empty token — the service
			// must still boot; the internalauth middleware, not config
			// validation, is what fails closed on an empty token.
			name:         "env mode unset INTERNAL_API_TOKEN yields empty token, config still loads",
			isDocker:     true,
			env:          baseEnv,
			wantAPIToken: "",
		},
		{
			// criterion: 1 — yaml mode reads internal.api_token from the file.
			name: "file mode reads internal.api_token from yaml",
			setup: func(t *testing.T) string {
				return writeTempConfig(t, validYAML+"internal:\n  api_token: \"yaml-secret-token\"\n")
			},
			wantAPIToken: "yaml-secret-token",
		},
		{
			// criterion: 1 — an absent internal section in yaml still loads
			// successfully with an empty token (not validate:"required").
			name: "file mode without internal section still loads with empty token",
			setup: func(t *testing.T) string {
				return writeTempConfig(t, validYAML)
			},
			wantAPIToken: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.isDocker {
				t.Setenv("IS_DOCKER", "1")
			}
			for k, v := range tt.env {
				t.Setenv(k, v)
			}

			path := ""
			if tt.setup != nil {
				path = tt.setup(t)
			}

			cfg, err := Load(path)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Load() error = nil, want error")
				}

				return
			}
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}

			if cfg.Internal.APIToken != tt.wantAPIToken {
				t.Errorf("Internal.APIToken = %q, want %q", cfg.Internal.APIToken, tt.wantAPIToken)
			}
		})
	}
}

func writeTempConfig(t *testing.T, body string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write temp config: %v", err)
	}

	return path
}

func merge(base, extra map[string]string) map[string]string {
	out := make(map[string]string, len(base)+len(extra))
	for k, v := range base {
		out[k] = v
	}

	for k, v := range extra {
		out[k] = v
	}

	return out
}
