package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

const (
	validDSN           = "postgres://app:app@localhost:5432/app?sslmode=disable"
	validRedisAddr     = "localhost:6379"
	validThresholds    = "5000,15000,30000,60000,120000"
	validStoreBaseURL  = "http://localhost:8081"
	validMediaBaseURL  = "http://localhost:8082"
	validResetInterval = "1m"
)

// fullEnv returns a map with all required environment variables set to valid values.
func fullEnv() map[string]string {
	return map[string]string{
		"POSTGRES_DSN":         validDSN,
		"REDIS_ADDR":           validRedisAddr,
		"RANKED_THRESHOLDS_MS": validThresholds,
		"STORE_BASE_URL":       validStoreBaseURL,
		"MEDIA_BASE_URL":       validMediaBaseURL,
		"RESET_CHECK_INTERVAL": validResetInterval,
	}
}

// fullYAML returns a complete config YAML string.
func fullYAML(httpAddr string) string {
	addr := httpAddr
	if addr == "" {
		addr = ":8080"
	}

	return "http:\n  addr: \"" + addr + "\"\n" +
		"postgres:\n  dsn: \"" + validDSN + "\"\n" +
		"redis:\n  addr: \"" + validRedisAddr + "\"\n" +
		"ranked:\n  thresholds: [5000, 15000, 30000, 60000, 120000]\n" +
		"store:\n  base_url: \"" + validStoreBaseURL + "\"\n" +
		"media:\n  base_url: \"" + validMediaBaseURL + "\"\n" +
		"reset:\n  check_interval: \"" + validResetInterval + "\"\n"
}

