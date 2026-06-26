package service_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.uber.org/zap"

	"github.com/pizdagladki/full/services/reports/internal/api/service"
)

func TestTelegramNotifier_Notify(t *testing.T) {
	// NOTE: this test mutates http.DefaultTransport; do NOT run t.Parallel()
	// across subtests that share the same server redirect.

	const chatID = "-100123"

	tests := []struct {
		name        string
		serverCode  int
		serverBody  string
		userID      int64
		device      string
		description string
		objectRef   string
		wantErr     bool
		checkBody   func(t *testing.T, body map[string]string)
		criterion   string
	}{
		{
			// criterion: 3 — notify sends correct payload to Telegram API
			name:        "successful notify sends correct payload",
			serverCode:  http.StatusOK,
			serverBody:  `{"ok":true}`,
			userID:      42,
			device:      "mobile",
			description: "app crash",
			objectRef:   "",
			wantErr:     false,
			checkBody: func(t *testing.T, body map[string]string) {
				t.Helper()
				if !strings.Contains(body["text"], "42") {
					t.Errorf("text %q does not contain user id 42", body["text"])
				}
				if !strings.Contains(body["text"], "mobile") {
					t.Errorf("text %q does not contain device", body["text"])
				}
				if !strings.Contains(body["text"], "app crash") {
					t.Errorf("text %q does not contain description", body["text"])
				}
			},
			criterion: "AC3",
		},
		{
			// criterion: 3 — pc report includes object reference in telegram message
			name:        "pc report includes object reference",
			serverCode:  http.StatusOK,
			serverBody:  `{"ok":true}`,
			userID:      7,
			device:      "pc",
			description: "freeze",
			objectRef:   "7-123.webm",
			wantErr:     false,
			checkBody: func(t *testing.T, body map[string]string) {
				t.Helper()
				if !strings.Contains(body["text"], "7-123.webm") {
					t.Errorf("text %q does not contain object reference", body["text"])
				}
			},
			criterion: "AC3",
		},
		{
			// criterion: 4 — Telegram API non-200 response returns error (caller logs it)
			name:        "non-200 response returns error",
			serverCode:  http.StatusBadRequest,
			serverBody:  `{"ok":false}`,
			userID:      1,
			device:      "mobile",
			description: "test",
			objectRef:   "",
			wantErr:     true,
			checkBody:   nil,
			criterion:   "AC4",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Each subtest must NOT run in parallel because they share
			// http.DefaultTransport replacement; run them sequentially.

			var lastBody []byte

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				lastBody, _ = io.ReadAll(r.Body)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.serverCode)
				_, _ = w.Write([]byte(tt.serverBody))
			}))
			defer srv.Close()

			// Redirect all outgoing HTTP from the notifier to our test server.
			// We use a fresh http.Transport (the real one) wrapped to rewrite
			// the host/scheme, avoiding any recursive type-assertion.
			baseTransport := &http.Transport{}
			old := http.DefaultTransport
			http.DefaultTransport = &hostRewriteTransport{
				base:       baseTransport,
				targetHost: strings.TrimPrefix(srv.URL, "http://"),
			}
			t.Cleanup(func() { http.DefaultTransport = old })

			notifier := service.NewTelegramNotifier("TESTTOKEN", chatID, zap.NewNop())
			err := notifier.Notify(context.Background(), tt.userID, tt.device, tt.description, tt.objectRef)

			if tt.wantErr {
				if err == nil {
					t.Fatal("Notify() error = nil, want error")
				}
				return
			}

			if err != nil {
				t.Fatalf("Notify() unexpected error = %v", err)
			}

			if tt.checkBody != nil && len(lastBody) > 0 {
				var parsed map[string]string
				if jsonErr := json.Unmarshal(lastBody, &parsed); jsonErr != nil {
					t.Fatalf("parse request body: %v", jsonErr)
				}
				tt.checkBody(t, parsed)
			}
		})
	}
}

// hostRewriteTransport rewrites every outgoing request's host/scheme to
// targetHost (plain HTTP) before forwarding it via base.
type hostRewriteTransport struct {
	base       http.RoundTripper
	targetHost string
}

func (rt *hostRewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	cloned := req.Clone(req.Context())
	cloned.URL.Scheme = "http"
	cloned.URL.Host = rt.targetHost
	return rt.base.RoundTrip(cloned)
}
