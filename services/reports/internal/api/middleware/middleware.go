// Package middleware holds cross-cutting request logic for the reports service.
package middleware

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"
	"go.uber.org/zap"

	"github.com/pizdagladki/full/services/reports/internal/api/delivery"
	"github.com/pizdagladki/full/services/reports/internal/api/repository"
	"github.com/pizdagladki/full/services/reports/internal/api/service"
)

// AuthMiddleware validates session cookies and injects the authenticated user
// ID into the Echo context.
type AuthMiddleware struct {
	sessionSvc service.SessionService
	cookieName string
	logger     *zap.Logger
}

// NewAuthMiddleware constructs an AuthMiddleware.
func NewAuthMiddleware(sessionSvc service.SessionService, cookieName string, logger *zap.Logger) *AuthMiddleware {
	return &AuthMiddleware{
		sessionSvc: sessionSvc,
		cookieName: cookieName,
		logger:     logger,
	}
}

// RequireAuth is an Echo middleware that reads the session cookie, resolves it
// to a user ID, and sets delivery.UserIDContextKey on the context.
// Returns 401 when the cookie is absent, the session is not found, or any
// other lookup error occurs.
func (m *AuthMiddleware) RequireAuth(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		cookie, err := c.Cookie(m.cookieName)
		if err != nil {
			return echo.NewHTTPError(http.StatusUnauthorized, "missing session cookie")
		}

		userID, err := m.sessionSvc.ResolveSession(c.Request().Context(), cookie.Value)
		if err != nil {
			if errors.Is(err, repository.ErrSessionNotFound) {
				return echo.NewHTTPError(http.StatusUnauthorized, "invalid or expired session")
			}

			m.logger.Error("resolve session failed", zap.Error(err))

			return echo.NewHTTPError(http.StatusUnauthorized, "could not verify session")
		}

		c.Set(delivery.UserIDContextKey, userID)

		return next(c)
	}
}