func TestLoad(t *testing.T) {
	tests := []struct {
		name     string
		isDocker bool
		env      map[string]string
		setup    func(t *testing.T) string // returns the config path
		wantAddr string
		wantErr  bool
	}{
		{
			// criterion: env mode reads HTTP_ADDR, POSTGRES_DSN, REDIS_ADDR, REDIS_PASSWORD when explicitly set
			name:     "env mode all set with explicit HTTP_ADDR",
			isDocker: true,
			env:      merge(fullEnv(), map[string]string{"HTTP_ADDR": ":9090"}),
			wantAddr: ":9090",
		},
		{
			// criterion: env mode defaults HTTP_ADDR to :8080 when unset
			name:     "env mode default HTTP addr",
			isDocker: true,
			env:      fullEnv(),
			wantAddr: ":8080",
		},
		{
			// criterion: env mode fails validation when POSTGRES_DSN is missing
			name:     "env mode missing Postgres DSN fails validation",
			isDocker: true,
			env:      map[string]string{"REDIS_ADDR": validRedisAddr, "RANKED_THRESHOLDS_MS": validThresholds},
			wantErr:  true,
		},
		{
			// criterion: env mode fails validation when REDIS_ADDR is missing
			name:     "env mode missing Redis addr fails validation",
			isDocker: true,
			env:      map[string]string{"POSTGRES_DSN": validDSN, "RANKED_THRESHOLDS_MS": validThresholds},
			wantErr:  true,
		},
		{
			// criterion: 5 — env mode fails validation when RANKED_THRESHOLDS_MS is unset (no hardcoded fallback)
			name:     "env mode missing ranked thresholds fails validation",
			isDocker: true,
			env:      map[string]string{"POSTGRES_DSN": validDSN, "REDIS_ADDR": validRedisAddr},
			wantErr:  true,
		},
		{
			// criterion: file mode reads a fully populated config.yaml
			name: "file mode reads full yaml",
			setup: func(t *testing.T) string {
				return writeTempConfig(t, fullYAML(":7070"))
			},
			wantAddr: ":7070",
		},
		{
			// criterion: file mode falls back to default HTTP addr ":8080" when addr is empty
			name: "file mode empty addr falls back to default",
			setup: func(t *testing.T) string {
				return writeTempConfig(t, fullYAML(""))
			},
			wantAddr: ":8080",
		},
		{
			// criterion: file mode fails validation when postgres section is missing
			name: "file mode missing required Postgres fails validation",
			setup: func(t *testing.T) string {
				return writeTempConfig(t,
					"redis:\n  addr: \""+validRedisAddr+"\"\n"+
						"ranked:\n  thresholds: [5000, 15000]\n",
				)
			},
			wantErr: true,
		},
		{
			// criterion: file mode errors when the config file does not exist
			name: "file mode missing file errors",
			setup: func(t *testing.T) string {
				return filepath.Join(t.TempDir(), "missing.yaml")
			},
			wantErr: true,
		},
		{
			// criterion: file mode errors on invalid yaml
			name: "file mode invalid yaml errors",
			setup: func(t *testing.T) string {
				return writeTempConfig(t, "http: [unterminated\n")
			},
			wantErr: true,
		},
		{
			// criterion: 5 — file mode fails validation when ranked.thresholds is empty (config-driven, not hardcoded)
			name: "file mode empty ranked thresholds fails validation",
			setup: func(t *testing.T) string {
				return writeTempConfig(t,
					"postgres:\n  dsn: \""+validDSN+"\"\n"+
						"redis:\n  addr: \""+validRedisAddr+"\"\n",
				)
			},
			wantErr: true,
		},
		{
			// criterion: 5 — file mode fails validation when ranked.thresholds is not strictly ascending
			name: "file mode non-ascending thresholds fails validation",
			setup: func(t *testing.T) string {
				return writeTempConfig(t,
					"postgres:\n  dsn: \""+validDSN+"\"\n"+
						"redis:\n  addr: \""+validRedisAddr+"\"\n"+
						"ranked:\n  thresholds: [5000, 3000, 30000]\n",
				)
			},
			wantErr: true,
		},
		{
			// criterion: 4 — env mode fails validation when STORE_BASE_URL is missing (no reward
			// credit target — the config-driven reset job cannot run without it)
			name:     "env mode missing store base url fails validation",
			isDocker: true,
			env: map[string]string{
				"POSTGRES_DSN": validDSN, "REDIS_ADDR": validRedisAddr, "RANKED_THRESHOLDS_MS": validThresholds,
				"MEDIA_BASE_URL": validMediaBaseURL, "RESET_CHECK_INTERVAL": validResetInterval,
			},
			wantErr: true,
		},
		{
			// criterion: 3 — env mode fails validation when MEDIA_BASE_URL is missing (no clip
			// expiry target for the reset job)
			name:     "env mode missing media base url fails validation",
			isDocker: true,
			env: map[string]string{
				"POSTGRES_DSN": validDSN, "REDIS_ADDR": validRedisAddr, "RANKED_THRESHOLDS_MS": validThresholds,
				"STORE_BASE_URL": validStoreBaseURL, "RESET_CHECK_INTERVAL": validResetInterval,
			},
			wantErr: true,
		},
		{
			// criterion: 4 — env mode fails validation when RESET_CHECK_INTERVAL is unset (the
			// scheduled worker's boundary-poll cadence is config-driven, not hardcoded)
			name:     "env mode missing reset check interval fails validation",
			isDocker: true,
			env: map[string]string{
				"POSTGRES_DSN": validDSN, "REDIS_ADDR": validRedisAddr, "RANKED_THRESHOLDS_MS": validThresholds,
				"STORE_BASE_URL": validStoreBaseURL, "MEDIA_BASE_URL": validMediaBaseURL,
			},
			wantErr: true,
		},
		{
			// criterion: 4 — env mode fails validation when RESET_CHECK_INTERVAL is malformed
			// (parseDuration falls back to the zero Duration, which fails "required")
			name:     "env mode malformed reset check interval fails validation",
			isDocker: true,
			env:      merge(fullEnv(), map[string]string{"RESET_CHECK_INTERVAL": "not-a-duration"}),
			wantErr:  true,
		},
		{
			// criterion: file mode fails validation when store.base_url is missing
			name: "file mode missing store base url fails validation",
			setup: func(t *testing.T) string {
				return writeTempConfig(t,
					"postgres:\n  dsn: \""+validDSN+"\"\n"+
						"redis:\n  addr: \""+validRedisAddr+"\"\n"+
						"ranked:\n  thresholds: [5000, 15000]\n"+
						"media:\n  base_url: \""+validMediaBaseURL+"\"\n"+
						"reset:\n  check_interval: \""+validResetInterval+"\"\n",
				)
			},
			wantErr: true,
		},
		{
			// criterion: file mode fails validation when media.base_url is missing
			name: "file mode missing media base url fails validation",
			setup: func(t *testing.T) string {
				return writeTempConfig(t,
					"postgres:\n  dsn: \""+validDSN+"\"\n"+
						"redis:\n  addr: \""+validRedisAddr+"\"\n"+
						"ranked:\n  thresholds: [5000, 15000]\n"+
						"store:\n  base_url: \""+validStoreBaseURL+"\"\n"+
						"reset:\n  check_interval: \""+validResetInterval+"\"\n",
				)
			},
			wantErr: true,
		},
		{
			// criterion: file mode fails validation when reset.check_interval is missing
			name: "file mode missing reset check interval fails validation",
			setup: func(t *testing.T) string {
				return writeTempConfig(t,
					"postgres:\n  dsn: \""+validDSN+"\"\n"+
						"redis:\n  addr: \""+validRedisAddr+"\"\n"+
						"ranked:\n  thresholds: [5000, 15000]\n"+
						"store:\n  base_url: \""+validStoreBaseURL+"\"\n"+
						"media:\n  base_url: \""+validMediaBaseURL+"\"\n",
				)
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
		})
	}
}

