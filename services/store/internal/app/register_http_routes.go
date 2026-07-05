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

	// Public store endpoints.
	e.GET("/v1/store/catalog", a.storeHandler.GetCatalog)

	// Public Stripe webhook endpoint — Stripe POSTs here without a session.
	e.POST("/v1/store/stripe/webhook", a.purchaseHandler.StripeWebhook)

	// Protected store endpoints — session required.
	v1 := e.Group("/v1", a.authMiddleware.RequireAuth)
	v1.GET("/store/inventory", a.storeHandler.GetInventory)
	v1.POST("/store/purchase", a.purchaseHandler.CreatePurchase)
	v1.POST("/store/rewarded/grant", a.rewardedHandler.Grant)
	v1.GET("/points/balance", a.pointsHandler.GetBalance)

	// Internal server-to-server endpoints — internal bearer token required,
	// no user session.
	internal := e.Group("/v1", internalauth.New(a.cfg.Internal.APIToken))
	internal.POST("/points/credit", a.pointsHandler.Credit)

	return e
}

// handleHealthz is the liveness probe: it reports that the process is up.
func handleHealthz(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}
