package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad(t *testing.T) {
	const (
		validDSN  = "postgres://app:app@localhost:5432/app?sslmode=disable"
		validAddr = "localhost:6379"
	)

	const (
		validStripeKey     = "sk_test_valid"
		validStripeWebhook = "whsec_valid"
	)

	stripeEnv := map[string]string{
		"STRIPE_SECRET_KEY":             validStripeKey,
		"STRIPE_WEBHOOK_SIGNING_SECRET": validStripeWebhook,
	}

	// stripeYAML appends the stripe section to a YAML config string.
	stripeYAML := "stripe:\n  secret_key: \"" + validStripeKey + "\"\n  webhook_signing_secret: \"" + validStripeWebhook + "\"\n"

	tests := []struct {
		name           string
		isDocker       bool
		env            map[string]string
		setup          func(t *testing.T) string // returns the config path
		wantAddr       string
		wantCookieName string
		wantErr        bool
	}{
		{
			name:           "env mode all set with explicit HTTP_ADDR",
			isDocker:       true,
			env:            mergeMaps(map[string]string{"HTTP_ADDR": ":9090", "POSTGRES_DSN": validDSN, "REDIS_ADDR": validAddr}, stripeEnv),
			wantAddr:       ":9090",
			wantCookieName: "session",
		},
		{
			name:           "env mode default HTTP addr",
			isDocker:       true,
			env:            mergeMaps(map[string]string{"POSTGRES_DSN": validDSN, "REDIS_ADDR": validAddr}, stripeEnv),
			wantAddr:       ":8083",
			wantCookieName: "session",
		},
		{
			name:           "env mode custom session cookie name",
			isDocker:       true,
			env:            mergeMaps(map[string]string{"POSTGRES_DSN": validDSN, "REDIS_ADDR": validAddr, "SESSION_COOKIE_NAME": "my_session"}, stripeEnv),
			wantAddr:       ":8083",
			wantCookieName: "my_session",
		},
		{
			name:     "env mode missing Postgres DSN fails validation",
			isDocker: true,
			env:      mergeMaps(map[string]string{"REDIS_ADDR": validAddr}, stripeEnv),
			wantErr:  true,
		},
		{
			name:     "env mode missing Redis addr fails validation",
			isDocker: true,
			env:      mergeMaps(map[string]string{"POSTGRES_DSN": validDSN}, stripeEnv),
			wantErr:  true,
		},
		{
			// criterion: stripe-required — missing Stripe secret key fails validation
			name:     "env mode missing Stripe secret key fails validation",
			isDocker: true,
			env:      map[string]string{"POSTGRES_DSN": validDSN, "REDIS_ADDR": validAddr, "STRIPE_WEBHOOK_SIGNING_SECRET": validStripeWebhook},
			wantErr:  true,
		},
		{
			// criterion: stripe-required — missing Stripe webhook signing secret fails validation
			name:     "env mode missing Stripe webhook signing secret fails validation",
			isDocker: true,
			env:      map[string]string{"POSTGRES_DSN": validDSN, "REDIS_ADDR": validAddr, "STRIPE_SECRET_KEY": validStripeKey},
			wantErr:  true,
		},
		{
			name: "file mode reads full yaml",
			setup: func(t *testing.T) string {
				return writeTempConfig(t, "http:\n  addr: \":7070\"\npostgres:\n  dsn: \""+validDSN+"\"\nredis:\n  addr: \""+validAddr+"\"\n"+stripeYAML)
			},
			wantAddr:       ":7070",
			wantCookieName: "session",
		},
		{
			name: "file mode reads session cookie name from yaml",
			setup: func(t *testing.T) string {
				return writeTempConfig(t, "http:\n  addr: \":7070\"\npostgres:\n  dsn: \""+validDSN+"\"\nredis:\n  addr: \""+validAddr+"\"\nsession:\n  cookie_name: custom_cookie\n"+stripeYAML)
			},
			wantAddr:       ":7070",
			wantCookieName: "custom_cookie",
		},
		{
			name: "file mode empty addr falls back to default",
			setup: func(t *testing.T) string {
				return writeTempConfig(t, "postgres:\n  dsn: \""+validDSN+"\"\nredis:\n  addr: \""+validAddr+"\"\n"+stripeYAML)
			},
			wantAddr:       ":8083",
			wantCookieName: "session",
		},
		{
			name: "file mode missing required Postgres fails validation",
			setup: func(t *testing.T) string {
				return writeTempConfig(t, "redis:\n  addr: \""+validAddr+"\"\n"+stripeYAML)
			},
			wantErr: true,
		},
		{
			// criterion: stripe-required — missing Stripe in YAML fails validation
			name: "file mode missing Stripe section fails validation",
			setup: func(t *testing.T) string {
				return writeTempConfig(t, "http:\n  addr: \":7070\"\npostgres:\n  dsn: \""+validDSN+"\"\nredis:\n  addr: \""+validAddr+"\"\n")
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
			if tt.wantCookieName != "" && cfg.Session.CookieName != tt.wantCookieName {
				t.Errorf("cookie name = %q, want %q", cfg.Session.CookieName, tt.wantCookieName)
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

// mergeMaps merges multiple string maps into a new map. Later maps win on
// duplicate keys.
func mergeMaps(maps ...map[string]string) map[string]string {
	out := make(map[string]string)
	for _, m := range maps {
		for k, v := range m {
			out[k] = v
		}
	}

	return out
}
