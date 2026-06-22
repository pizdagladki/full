package service_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pizdagladki/full/services/auth/internal/api/service"
)

// tokenHandler returns a minimal OAuth2 token JSON response.
func tokenHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"access_token":"fake-token","token_type":"Bearer","expires_in":3600}`))
}

func TestNewGoogleOAuth_Constructor(t *testing.T) {
	t.Parallel()

	exchanger := service.NewGoogleOAuth("client-id", "client-secret", "http://localhost/callback")
	if exchanger == nil {
		t.Fatal("NewGoogleOAuth() returned nil")
	}
}

func TestGoogleOAuth_ExchangeCode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		userinfoStatus int
		userinfoBody   string
		cancelCtx      bool
		wantSub        string
		wantEmail      string
		wantErrIs      error
	}{
		{
			name:           "success: valid token + well-formed userinfo",
			userinfoStatus: http.StatusOK,
			userinfoBody:   `{"sub":"uid-42","email":"alice@example.com"}`,
			wantSub:        "uid-42",
			wantEmail:      "alice@example.com",
		},
		{
			name:           "userinfo 401 maps to ErrInvalidCode",
			userinfoStatus: http.StatusUnauthorized,
			userinfoBody:   "",
			wantErrIs:      service.ErrInvalidCode,
		},
		{
			name:           "userinfo malformed JSON maps to ErrInvalidCode",
			userinfoStatus: http.StatusOK,
			userinfoBody:   `not-json{{{`,
			wantErrIs:      service.ErrInvalidCode,
		},
		{
			name:      "canceled context maps to ErrInvalidCode",
			cancelCtx: true,
			wantErrIs: service.ErrInvalidCode,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Fake token endpoint — shared across non-cancel cases.
			tokenServer := httptest.NewServer(http.HandlerFunc(tokenHandler))
			defer tokenServer.Close()

			// Fake userinfo endpoint.
			userinfoServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.userinfoStatus)
				if tt.userinfoBody != "" {
					_, _ = fmt.Fprint(w, tt.userinfoBody)
				}
			}))
			defer userinfoServer.Close()

			exchanger := service.NewGoogleOAuthWithEndpoints(
				"cid", "csecret", "http://localhost/callback",
				tokenServer.URL+"/token",
				userinfoServer.URL+"/userinfo",
			)

			ctx := context.Background()
			if tt.cancelCtx {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel() // cancel immediately
			}

			got, err := exchanger.ExchangeCode(ctx, "fake-code")

			if tt.wantErrIs != nil {
				if err == nil {
					t.Fatal("ExchangeCode() error = nil, want error")
				}
				if !errors.Is(err, tt.wantErrIs) {
					t.Errorf("ExchangeCode() error = %v, want errors.Is(%v)", err, tt.wantErrIs)
				}
				return
			}

			if err != nil {
				t.Fatalf("ExchangeCode() unexpected error: %v", err)
			}
			if got.Sub != tt.wantSub {
				t.Errorf("Sub = %q, want %q", got.Sub, tt.wantSub)
			}
			if got.Email != tt.wantEmail {
				t.Errorf("Email = %q, want %q", got.Email, tt.wantEmail)
			}
		})
	}
}
