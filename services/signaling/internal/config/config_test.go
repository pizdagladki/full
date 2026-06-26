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
		name                  string
		isDocker              bool
		env                   map[string]string
		setup                 func(t *testing.T) string // returns the config path
		wantAddr              string
		wantCookie            string
		wantRoomTTL           time.Duration
		wantKeepaliveInterval time.Duration
		wantKeepalivePingTO   time.Duration
		wantErr               bool
	}{
		{
			name:        "env mode all set with explicit HTTP_ADDR",
			isDocker:    true,
			env:         map[string]string{"HTTP_ADDR": ":9090", "REDIS_ADDR": validAddr},
			wantAddr:    ":9090",
			wantCookie:  defaultSessionCookie,
			wantRoomTTL: defaultRoomTTL,
		},
		{
			name:        "env mode default HTTP addr",
			isDocker:    true,
			env:         map[string]string{"REDIS_ADDR": validAddr},
			wantAddr:    ":8081",
			wantCookie:  defaultSessionCookie,
			wantRoomTTL: defaultRoomTTL,
		},
		{
			name:     "env mode missing Redis addr fails validation",
			isDocker: true,
			env:      map[string]string{},
			wantErr:  true,
		},
		{
			name:     "env mode custom session cookie and room TTL",
			isDocker: true,
			env: map[string]string{
				"REDIS_ADDR":         validAddr,
				"SIG_SESSION_COOKIE": "my_session",
				"SIG_ROOM_TTL":       "1h",
			},
			wantAddr:    defaultAddr,
			wantCookie:  "my_session",
			wantRoomTTL: time.Hour,
		},
		{
			name:     "env mode custom keepalive settings",
			isDocker: true,
			env: map[string]string{
				"REDIS_ADDR":                 validAddr,
				"SIG_KEEPALIVE_INTERVAL":     "45s",
				"SIG_KEEPALIVE_PING_TIMEOUT": "15s",
			},
			wantAddr:              defaultAddr,
			wantCookie:            defaultSessionCookie,
			wantRoomTTL:           defaultRoomTTL,
			wantKeepaliveInterval: 45 * time.Second,
			wantKeepalivePingTO:   15 * time.Second,
		},
		{
			name: "file mode reads full yaml",
			setup: func(t *testing.T) string {
				return writeTempConfig(t, "http:\n  addr: \":7070\"\nredis:\n  addr: \""+validAddr+"\"\n")
			},
			wantAddr:    ":7070",
			wantCookie:  defaultSessionCookie,
			wantRoomTTL: defaultRoomTTL,
		},
		{
			name: "file mode reads signaling section",
			setup: func(t *testing.T) string {
				return writeTempConfig(t,
					"http:\n  addr: \":8081\"\nredis:\n  addr: \""+validAddr+"\"\nsignaling:\n  session_cookie: custom\n  room_ttl: \"15m\"\n",
				)
			},
			wantAddr:    ":8081",
			wantCookie:  "custom",
			wantRoomTTL: 15 * time.Minute,
		},
		{
			name: "file mode empty addr falls back to default",
			setup: func(t *testing.T) string {
				return writeTempConfig(t, "redis:\n  addr: \""+validAddr+"\"\n")
			},
			wantAddr:    ":8081",
			wantCookie:  defaultSessionCookie,
			wantRoomTTL: defaultRoomTTL,
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
		{
			name: "file mode invalid room_ttl duration errors",
			setup: func(t *testing.T) string {
				return writeTempConfig(t,
					"http:\n  addr: \":8081\"\nredis:\n  addr: \""+validAddr+"\"\nsignaling:\n  room_ttl: \"not-a-duration\"\n",
				)
			},
			wantErr: true,
		},
		{
			name: "file mode reads keepalive settings",
			setup: func(t *testing.T) string {
				return writeTempConfig(t,
					"http:\n  addr: \":8081\"\nredis:\n  addr: \""+validAddr+"\"\nsignaling:\n  keepalive_interval: \"45s\"\n  keepalive_ping_timeout: \"15s\"\n",
				)
			},
			wantAddr:              ":8081",
			wantCookie:            defaultSessionCookie,
			wantRoomTTL:           defaultRoomTTL,
			wantKeepaliveInterval: 45 * time.Second,
			wantKeepalivePingTO:   15 * time.Second,
		},
		{
			name: "file mode invalid keepalive_interval errors",
			setup: func(t *testing.T) string {
				return writeTempConfig(t,
					"http:\n  addr: \":8081\"\nredis:\n  addr: \""+validAddr+"\"\nsignaling:\n  keepalive_interval: \"bad\"\n",
				)
			},
			wantErr: true,
		},
		{
			name: "file mode invalid keepalive_ping_timeout errors",
			setup: func(t *testing.T) string {
				return writeTempConfig(t,
					"http:\n  addr: \":8081\"\nredis:\n  addr: \""+validAddr+"\"\nsignaling:\n  keepalive_ping_timeout: \"bad\"\n",
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

			if tt.wantCookie != "" && cfg.Signaling.SessionCookie != tt.wantCookie {
				t.Errorf("session_cookie = %q, want %q", cfg.Signaling.SessionCookie, tt.wantCookie)
			}

			if tt.wantRoomTTL != 0 && cfg.Signaling.RoomTTL != tt.wantRoomTTL {
				t.Errorf("room_ttl = %v, want %v", cfg.Signaling.RoomTTL, tt.wantRoomTTL)
			}

			if tt.wantKeepaliveInterval != 0 && cfg.Signaling.KeepaliveInterval != tt.wantKeepaliveInterval {
				t.Errorf("keepalive_interval = %v, want %v", cfg.Signaling.KeepaliveInterval, tt.wantKeepaliveInterval)
			}

			if tt.wantKeepalivePingTO != 0 && cfg.Signaling.KeepalivePingTimeout != tt.wantKeepalivePingTO {
				t.Errorf("keepalive_ping_timeout = %v, want %v", cfg.Signaling.KeepalivePingTimeout, tt.wantKeepalivePingTO)
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
