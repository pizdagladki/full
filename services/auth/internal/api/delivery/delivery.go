// Package delivery holds the auth service HTTP handlers (transport layer:
// request parse/validate, status codes, serialization). Handler interfaces are
// added here by downstream resource slices via the new-resource skill; the
// scaffold ships only the liveness probe wired in the app layer.
package delivery

import "github.com/labstack/echo/v4"

// AuthHandler is the transport contract for the auth resource.
type AuthHandler interface {
	// LoginGoogle handles POST /v1/auth/google.
	LoginGoogle(c echo.Context) error
	// GetMe handles GET /v1/auth/me (requires auth middleware).
	GetMe(c echo.Context) error
}
