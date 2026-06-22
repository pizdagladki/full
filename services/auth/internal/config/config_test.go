package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

const (
	validDSN         = "postgres://app:app@localhost:5432/app?sslmode=disable"
	validRedisAddr   = "localhost:6379"
	validClientID    = "my-client-id"
	validClientSec   = "my-client-secret"
	validRedirectURL = "http://localhost:8080/callback"
)

// fullEnv returns a map with all required environment variables set to valid values.
func fullEnv() map[string]string {
	return map[string]string{
		"POSTGRES_DSN":               validDSN,
		"REDIS_ADDR":                 validRedisAddr,
		"GOOGLE_OAUTH_CLIENT_ID":     validClientID,
		"GOOGLE_OAUTH_CLIENT_SECRET": validClientSec,
		"GOOGLE_OAUTH_REDIRECT_URL":  validRedirectURL,
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
		"google_oauth:\n  client_id: \"" + validClientID + "\"\n  client_secret: \"" + validClientSec + "\"\n  redirect_url: \"" + validRedirectURL + "\"\n" +
		"session:\n  name: \"session\"\n  ttl: 24h\n"
}

func TestLoad(t *testing.T) {
	tests := []struct {
		name     string
		isDocker bool
		env      map[string]string
		setup    func(t *testing.T) string // returns the config path
		wantAddr string
		wantTTL  time.Duration
		wantErr  bool
	}{
		{
			name:     "env mode all set with explicit HTTP_ADDR",
			isDocker: true,
			env:      merge(fullEnv(), map[string]string{"HTTP_ADDR": ":9090"}),
			wantAddr: ":9090",
			wantTTL:  defaultSessionTTL,
		},
		{
			name:     "env mode default HTTP addr",
			isDocker: true,
			env:      fullEnv(),
			wantAddr: ":8080",
			wantTTL:  defaultSessionTTL,
		},
		{
			name:     "env mode SESSION_TTL as seconds",
			isDocker: true,
			env:      merge(fullEnv(), map[string]string{"SESSION_TTL": "3600"}),
			wantAddr: ":8080",
			wantTTL:  time.Hour,
		},
		{
			name:     "env mode SESSION_TTL as duration",
			isDocker: true,
			env:      merge(fullEnv(), map[string]string{"SESSION_TTL": "2h"}),
			wantAddr: ":8080",
			wantTTL:  2 * time.Hour,
		},
		{
			name:     "env mode missing Postgres DSN fails validation",
			isDocker: true,
			env:      map[string]string{"REDIS_ADDR": validRedisAddr, "GOOGLE_OAUTH_CLIENT_ID": validClientID, "GOOGLE_OAUTH_CLIENT_SECRET": validClientSec, "GOOGLE_OAUTH_REDIRECT_URL": validRedirectURL},
			wantErr:  true,
		},
		{
			name:     "env mode missing Redis addr fails validation",
			isDocker: true,
			env:      map[string]string{"POSTGRES_DSN": validDSN, "GOOGLE_OAUTH_CLIENT_ID": validClientID, "GOOGLE_OAUTH_CLIENT_SECRET": validClientSec, "GOOGLE_OAUTH_REDIRECT_URL": validRedirectURL},
			wantErr:  true,
		},
		{
			name:     "env mode missing Google OAuth client id fails validation",
			isDocker: true,
			env:      map[string]string{"POSTGRES_DSN": validDSN, "REDIS_ADDR": validRedisAddr, "GOOGLE_OAUTH_CLIENT_SECRET": validClientSec, "GOOGLE_OAUTH_REDIRECT_URL": validRedirectURL},
			wantErr:  true,
		},
		{
			name:     "env mode missing Google OAuth client secret fails validation",
			isDocker: true,
			env:      map[string]string{"POSTGRES_DSN": validDSN, "REDIS_ADDR": validRedisAddr, "GOOGLE_OAUTH_CLIENT_ID": validClientID, "GOOGLE_OAUTH_REDIRECT_URL": validRedirectURL},
			wantErr:  true,
		},
		{
			name:     "env mode missing Google OAuth redirect url fails validation",
			isDocker: true,
			env:      map[string]string{"POSTGRES_DSN": validDSN, "REDIS_ADDR": validRedisAddr, "GOOGLE_OAUTH_CLIENT_ID": validClientID, "GOOGLE_OAUTH_CLIENT_SECRET": validClientSec},
			wantErr:  true,
		},
		{
			name: "file mode reads full yaml",
			setup: func(t *testing.T) string {
				return writeTempConfig(t, fullYAML(":7070"))
			},
			wantAddr: ":7070",
			wantTTL:  24 * time.Hour,
		},
		{
			name: "file mode empty addr falls back to default",
			setup: func(t *testing.T) string {
				return writeTempConfig(t, fullYAML(""))
			},
			wantAddr: ":8080",
			wantTTL:  24 * time.Hour,
		},
		{
			name: "file mode missing required Postgres fails validation",
			setup: func(t *testing.T) string {
				return writeTempConfig(t,
					"redis:\n  addr: \""+validRedisAddr+"\"\n"+
						"google_oauth:\n  client_id: \""+validClientID+"\"\n  client_secret: \""+validClientSec+"\"\n  redirect_url: \""+validRedirectURL+"\"\n"+
						"session:\n  name: \"session\"\n  ttl: 24h\n",
				)
			},
			wantErr: true,
		},
		{
			name: "file mode missing required Google OAuth fails validation",
			setup: func(t *testing.T) string {
				return writeTempConfig(t,
					"postgres:\n  dsn: \""+validDSN+"\"\n"+
						"redis:\n  addr: \""+validRedisAddr+"\"\n"+
						"session:\n  name: \"session\"\n  ttl: 24h\n",
				)
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
			if tt.wantTTL != 0 && cfg.Session.TTL != tt.wantTTL {
				t.Errorf("session TTL = %v, want %v", cfg.Session.TTL, tt.wantTTL)
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
