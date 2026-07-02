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
		validEndpoint = "localhost:9000"
		validKey      = "minioadmin"
		validSecret   = "minioadmin"
		validBucket   = "reports"
		validToken    = "123456:ABC-DEF"
		validChatID   = "-1001234567890"
	)

	// fullEnv is a helper that returns a map with all required env vars set.
	fullEnv := func(overrides map[string]string) map[string]string {
		base := map[string]string{
			"POSTGRES_DSN":       validDSN,
			"REDIS_ADDR":         validAddr,
			"MINIO_ENDPOINT":     validEndpoint,
			"MINIO_ACCESS_KEY":   validKey,
			"MINIO_SECRET_KEY":   validSecret,
			"MINIO_BUCKET":       validBucket,
			"TELEGRAM_BOT_TOKEN": validToken,
			"TELEGRAM_CHAT_ID":   validChatID,
		}
		for k, v := range overrides {
			if v == "" {
				delete(base, k)
			} else {
				base[k] = v
			}
		}
		return base
	}

	// fullYAML returns a YAML string with all required sections set.
	fullYAML := func(httpAddr string) string {
		return "http:\n  addr: \"" + httpAddr + "\"\n" +
			"postgres:\n  dsn: \"" + validDSN + "\"\n" +
			"redis:\n  addr: \"" + validAddr + "\"\n" +
			"storage:\n  endpoint: \"" + validEndpoint + "\"\n" +
			"  access_key: \"" + validKey + "\"\n" +
			"  secret_key: \"" + validSecret + "\"\n" +
			"  bucket: \"" + validBucket + "\"\n" +
			"telegram:\n  bot_token: \"" + validToken + "\"\n" +
			"  chat_id: \"" + validChatID + "\"\n"
	}

	tests := []struct {
		name     string
		isDocker bool
		env      map[string]string
		setup    func(t *testing.T) string // returns the config path
		wantAddr string
		wantErr  bool
	}{
		{
			// criterion: 3 — env mode, all required fields set
			name:     "env mode all set with explicit HTTP_ADDR",
			isDocker: true,
			env:      fullEnv(map[string]string{"HTTP_ADDR": ":9090"}),
			wantAddr: ":9090",
		},
		{
			// criterion: 3 — env mode, default HTTP addr applied
			name:     "env mode default HTTP addr",
			isDocker: true,
			env:      fullEnv(nil),
			wantAddr: ":8080",
		},
		{
			// criterion: 3 — ValidateConfig fails when Postgres DSN is unset
			name:     "env mode missing Postgres DSN fails validation",
			isDocker: true,
			env:      fullEnv(map[string]string{"POSTGRES_DSN": ""}),
			wantErr:  true,
		},
		{
			// criterion: 3 — ValidateConfig fails when Redis addr is unset
			name:     "env mode missing Redis addr fails validation",
			isDocker: true,
			env:      fullEnv(map[string]string{"REDIS_ADDR": ""}),
			wantErr:  true,
		},
		{
			// criterion: 3 — ValidateConfig fails when Storage endpoint is unset
			name:     "env mode missing storage endpoint fails validation",
			isDocker: true,
			env:      fullEnv(map[string]string{"MINIO_ENDPOINT": ""}),
			wantErr:  true,
		},
		{
			// criterion: 3 — ValidateConfig fails when Telegram bot token is unset
			name:     "env mode missing telegram token fails validation",
			isDocker: true,
			env:      fullEnv(map[string]string{"TELEGRAM_BOT_TOKEN": ""}),
			wantErr:  true,
		},
		{
			// criterion: 3 — ValidateConfig fails when Telegram chat_id is unset
			name:     "env mode missing telegram chat_id fails validation",
			isDocker: true,
			env:      fullEnv(map[string]string{"TELEGRAM_CHAT_ID": ""}),
			wantErr:  true,
		},
		{
			// criterion: 3 — file mode reads full yaml
			name: "file mode reads full yaml",
			setup: func(t *testing.T) string {
				return writeTempConfig(t, fullYAML(":7070"))
			},
			wantAddr: ":7070",
		},
		{
			// criterion: 3 — file mode empty addr falls back to default
			name: "file mode empty addr falls back to default",
			setup: func(t *testing.T) string {
				return writeTempConfig(t,
					"postgres:\n  dsn: \""+validDSN+"\"\n"+
						"redis:\n  addr: \""+validAddr+"\"\n"+
						"storage:\n  endpoint: \""+validEndpoint+"\"\n"+
						"  access_key: \""+validKey+"\"\n"+
						"  secret_key: \""+validSecret+"\"\n"+
						"  bucket: \""+validBucket+"\"\n"+
						"telegram:\n  bot_token: \""+validToken+"\"\n"+
						"  chat_id: \""+validChatID+"\"\n",
				)
			},
			wantAddr: ":8080",
		},
		{
			// criterion: 3 — file mode missing Postgres DSN fails validation
			name: "file mode missing required Postgres fails validation",
			setup: func(t *testing.T) string {
				return writeTempConfig(t, "redis:\n  addr: \""+validAddr+"\"\n")
			},
			wantErr: true,
		},
		{
			// criterion: 3 — missing config file returns error
			name: "file mode missing file errors",
			setup: func(t *testing.T) string {
				return filepath.Join(t.TempDir(), "missing.yaml")
			},
			wantErr: true,
		},
		{
			// criterion: 3 — invalid YAML returns error
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

func writeTempConfig(t *testing.T, body string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write temp config: %v", err)
	}

	return path
}
