package config

import (
	"os"
	"path/filepath"
	"testing"
)

const (
	validDSN       = "postgres://app:app@localhost:5432/app?sslmode=disable"
	validRedisAddr = "localhost:6379"
)

// fullEnv returns a map with all required environment variables set to valid values.
func fullEnv() map[string]string {
	return map[string]string{
		"POSTGRES_DSN": validDSN,
		"REDIS_ADDR":   validRedisAddr,
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
		"redis:\n  addr: \"" + validRedisAddr + "\"\n"
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
			env:      map[string]string{"REDIS_ADDR": validRedisAddr},
			wantErr:  true,
		},
		{
			// criterion: env mode fails validation when REDIS_ADDR is missing
			name:     "env mode missing Redis addr fails validation",
			isDocker: true,
			env:      map[string]string{"POSTGRES_DSN": validDSN},
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
					"redis:\n  addr: \""+validRedisAddr+"\"\n",
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
			},
		},
		{
			// criterion: HTTPConfig.Addr is required
			name: "missing http addr fails",
			cfg: &Config{
				Postgres: PostgresConfig{DSN: validDSN},
				Redis:    RedisConfig{Addr: validRedisAddr},
			},
			wantErr: true,
		},
		{
			// criterion: PostgresConfig.DSN is required
			name: "missing postgres dsn fails",
			cfg: &Config{
				HTTP:  HTTPConfig{Addr: ":8080"},
				Redis: RedisConfig{Addr: validRedisAddr},
			},
			wantErr: true,
		},
		{
			// criterion: RedisConfig.Addr is required
			name: "missing redis addr fails",
			cfg: &Config{
				HTTP:     HTTPConfig{Addr: ":8080"},
				Postgres: PostgresConfig{DSN: validDSN},
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
