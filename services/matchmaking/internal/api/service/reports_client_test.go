package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestHTTPReportsClient_GetCooldown covers the HTTP implementation of ReportsClient.
func TestHTTPReportsClient_GetCooldown(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		handler    http.HandlerFunc
		wantActive bool
		wantSecs   int
		wantErr    bool
	}{
		{
			// criterion: 1 — active cooldown response is parsed correctly
			name: "active cooldown parsed",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"active":true,"seconds_remaining":1800}`))
			},
			wantActive: true,
			wantSecs:   1800,
		},
		{
			// criterion: 2 — no cooldown response is parsed correctly
			name: "no cooldown parsed",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"active":false,"seconds_remaining":0}`))
			},
			wantActive: false,
			wantSecs:   0,
		},
		{
			// criterion: 3 — non-2xx triggers error (fail-open handled at call site)
			name: "non-200 returns error",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			},
			wantErr: true,
		},
		{
			// criterion: 3 — malformed JSON triggers error
			name: "invalid json returns error",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`not-json`))
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			srv := httptest.NewServer(tt.handler)
			t.Cleanup(srv.Close)

			client := NewHTTPReportsClient(srv.URL)
			got, err := client.GetCooldown(context.Background(), 42)

			if tt.wantErr {
				if err == nil {
					t.Fatal("GetCooldown() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("GetCooldown() unexpected error = %v", err)
			}
			if got.Active != tt.wantActive {
				t.Errorf("Active = %v, want %v", got.Active, tt.wantActive)
			}
			if got.SecondsRemaining != tt.wantSecs {
				t.Errorf("SecondsRemaining = %d, want %d", got.SecondsRemaining, tt.wantSecs)
			}
		})
	}
}
