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

	const configuredToken = "s2s-secret-token"

	tests := []struct {
		name    string
		token   string
		baseURL func(srv *httptest.Server) string
		handler http.HandlerFunc
		wantErr bool
	}{
		{
			// criterion: 3 — a 2xx response from the store is treated as success.
			name:  "2xx response returns nil error",
			token: configuredToken,
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
			// criterion: 1/2 — the client sends "Authorization: Bearer <token>";
			// the handler rejects the request (fails this test) if the header
			// is absent or wrong, so an omitted header fails the test.
			name:  "sends Authorization Bearer header with configured token",
			token: configuredToken,
			handler: func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer "+configuredToken {
					w.WriteHeader(http.StatusBadRequest)

					return
				}
				w.WriteHeader(http.StatusOK)
			},
		},
		{
			// criterion: 2 — a 401 from the store (e.g. bad/missing internal
			// token) is surfaced as an error by the existing non-2xx handling.
			name:  "401 response from store surfaced as error",
			token: configuredToken,
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusUnauthorized)
			},
			wantErr: true,
		},
		{
			// criterion: 4 — a non-2xx response is surfaced as an error (caller logs it).
			name:  "non-2xx response returns error",
			token: configuredToken,
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			},
			wantErr: true,
		},
		{
			// criterion: 4 — a transport error (unreachable server) is surfaced as an error.
			name:  "transport error returns error",
			token: configuredToken,
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

			client := NewHTTPPointsClient(baseURL, tt.token, &http.Client{Timeout: 2 * time.Second})
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
