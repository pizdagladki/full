package delivery

import (
	"errors"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"go.uber.org/zap"

	"github.com/pizdagladki/full/services/auth/internal/api/domain"
	"github.com/pizdagladki/full/services/auth/internal/api/service"
)

// UserContextKey is the key under which RequireAuth stores the domain.User in
// the Echo context (via c.Set / c.Get). Both the middleware and the handler
// use this constant to avoid magic strings.
const UserContextKey = "auth_user"

// HandlerConfig holds the cookie settings injected at construction time.
type HandlerConfig struct {
	CookieName   string
	CookieTTL    time.Duration
	CookieSecure bool
}

type authHandler struct {
	svc    service.AuthService
	logger *zap.Logger
	cfg    HandlerConfig
}

// NewAuthHandler returns an AuthHandler wired to the given AuthService.
func NewAuthHandler(svc service.AuthService, logger *zap.Logger, cfg HandlerConfig) AuthHandler {
	return &authHandler{svc: svc, logger: logger, cfg: cfg}
}

func (h *authHandler) LoginGoogle(c echo.Context) error {
	var req domain.GoogleLoginRequest

	err := c.Bind(&req)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "bad request"})
	}

	err = c.Validate(&req)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}

	sessionID, user, err := h.svc.LoginGoogle(c.Request().Context(), req.Code)
	if err != nil {
		if errors.Is(err, service.ErrInvalidCode) {
			return c.JSON(http.StatusUnauthorized, map[string]string{"error": "invalid or expired code"})
		}

		h.logger.Error("login google", zap.Error(err))

		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}

	maxAge := max(int(h.cfg.CookieTTL.Seconds()), 0)

	c.SetCookie(&http.Cookie{
		Name:     h.cfg.CookieName,
		Value:    sessionID,
		Path:     "/",
		HttpOnly: true,
		Secure:   h.cfg.CookieSecure, //nolint:gosec // controlled via config; false is acceptable for local dev
		SameSite: http.SameSiteLaxMode,
		MaxAge:   maxAge,
	})

	return c.JSON(http.StatusOK, domain.MeResponse{ID: user.ID, Email: user.Email})
}

func (h *authHandler) GetMe(c echo.Context) error {
	user, ok := c.Get(UserContextKey).(domain.User)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
	}

	return c.JSON(http.StatusOK, domain.MeResponse{ID: user.ID, Email: user.Email})
}
