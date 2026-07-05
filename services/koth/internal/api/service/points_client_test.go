package service_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/pizdagladki/full/services/koth/internal/api/service"
)

// TestHTTPPointsClient_Credit verifies criterion: 3 — the production
// PointsClient POSTs to baseURL+"/v1/points/credit" with the CreditRequest as
// its JSON body and a Content-Type: application/json header, treats a 2xx
// store response as success, and surfaces a non-2xx store response as an
// error (so a broken transport/status-handling branch fails these cases).
// It also verifies criterion: 1 — the request carries
// "Authorization: Bearer <internalToken>".
func TestHTTPPointsClient_Credit(t *testing.T) {
	t.Parallel()

	const configuredToken = "s2s-secret-token"

	req := service.CreditRequest{
		UserID: 99,
		Reason: "koth_win",
		RefID:  "501",
		Delta:  7,
	}

	tests := []struct {
		name       string
		statusCode int
		wantErr    bool
	}{
		{
			// criterion: 3 — a 2xx response from the store is treated as success.
			name:       "2xx response returns nil error",
			statusCode: http.StatusOK,
		},
		{
			// criterion: 3 — a non-2xx response from the store is surfaced as an error.
			name:       "non-2xx response returns error",
			statusCode: http.StatusInternalServerError,
			wantErr:    true,
		},
		{
			// criterion: 1 — a 401 from the store (e.g. bad/missing internal
			// token) is surfaced as an error via the existing non-2xx handling.
			name:       "401 response from store surfaced as error",
			statusCode: http.StatusUnauthorized,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var (
				gotMethod      string
				gotPath        string
				gotContentType string
				gotAuth        string
				gotBody        service.CreditRequest
			)

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotMethod = r.Method
				gotPath = r.URL.Path
				gotContentType = r.Header.Get("Content-Type")
				gotAuth = r.Header.Get("Authorization")

				if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
					t.Errorf("decode request body: %v", err)
				}

				w.WriteHeader(tt.statusCode)
			}))
			t.Cleanup(srv.Close)

			client := service.NewHTTPPointsClient(srv.URL, configuredToken, &http.Client{Timeout: 2 * time.Second})

			err := client.Credit(context.Background(), req)

			// criterion: 3 — POSTs to exactly baseURL + "/v1/points/credit".
			if gotMethod != http.MethodPost {
				t.Errorf("method = %q, want %q", gotMethod, http.MethodPost)
			}

			if gotPath != "/v1/points/credit" {
				t.Errorf("path = %q, want %q", gotPath, "/v1/points/credit")
			}

			// criterion: 3 — Content-Type: application/json header is set.
			if gotContentType != "application/json" {
				t.Errorf("Content-Type = %q, want %q", gotContentType, "application/json")
			}

			// criterion: 1 — Authorization: Bearer <token> header is set on every request
			// (fails if the header is absent or holds the wrong token).
			if gotAuth != "Bearer "+configuredToken {
				t.Errorf("Authorization = %q, want %q", gotAuth, "Bearer "+configuredToken)
			}

			// criterion: 3 — the request body is the CreditRequest sent in.
			if gotBody != req {
				t.Errorf("request body = %+v, want %+v", gotBody, req)
			}

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
