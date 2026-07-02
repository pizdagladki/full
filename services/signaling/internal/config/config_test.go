package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

const validRatingsURL = "http://localhost:8082"

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
		wantRatingsURL        string
		wantErr               bool
	}{
		{
			name:           "env mode all set with explicit HTTP_ADDR",
			isDocker:       true,
			env:            map[string]string{"HTTP_ADDR": ":9090", "REDIS_ADDR": validAddr, "RATINGS_BASE_URL": validRatingsURL},
			wantAddr:       ":9090",
			wantCookie:     defaultSessionCookie,
			wantRoomTTL:    defaultRoomTTL,
			wantRatingsURL: validRatingsURL,
		},
		{
			name:           "env mode default HTTP addr",
			isDocker:       true,
			env:            map[string]string{"REDIS_ADDR": validAddr, "RATINGS_BASE_URL": validRatingsURL},
			wantAddr:       ":8081",
			wantCookie:     defaultSessionCookie,
			wantRoomTTL:    defaultRoomTTL,
			wantRatingsURL: validRatingsURL,
		},
		{
			name:     "env mode missing Redis addr fails validation",
			isDocker: true,
			env:      map[string]string{},
			wantErr:  true,
		},
		{
			// criterion: ratings-1 — fails if missing RATINGS_BASE_URL does not fail validation
			name:     "env mode missing RATINGS_BASE_URL fails validation",
			isDocker: true,
			env:      map[string]string{"REDIS_ADDR": validAddr},
			wantErr:  true,
		},
		{
			name:     "env mode custom session cookie and room TTL",
			isDocker: true,
			env: map[string]string{
				"REDIS_ADDR":         validAddr,
				"SIG_SESSION_COOKIE": "my_session",
				"SIG_ROOM_TTL":       "1h",
				"RATINGS_BASE_URL":   validRatingsURL,
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
				"RATINGS_BASE_URL":           validRatingsURL,
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
				return writeTempConfig(t, "http:\n  addr: \":7070\"\nredis:\n  addr: \""+validAddr+"\"\nratings_base_url: \""+validRatingsURL+"\"\n")
			},
			wantAddr:       ":7070",
			wantCookie:     defaultSessionCookie,
			wantRoomTTL:    defaultRoomTTL,
			wantRatingsURL: validRatingsURL,
		},
		{
			name: "file mode reads signaling section",
			setup: func(t *testing.T) string {
				return writeTempConfig(t,
					"http:\n  addr: \":8081\"\nredis:\n  addr: \""+validAddr+"\"\nratings_base_url: \""+validRatingsURL+"\"\nsignaling:\n  session_cookie: custom\n  room_ttl: \"15m\"\n",
				)
			},
			wantAddr:    ":8081",
			wantCookie:  "custom",
			wantRoomTTL: 15 * time.Minute,
		},
		{
			name: "file mode empty addr falls back to default",
			setup: func(t *testing.T) string {
				return writeTempConfig(t, "redis:\n  addr: \""+validAddr+"\"\nratings_base_url: \""+validRatingsURL+"\"\n")
			},
			wantAddr:    ":8081",
			wantCookie:  defaultSessionCookie,
			wantRoomTTL: defaultRoomTTL,
		},
		{
			name: "file mode missing required Redis fails validation",
			setup: func(t *testing.T) string {
				return writeTempConfig(t, "http:\n  addr: \":8081\"\nratings_base_url: \""+validRatingsURL+"\"\n")
			},
			wantErr: true,
		},
		{
			// criterion: ratings-1 — fails if missing ratings_base_url in file does not fail validation
			name: "file mode missing ratings_base_url fails validation",
			setup: func(t *testing.T) string {
				return writeTempConfig(t, "http:\n  addr: \":8081\"\nredis:\n  addr: \""+validAddr+"\"\n")
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
					"http:\n  addr: \":8081\"\nredis:\n  addr: \""+validAddr+"\"\nratings_base_url: \""+validRatingsURL+"\"\nsignaling:\n  room_ttl: \"not-a-duration\"\n",
				)
			},
			wantErr: true,
		},
		{
			name: "file mode reads keepalive settings",
			setup: func(t *testing.T) string {
				return writeTempConfig(t,
					"http:\n  addr: \":8081\"\nredis:\n  addr: \""+validAddr+"\"\nratings_base_url: \""+validRatingsURL+"\"\nsignaling:\n  keepalive_interval: \"45s\"\n  keepalive_ping_timeout: \"15s\"\n",
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
					"http:\n  addr: \":8081\"\nredis:\n  addr: \""+validAddr+"\"\nratings_base_url: \""+validRatingsURL+"\"\nsignaling:\n  keepalive_interval: \"bad\"\n",
				)
			},
			wantErr: true,
		},
		{
			name: "file mode invalid keepalive_ping_timeout errors",
			setup: func(t *testing.T) string {
				return writeTempConfig(t,
					"http:\n  addr: \":8081\"\nredis:\n  addr: \""+validAddr+"\"\nratings_base_url: \""+validRatingsURL+"\"\nsignaling:\n  keepalive_ping_timeout: \"bad\"\n",
				)
			},
			wantErr: true,
		},
		{
			// criterion: ratings-1 — RATINGS_BASE_URL loaded correctly from env
			name:     "env mode RATINGS_BASE_URL loaded",
			isDocker: true,
			env: map[string]string{
				"REDIS_ADDR":       validAddr,
				"RATINGS_BASE_URL": "http://ratings:9000",
			},
			wantAddr:       defaultAddr,
			wantCookie:     defaultSessionCookie,
			wantRoomTTL:    defaultRoomTTL,
			wantRatingsURL: "http://ratings:9000",
		},
		{
			// criterion: ratings-1 — ratings_base_url loaded correctly from file
			name: "file mode ratings_base_url loaded",
			setup: func(t *testing.T) string {
				return writeTempConfig(t,
					"http:\n  addr: \":8081\"\nredis:\n  addr: \""+validAddr+"\"\nratings_base_url: \"http://ratings:9000\"\n",
				)
			},
			wantAddr:       ":8081",
			wantCookie:     defaultSessionCookie,
			wantRoomTTL:    defaultRoomTTL,
			wantRatingsURL: "http://ratings:9000",
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

			if tt.wantRatingsURL != "" && cfg.RatingsBaseURL != tt.wantRatingsURL {
				t.Errorf("ratings_base_url = %q, want %q", cfg.RatingsBaseURL, tt.wantRatingsURL)
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
