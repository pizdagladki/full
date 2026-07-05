// Package internalauth provides an Echo middleware for server-to-server
// endpoints that must be reachable only with a shared internal bearer token
// (e.g. one microservice calling another). It is dependency-free by design:
// no logger, no config package — just the expected token and a comparison.
package internalauth

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
)

// bearerPrefix is the required Authorization header scheme.
const bearerPrefix = "Bearer "

// New returns an echo.MiddlewareFunc that requires the Authorization header
// to be "Bearer <expectedToken>", compared in constant time via
// crypto/subtle.ConstantTimeCompare.
//
// An empty expectedToken means the middleware is misconfigured; it then
// fails CLOSED — every request is rejected with 503 — rather than accepting
// (or worse, matching) requests. The token value is never logged: this
// middleware has no logger dependency at all.
func New(expectedToken string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if expectedToken == "" {
				return c.JSON(http.StatusServiceUnavailable, map[string]string{"error": "service unavailable"})
			}

			header := c.Request().Header.Get(echo.HeaderAuthorization)
			if !strings.HasPrefix(header, bearerPrefix) {
				return c.JSON(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			}

			got := strings.TrimPrefix(header, bearerPrefix)

			if subtle.ConstantTimeCompare([]byte(got), []byte(expectedToken)) != 1 {
				return c.JSON(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			}

			return next(c)
		}
	}
}
