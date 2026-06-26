package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
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
			wantAddr: ":8080",
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
			wantAddr: ":8080",
		},
		{
			name: "file mode missing required Redis fails validation",
			setup: func(t *testing.T) string {
				return writeTempConfig(t, "http:\n  addr: \":8080\"\n")
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

func TestLoad_MatchmakingConfig_File(t *testing.T) {
	const validAddr = "localhost:6379"

	tests := []struct {
		name              string
		yaml              string
		wantLevelDist     int
		wantFallbackAfter time.Duration
		wantSessionCookie string
		wantErr           bool
	}{
		{
			name: "full matchmaking section parses correctly",
			yaml: "http:\n  addr: \":8080\"\nredis:\n  addr: \"" + validAddr + "\"\n" +
				"matchmaking:\n  level_distance: 5\n  fallback_after: \"10s\"\n  session_cookie: \"tok\"\n",
			wantLevelDist:     5,
			wantFallbackAfter: 10 * time.Second,
			wantSessionCookie: "tok",
		},
		{
			name:              "matchmaking section omitted uses defaults",
			yaml:              "http:\n  addr: \":8080\"\nredis:\n  addr: \"" + validAddr + "\"\n",
			wantLevelDist:     defaultLevelDist,
			wantFallbackAfter: defaultFallbackAfter,
			wantSessionCookie: defaultSessionCookie,
		},
		{
			name: "fallback_after string 30s",
			yaml: "http:\n  addr: \":8080\"\nredis:\n  addr: \"" + validAddr + "\"\n" +
				"matchmaking:\n  fallback_after: \"30s\"\n",
			wantLevelDist:     defaultLevelDist,
			wantFallbackAfter: 30 * time.Second,
			wantSessionCookie: defaultSessionCookie,
		},
		{
			name: "invalid fallback_after string errors",
			yaml: "http:\n  addr: \":8080\"\nredis:\n  addr: \"" + validAddr + "\"\n" +
				"matchmaking:\n  fallback_after: \"not-a-duration\"\n",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeTempConfig(t, tt.yaml)

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
			if cfg.Matchmaking.LevelDistance != tt.wantLevelDist {
				t.Errorf("LevelDistance = %d, want %d", cfg.Matchmaking.LevelDistance, tt.wantLevelDist)
			}
			if cfg.Matchmaking.FallbackAfter != tt.wantFallbackAfter {
				t.Errorf("FallbackAfter = %v, want %v", cfg.Matchmaking.FallbackAfter, tt.wantFallbackAfter)
			}
			if cfg.Matchmaking.SessionCookie != tt.wantSessionCookie {
				t.Errorf("SessionCookie = %q, want %q", cfg.Matchmaking.SessionCookie, tt.wantSessionCookie)
			}
		})
	}
}

func TestLoad_MatchmakingConfig_Env(t *testing.T) {
	const validAddr = "localhost:6379"

	tests := []struct {
		name              string
		env               map[string]string
		wantLevelDist     int
		wantFallbackAfter time.Duration
		wantSessionCookie string
	}{
		{
			name: "all matchmaking env vars set",
			env: map[string]string{
				"REDIS_ADDR":        validAddr,
				"MM_LEVEL_DISTANCE": "7",
				"MM_FALLBACK_AFTER": "20s",
				"MM_SESSION_COOKIE": "auth",
			},
			wantLevelDist:     7,
			wantFallbackAfter: 20 * time.Second,
			wantSessionCookie: "auth",
		},
		{
			name: "matchmaking env vars absent use defaults",
			env: map[string]string{
				"REDIS_ADDR": validAddr,
			},
			wantLevelDist:     defaultLevelDist,
			wantFallbackAfter: defaultFallbackAfter,
			wantSessionCookie: defaultSessionCookie,
		},
		{
			name: "invalid MM_LEVEL_DISTANCE falls back to default",
			env: map[string]string{
				"REDIS_ADDR":        validAddr,
				"MM_LEVEL_DISTANCE": "not-an-int",
			},
			wantLevelDist:     defaultLevelDist,
			wantFallbackAfter: defaultFallbackAfter,
			wantSessionCookie: defaultSessionCookie,
		},
		{
			name: "invalid MM_FALLBACK_AFTER falls back to default",
			env: map[string]string{
				"REDIS_ADDR":        validAddr,
				"MM_FALLBACK_AFTER": "not-a-duration",
			},
			wantLevelDist:     defaultLevelDist,
			wantFallbackAfter: defaultFallbackAfter,
			wantSessionCookie: defaultSessionCookie,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("IS_DOCKER", "1")
			for k, v := range tt.env {
				t.Setenv(k, v)
			}

			cfg, err := Load("")
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if cfg.Matchmaking.LevelDistance != tt.wantLevelDist {
				t.Errorf("LevelDistance = %d, want %d", cfg.Matchmaking.LevelDistance, tt.wantLevelDist)
			}
			if cfg.Matchmaking.FallbackAfter != tt.wantFallbackAfter {
				t.Errorf("FallbackAfter = %v, want %v", cfg.Matchmaking.FallbackAfter, tt.wantFallbackAfter)
			}
			if cfg.Matchmaking.SessionCookie != tt.wantSessionCookie {
				t.Errorf("SessionCookie = %q, want %q", cfg.Matchmaking.SessionCookie, tt.wantSessionCookie)
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
