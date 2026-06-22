package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad(t *testing.T) {
	const (
		validDSN      = "postgres://app:app@localhost:5432/app?sslmode=disable"
		validEndpoint = "localhost:9000"
		validKey      = "minioadmin"
		validBucket   = "media"
	)

	minioEnv := map[string]string{
		"STORAGE_ENDPOINT":   validEndpoint,
		"STORAGE_ACCESS_KEY": validKey,
		"STORAGE_SECRET_KEY": validKey,
		"STORAGE_BUCKET":     validBucket,
	}

	validYAML := "http:\n  addr: \":7070\"\npostgres:\n  dsn: \"" + validDSN + "\"\nstorage:\n  endpoint: \"" + validEndpoint + "\"\n  access_key: \"" + validKey + "\"\n  secret_key: \"" + validKey + "\"\n  bucket: \"" + validBucket + "\"\n"

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
			env:      merge(map[string]string{"HTTP_ADDR": ":9090", "POSTGRES_DSN": validDSN}, minioEnv),
			wantAddr: ":9090",
		},
		{
			name:     "env mode default HTTP addr",
			isDocker: true,
			env:      merge(map[string]string{"POSTGRES_DSN": validDSN}, minioEnv),
			wantAddr: ":8082",
		},
		{
			name:     "env mode missing Postgres DSN fails validation",
			isDocker: true,
			env:      minioEnv,
			wantErr:  true,
		},
		{
			name:     "env mode missing storage endpoint fails validation",
			isDocker: true,
			env: map[string]string{
				"POSTGRES_DSN":       validDSN,
				"STORAGE_ACCESS_KEY": validKey,
				"STORAGE_SECRET_KEY": validKey,
				"STORAGE_BUCKET":     validBucket,
			},
			wantErr: true,
		},
		{
			name:     "env mode missing storage access_key fails validation",
			isDocker: true,
			env: map[string]string{
				"POSTGRES_DSN":       validDSN,
				"STORAGE_ENDPOINT":   validEndpoint,
				"STORAGE_SECRET_KEY": validKey,
				"STORAGE_BUCKET":     validBucket,
			},
			wantErr: true,
		},
		{
			name:     "env mode missing storage secret_key fails validation",
			isDocker: true,
			env: map[string]string{
				"POSTGRES_DSN":       validDSN,
				"STORAGE_ENDPOINT":   validEndpoint,
				"STORAGE_ACCESS_KEY": validKey,
				"STORAGE_BUCKET":     validBucket,
			},
			wantErr: true,
		},
		{
			name:     "env mode missing storage bucket fails validation",
			isDocker: true,
			env: map[string]string{
				"POSTGRES_DSN":       validDSN,
				"STORAGE_ENDPOINT":   validEndpoint,
				"STORAGE_ACCESS_KEY": validKey,
				"STORAGE_SECRET_KEY": validKey,
			},
			wantErr: true,
		},
		{
			name: "file mode reads full yaml",
			setup: func(t *testing.T) string {
				return writeTempConfig(t, validYAML)
			},
			wantAddr: ":7070",
		},
		{
			name: "file mode empty addr falls back to default",
			setup: func(t *testing.T) string {
				return writeTempConfig(t, "postgres:\n  dsn: \""+validDSN+"\"\nstorage:\n  endpoint: \""+validEndpoint+"\"\n  access_key: \""+validKey+"\"\n  secret_key: \""+validKey+"\"\n  bucket: \""+validBucket+"\"\n")
			},
			wantAddr: ":8082",
		},
		{
			name: "file mode missing required Postgres fails validation",
			setup: func(t *testing.T) string {
				return writeTempConfig(t, "storage:\n  endpoint: \""+validEndpoint+"\"\n  access_key: \""+validKey+"\"\n  secret_key: \""+validKey+"\"\n  bucket: \""+validBucket+"\"\n")
			},
			wantErr: true,
		},
		{
			name: "file mode missing storage endpoint fails validation",
			setup: func(t *testing.T) string {
				return writeTempConfig(t, "postgres:\n  dsn: \""+validDSN+"\"\nstorage:\n  access_key: \""+validKey+"\"\n  secret_key: \""+validKey+"\"\n  bucket: \""+validBucket+"\"\n")
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
