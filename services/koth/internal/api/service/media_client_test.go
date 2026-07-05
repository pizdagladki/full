package service_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/pizdagladki/full/services/koth/internal/api/service"
)

// TestHTTPMediaClient_ExpireKingClip verifies criterion: 2 — the production
// MediaClient targets the media service's internal expiry route
// (DELETE /internal/v1/king-clips/:id) with an
// "Authorization: Bearer <internalToken>" header, treats a 404 (clip already
// gone) as success, treats any other non-2xx status as an error, and treats a
// 2xx status as success.
func TestHTTPMediaClient_ExpireKingClip(t *testing.T) {
	t.Parallel()

	const (
		configuredToken = "s2s-secret-token"
		clipID          = "42"
	)

	tests := []struct {
		name       string
		statusCode int
		wantErr    bool
	}{
		{
			// criterion: 2 — a 2xx response from media is treated as success.
			name:       "2xx response returns nil error",
			statusCode: http.StatusNoContent,
		},
		{
			// criterion: 2 — a 404 (clip already gone) is treated as success, NOT an error.
			name:       "404 response is treated as success",
			statusCode: http.StatusNotFound,
		},
		{
			// criterion: 2 — a 401 (bad/missing internal token) is surfaced as an error.
			name:       "401 response returns error",
			statusCode: http.StatusUnauthorized,
			wantErr:    true,
		},
		{
			// criterion: 2 — a 500 is surfaced as an error.
			name:       "500 response returns error",
			statusCode: http.StatusInternalServerError,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var (
				gotMethod string
				gotPath   string
				gotAuth   string
			)

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotMethod = r.Method
				gotPath = r.URL.Path
				gotAuth = r.Header.Get("Authorization")

				w.WriteHeader(tt.statusCode)
			}))
			t.Cleanup(srv.Close)

			client := service.NewHTTPMediaClient(srv.URL, configuredToken, &http.Client{Timeout: 2 * time.Second})

			err := client.ExpireKingClip(context.Background(), clipID)

			// criterion: 2 — DELETEs to exactly baseURL + "/internal/v1/king-clips/" + clipID.
			if gotMethod != http.MethodDelete {
				t.Errorf("method = %q, want %q", gotMethod, http.MethodDelete)
			}

			// criterion: 3 — path targets the internal route (contains "/internal/") and
			// the exact expected path, so a regression back to the public route fails this.
			wantPath := "/internal/v1/king-clips/" + clipID
			if gotPath != wantPath {
				t.Errorf("path = %q, want %q", gotPath, wantPath)
			}
			if !strings.Contains(gotPath, "/internal/") {
				t.Errorf("path %q does not target the internal route", gotPath)
			}

			// criterion: 2/3 — Authorization: Bearer <token> header is set on every
			// request (fails if the header is absent or holds the wrong token).
			if gotAuth != "Bearer "+configuredToken {
				t.Errorf("Authorization = %q, want %q", gotAuth, "Bearer "+configuredToken)
			}

			if tt.wantErr {
				if err == nil {
					t.Fatal("ExpireKingClip() error = nil, want error")
				}

				return
			}

			if err != nil {
				t.Fatalf("ExpireKingClip() unexpected error = %v", err)
			}
		})
	}
}

// TestHTTPMediaClient_ExpireKingClip_TransportError verifies that a transport
// error (unreachable server) is surfaced as an error rather than silently
// swallowed.
func TestHTTPMediaClient_ExpireKingClip_TransportError(t *testing.T) {
	t.Parallel()

	client := service.NewHTTPMediaClient("http://127.0.0.1:1", "token", &http.Client{Timeout: 500 * time.Millisecond})

	err := client.ExpireKingClip(context.Background(), "1")
	if err == nil {
		t.Fatal("ExpireKingClip() error = nil, want error")
	}
}
