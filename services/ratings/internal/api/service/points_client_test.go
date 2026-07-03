package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestHTTPPointsClient_Credit covers the HTTP implementation of PointsClient.
func TestHTTPPointsClient_Credit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		baseURL func(srv *httptest.Server) string
		handler http.HandlerFunc
		wantErr bool
	}{
		{
			// criterion: 3 — a 2xx response from the store is treated as success.
			name: "2xx response returns nil error",
			handler: func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v1/points/credit" {
					w.WriteHeader(http.StatusNotFound)

					return
				}
				if r.Header.Get("Content-Type") != "application/json" {
					w.WriteHeader(http.StatusBadRequest)

					return
				}
				w.WriteHeader(http.StatusOK)
			},
		},
		{
			// criterion: 4 — a non-2xx response is surfaced as an error (caller logs it).
			name: "non-2xx response returns error",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			},
			wantErr: true,
		},
		{
			// criterion: 4 — a transport error (unreachable server) is surfaced as an error.
			name: "transport error returns error",
			baseURL: func(_ *httptest.Server) string {
				return "http://127.0.0.1:1"
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			baseURL := ""

			if tt.handler != nil {
				srv := httptest.NewServer(tt.handler)
				t.Cleanup(srv.Close)
				baseURL = srv.URL
			}

			if tt.baseURL != nil {
				baseURL = tt.baseURL(nil)
			}

			client := NewHTTPPointsClient(baseURL, &http.Client{Timeout: 2 * time.Second})
			err := client.Credit(context.Background(), CreditRequest{
				UserID: 1, Reason: "match_win", RefID: "42",
			})

			if tt.wantErr {
				if err == nil {
					t.Fatal("Credit() error = nil, want error")
				}

				return
			}

			if err != nil {
				t.Fatalf("Credit() unexpected error = %v", err)
			}
		})
	}
}
