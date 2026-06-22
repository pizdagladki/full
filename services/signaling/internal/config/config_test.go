package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad(t *testing.T) {
	const validAddr = "localhost:6379"

	tests := []struct {
		name     string
		isDocker bool
		env      map[string]string
		setup    func(t *testing.T) string // returns the config path
		wantAddr string
		wantErr  bool
	}{
		{
			name:     "env mode all set with explicit HTTP_ADDR",
			isDocker: true,
			env:      map[string]string{"HTTP_ADDR": ":9090", "REDIS_ADDR": validAddr},
			wantAddr: ":9090",
		},
		{
			name:     "env mode default HTTP addr",
			isDocker: true,
			env:      map[string]string{"REDIS_ADDR": validAddr},
			wantAddr: ":8081",
		},
		{
			name:     "env mode missing Redis addr fails validation",
			isDocker: true,
			env:      map[string]string{},
			wantErr:  true,
		},
		{
			name: "file mode reads full yaml",
			setup: func(t *testing.T) string {
				return writeTempConfig(t, "http:\n  addr: \":7070\"\nredis:\n  addr: \""+validAddr+"\"\n")
			},
			wantAddr: ":7070",
		},
		{
			name: "file mode empty addr falls back to default",
			setup: func(t *testing.T) string {
				return writeTempConfig(t, "redis:\n  addr: \""+validAddr+"\"\n")
			},
			wantAddr: ":8081",
		},
		{
			name: "file mode missing required Redis fails validation",
			setup: func(t *testing.T) string {
				return writeTempConfig(t, "http:\n  addr: \":8081\"\n")
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
