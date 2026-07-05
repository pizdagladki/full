package internalauth_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/pizdagladki/full/internal/platform/internalauth"
)

// nextOK is the wrapped handler used by every case: it marks itself as
// having run and returns 200 with a small JSON body, so tests can tell
// "next ran" apart from "middleware short-circuited".
func nextOK(ranFlag *bool) echo.HandlerFunc {
	return func(c echo.Context) error {
		*ranFlag = true

		return c.JSON(http.StatusOK, map[string]bool{"ok": true})
	}
}

func TestNew(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		expectedToken string
		authHeader    string
		setHeader     bool
		wantStatus    int
		wantNextRan   bool
	}{
		{
			// criterion: an empty configured token must fail CLOSED with 503,
			// even when the request carries a header that looks correct.
			name:          "empty configured token -> 503 even with correct-looking header",
			expectedToken: "",
			authHeader:    "Bearer anything",
			setHeader:     true,
			wantStatus:    http.StatusServiceUnavailable,
			wantNextRan:   false,
		},
		{
			// criterion: missing Authorization header -> 401.
			name:          "missing Authorization header -> 401",
			expectedToken: "secret-token",
			setHeader:     false,
			wantStatus:    http.StatusUnauthorized,
			wantNextRan:   false,
		},
		{
			// criterion: malformed header (no Bearer prefix) -> 401.
			name:          "malformed header no Bearer prefix -> 401",
			expectedToken: "secret-token",
			authHeader:    "Token secret-token",
			setHeader:     true,
			wantStatus:    http.StatusUnauthorized,
			wantNextRan:   false,
		},
		{
			// criterion: malformed header (bare token, no scheme) -> 401.
			name:          "malformed header bare token -> 401",
			expectedToken: "secret-token",
			authHeader:    "secret-token",
			setHeader:     true,
			wantStatus:    http.StatusUnauthorized,
			wantNextRan:   false,
		},
		{
			// criterion: wrong token -> 401.
			name:          "wrong token -> 401",
			expectedToken: "secret-token",
			authHeader:    "Bearer wrong-token",
			setHeader:     true,
			wantStatus:    http.StatusUnauthorized,
			wantNextRan:   false,
		},
		{
			// criterion: an empty-string token in the header against a
			// NON-empty configured token must still 401 — guards against a
			// ConstantTimeCompare of equal-empty slices ever slipping through.
			name:          "empty token in header with non-empty configured token -> 401",
			expectedToken: "secret-token",
			authHeader:    "Bearer ",
			setHeader:     true,
			wantStatus:    http.StatusUnauthorized,
			wantNextRan:   false,
		},
		{
			// criterion: correct token -> next runs (200).
			name:          "correct token -> next runs 200",
			expectedToken: "secret-token",
			authHeader:    "Bearer secret-token",
			setHeader:     true,
			wantStatus:    http.StatusOK,
			wantNextRan:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			e := echo.New()
			req := httptest.NewRequest(http.MethodPost, "/v1/points/credit", nil)
			if tt.setHeader {
				req.Header.Set(echo.HeaderAuthorization, tt.authHeader)
			}
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			var nextRan bool
			h := internalauth.New(tt.expectedToken)(nextOK(&nextRan))

			if err := h(c); err != nil {
				t.Fatalf("handler returned error: %v", err)
			}

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d, body=%s", rec.Code, tt.wantStatus, rec.Body.String())
			}

			if nextRan != tt.wantNextRan {
				t.Errorf("next ran = %v, want %v", nextRan, tt.wantNextRan)
			}
		})
	}
}