func TestValidateConfig(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *Config
		wantErr bool
	}{
		{
			name: "valid config passes",
			cfg: &Config{
				HTTP:     HTTPConfig{Addr: ":8080"},
				Postgres: PostgresConfig{DSN: validDSN},
				Redis:    RedisConfig{Addr: validRedisAddr},
				Ranked:   RankedConfig{Thresholds: []int{5000, 15000, 30000}},
				Store:    StoreConfig{BaseURL: validStoreBaseURL},
				Media:    MediaConfig{BaseURL: validMediaBaseURL},
				Reset:    ResetConfig{CheckInterval: time.Minute},
			},
		},
		{
			// criterion: HTTPConfig.Addr is required
			name: "missing http addr fails",
			cfg: &Config{
				Postgres: PostgresConfig{DSN: validDSN},
				Redis:    RedisConfig{Addr: validRedisAddr},
				Ranked:   RankedConfig{Thresholds: []int{5000}},
				Store:    StoreConfig{BaseURL: validStoreBaseURL},
				Media:    MediaConfig{BaseURL: validMediaBaseURL},
				Reset:    ResetConfig{CheckInterval: time.Minute},
			},
			wantErr: true,
		},
		{
			// criterion: PostgresConfig.DSN is required
			name: "missing postgres dsn fails",
			cfg: &Config{
				HTTP:   HTTPConfig{Addr: ":8080"},
				Redis:  RedisConfig{Addr: validRedisAddr},
				Ranked: RankedConfig{Thresholds: []int{5000}},
				Store:  StoreConfig{BaseURL: validStoreBaseURL},
				Media:  MediaConfig{BaseURL: validMediaBaseURL},
				Reset:  ResetConfig{CheckInterval: time.Minute},
			},
			wantErr: true,
		},
		{
			// criterion: RedisConfig.Addr is required
			name: "missing redis addr fails",
			cfg: &Config{
				HTTP:     HTTPConfig{Addr: ":8080"},
				Postgres: PostgresConfig{DSN: validDSN},
				Ranked:   RankedConfig{Thresholds: []int{5000}},
				Store:    StoreConfig{BaseURL: validStoreBaseURL},
				Media:    MediaConfig{BaseURL: validMediaBaseURL},
				Reset:    ResetConfig{CheckInterval: time.Minute},
			},
			wantErr: true,
		},
		{
			// criterion: 5 — Ranked.Thresholds is required (non-empty) — no hardcoded fallback
			name: "missing ranked thresholds fails",
			cfg: &Config{
				HTTP:     HTTPConfig{Addr: ":8080"},
				Postgres: PostgresConfig{DSN: validDSN},
				Redis:    RedisConfig{Addr: validRedisAddr},
				Store:    StoreConfig{BaseURL: validStoreBaseURL},
				Media:    MediaConfig{BaseURL: validMediaBaseURL},
				Reset:    ResetConfig{CheckInterval: time.Minute},
			},
			wantErr: true,
		},
		{
			// criterion: 5 — Ranked.Thresholds must be strictly ascending
			name: "non-ascending thresholds fails",
			cfg: &Config{
				HTTP:     HTTPConfig{Addr: ":8080"},
				Postgres: PostgresConfig{DSN: validDSN},
				Redis:    RedisConfig{Addr: validRedisAddr},
				Ranked:   RankedConfig{Thresholds: []int{5000, 5000, 30000}},
				Store:    StoreConfig{BaseURL: validStoreBaseURL},
				Media:    MediaConfig{BaseURL: validMediaBaseURL},
				Reset:    ResetConfig{CheckInterval: time.Minute},
			},
			wantErr: true,
		},
		{
			// criterion: 4 — StoreConfig.BaseURL is required (the reset job's reward-credit target)
			name: "missing store base url fails",
			cfg: &Config{
				HTTP:     HTTPConfig{Addr: ":8080"},
				Postgres: PostgresConfig{DSN: validDSN},
				Redis:    RedisConfig{Addr: validRedisAddr},
				Ranked:   RankedConfig{Thresholds: []int{5000}},
				Media:    MediaConfig{BaseURL: validMediaBaseURL},
				Reset:    ResetConfig{CheckInterval: time.Minute},
			},
			wantErr: true,
		},
		{
			// criterion: 3 — MediaConfig.BaseURL is required (the reset job's clip-expiry target)
			name: "missing media base url fails",
			cfg: &Config{
				HTTP:     HTTPConfig{Addr: ":8080"},
				Postgres: PostgresConfig{DSN: validDSN},
				Redis:    RedisConfig{Addr: validRedisAddr},
				Ranked:   RankedConfig{Thresholds: []int{5000}},
				Store:    StoreConfig{BaseURL: validStoreBaseURL},
				Reset:    ResetConfig{CheckInterval: time.Minute},
			},
			wantErr: true,
		},
		{
			// criterion: 4 — ResetConfig.CheckInterval is required (config-driven poll cadence)
			name: "missing reset check interval fails",
			cfg: &Config{
				HTTP:     HTTPConfig{Addr: ":8080"},
				Postgres: PostgresConfig{DSN: validDSN},
				Redis:    RedisConfig{Addr: validRedisAddr},
				Ranked:   RankedConfig{Thresholds: []int{5000}},
				Store:    StoreConfig{BaseURL: validStoreBaseURL},
				Media:    MediaConfig{BaseURL: validMediaBaseURL},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateConfig(tt.cfg)
			if tt.wantErr && err == nil {
				t.Fatal("ValidateConfig() error = nil, want error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("ValidateConfig() error = %v, want nil", err)
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

// merge returns a new map combining all entries from src maps (later maps win).
func merge(maps ...map[string]string) map[string]string {
	result := make(map[string]string)
	for _, m := range maps {
		for k, v := range m {
			result[k] = v
		}
	}

	return result
}
