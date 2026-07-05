package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad(t *testing.T) {
	const (
		validDSN      = "postgres://app:app@localhost:5432/app?sslmode=disable"
		validAddr     = "localhost:6379"
		validStoreURL = "http://localhost:8083"
	)

	const validInternalToken = "s2s-secret-token"

	tests := []struct {
		name       string
		isDocker   bool
		env        map[string]string
		setup      func(t *testing.T) string // returns the config path
		wantAddr   string
		wantErr    bool
		wantToken  string
		checkToken bool
	}{
		{
			name:     "env mode all set with explicit HTTP_ADDR",
			isDocker: true,
			env: map[string]string{
				"HTTP_ADDR": ":9090", "POSTGRES_DSN": validDSN, "REDIS_ADDR": validAddr,
				"STORE_BASE_URL": validStoreURL,
			},
			wantAddr: ":9090",
		},
		{
			name:     "env mode default HTTP addr",
			isDocker: true,
			env: map[string]string{
				"POSTGRES_DSN": validDSN, "REDIS_ADDR": validAddr, "STORE_BASE_URL": validStoreURL,
			},
			wantAddr: ":8080",
		},
		{
			name:     "env mode missing Postgres DSN fails validation",
			isDocker: true,
			env:      map[string]string{"REDIS_ADDR": validAddr, "STORE_BASE_URL": validStoreURL},
			wantErr:  true,
		},
		{
			name:     "env mode missing Redis addr fails validation",
			isDocker: true,
			env:      map[string]string{"POSTGRES_DSN": validDSN, "STORE_BASE_URL": validStoreURL},
			wantErr:  true,
		},
		{
			// criterion: 1 — Store.BaseURL is validated at startup: missing it fails ValidateConfig.
			name:     "env mode missing store base URL fails validation",
			isDocker: true,
			env:      map[string]string{"POSTGRES_DSN": validDSN, "REDIS_ADDR": validAddr},
			wantErr:  true,
		},
		{
			// criterion: 1 — Store.InternalToken is read from INTERNAL_API_TOKEN in env mode.
			name:     "env mode reads INTERNAL_API_TOKEN into Store.InternalToken",
			isDocker: true,
			env: map[string]string{
				"POSTGRES_DSN": validDSN, "REDIS_ADDR": validAddr, "STORE_BASE_URL": validStoreURL,
				"INTERNAL_API_TOKEN": validInternalToken,
			},
			wantAddr:   ":8080",
			checkToken: true,
			wantToken:  validInternalToken,
		},
		{
			// criterion: 1 — an unset INTERNAL_API_TOKEN is valid config (not required).
			name:     "env mode missing INTERNAL_API_TOKEN is valid config",
			isDocker: true,
			env: map[string]string{
				"POSTGRES_DSN": validDSN, "REDIS_ADDR": validAddr, "STORE_BASE_URL": validStoreURL,
			},
			wantAddr:   ":8080",
			checkToken: true,
			wantToken:  "",
		},
		{
			name: "file mode reads full yaml",
			setup: func(t *testing.T) string {
				return writeTempConfig(t, "http:\n  addr: \":7070\"\npostgres:\n  dsn: \""+validDSN+
					"\"\nredis:\n  addr: \""+validAddr+"\"\nstore:\n  base_url: \""+validStoreURL+"\"\n")
			},
			wantAddr: ":7070",
		},
		{
			name: "file mode empty addr falls back to default",
			setup: func(t *testing.T) string {
				return writeTempConfig(t, "postgres:\n  dsn: \""+validDSN+"\"\nredis:\n  addr: \""+validAddr+
					"\"\nstore:\n  base_url: \""+validStoreURL+"\"\n")
			},
			wantAddr: ":8080",
		},
		{
			// criterion: 1 — Store.InternalToken is read from the nested store.internal_token yaml key.
			name: "file mode reads store.internal_token",
			setup: func(t *testing.T) string {
				return writeTempConfig(t, "postgres:\n  dsn: \""+validDSN+"\"\nredis:\n  addr: \""+validAddr+
					"\"\nstore:\n  base_url: \""+validStoreURL+"\"\n  internal_token: \""+validInternalToken+"\"\n")
			},
			wantAddr:   ":8080",
			checkToken: true,
			wantToken:  validInternalToken,
		},
		{
			name: "file mode missing required Postgres fails validation",
			setup: func(t *testing.T) string {
				return writeTempConfig(t, "redis:\n  addr: \""+validAddr+"\"\nstore:\n  base_url: \""+validStoreURL+"\"\n")
			},
			wantErr: true,
		},
		{
			// criterion: 1 — Store.BaseURL is validated at startup: missing it fails ValidateConfig.
			name: "file mode missing store base URL fails validation",
			setup: func(t *testing.T) string {
				return writeTempConfig(t, "postgres:\n  dsn: \""+validDSN+"\"\nredis:\n  addr: \""+validAddr+"\"\n")
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
			if tt.checkToken && cfg.Store.InternalToken != tt.wantToken {
				t.Errorf("Store.InternalToken = %q, want %q", cfg.Store.InternalToken, tt.wantToken)
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
