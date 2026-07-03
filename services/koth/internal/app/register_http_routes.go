package app

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

// registerHTTPRoutes builds the service's Echo router. Public routes are
// registered directly on the root; protected routes are grouped behind
// RequireAuth.
func (a *App) registerHTTPRoutes() *echo.Echo {
	e := echo.New()
	e.HideBanner = true
	e.HidePort = true
	e.Validator = a.validator

	e.GET("/healthz", handleHealthz)

	// Public ranked-hill leaderboard — no auth required.
	e.GET("/v1/koth/ranked/leaderboard", a.rankHandler.Leaderboard)

	// Public daily/monthly hill king lookup — no auth required.
	e.GET("/v1/koth/hills/:hill_type/king", a.hillHandler.CurrentKing)

	// Protected ranked-hill endpoints — session required.
	v1 := e.Group("/v1", a.authMiddleware.RequireAuth)
	v1.POST("/koth/ranked/attempt", a.rankHandler.SubmitAttempt)
	v1.GET("/koth/ranked/me", a.rankHandler.Me)

	// Protected daily/monthly hill challenge — session required.
	v1.POST("/koth/hills/:hill_type/challenge", a.hillHandler.Challenge)

	return e
}

// handleHealthz is the liveness probe: it reports that the process is up.
func handleHealthz(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}
