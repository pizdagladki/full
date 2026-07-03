// Package delivery holds the store service HTTP handlers (transport layer:
// request parse/validate, status codes, serialization). Handler interfaces are
// added here by downstream resource slices via the new-resource skill.
package delivery

import "github.com/labstack/echo/v4"

// UserIDContextKey is the key under which RequireAuth stores the int64 user ID
// in the Echo context (via c.Set / c.Get). Both the middleware and the handler
// use this constant to avoid magic strings.
const UserIDContextKey = "store_user_id"

// StoreHandler is the transport contract for the store resource.
type StoreHandler interface {
	// GetCatalog handles GET /v1/store/catalog.
	GetCatalog(c echo.Context) error
	// GetInventory handles GET /v1/store/inventory (requires auth).
	GetInventory(c echo.Context) error
}

// PurchaseHandler is the transport contract for the purchase resource.
type PurchaseHandler interface {
	// CreatePurchase handles POST /v1/store/purchase (requires auth).
	// Returns a Stripe client secret for the frontend to complete payment.
	CreatePurchase(c echo.Context) error
	// StripeWebhook handles POST /v1/store/stripe/webhook (public, no auth).
	StripeWebhook(c echo.Context) error
}

// PointsHandler is the transport contract for the points resource.
type PointsHandler interface {
	// Credit handles POST /v1/points/credit (public, server-to-server).
	Credit(c echo.Context) error
	// GetBalance handles GET /v1/points/balance (requires auth).
	GetBalance(c echo.Context) error
}
