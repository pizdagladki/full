package app

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/pizdagladki/full/internal/platform/internalauth"
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

	// Protected clip endpoints — session required.
	v1 := e.Group("/v1", a.authMiddleware.RequireAuth)
	v1.POST("/clips", a.clipHandler.Upload)
	v1.GET("/clips", a.clipHandler.List)
	v1.GET("/clips/:id/download", a.clipHandler.Download)
	v1.POST("/clips/:id/convert", a.clipHandler.Convert)
	v1.GET("/clips/:id/mp4", a.clipHandler.GetMP4)

	// Protected king-of-the-hill clip endpoints — session required. King clips
	// are a category separate from clips (win-clips): own storage prefix, own
	// table, no keep-last-10 FIFO.
	v1.POST("/king-clips", a.kingClipHandler.Upload)
	v1.GET("/king-clips/current", a.kingClipHandler.Current)
	v1.DELETE("/king-clips/:id", a.kingClipHandler.Delete)

	// Internal server-to-server king-clip expiry — internal bearer token
	// required, no user session. Used by the koth reset worker to expire the
	// king clip on a hill reset.
	internal := e.Group("/internal/v1", internalauth.New(a.cfg.Internal.APIToken))
	internal.DELETE("/king-clips/:id", a.kingClipHandler.DeleteInternal)

	return e
}

// handleHealthz is the liveness probe: it reports that the process is up.
func handleHealthz(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}
