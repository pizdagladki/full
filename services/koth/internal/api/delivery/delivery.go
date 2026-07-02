// Package delivery holds the koth service HTTP handlers (transport layer:
// request parse/validate, status codes, serialization). Handler interfaces are
// added here by downstream resource slices via the new-resource skill; the
// scaffold ships only the liveness probe wired directly in the app layer.
package delivery

import "github.com/labstack/echo/v4"

// UserIDContextKey is the key under which RequireAuth stores the int64 user ID
// in the Echo context (via c.Set / c.Get). Both the middleware and the handler
// use this constant to avoid magic strings.
const UserIDContextKey = "koth_user_id"

// RankHandler is the transport contract for the ranked-hill resource.
type RankHandler interface {
	// SubmitAttempt handles POST /v1/koth/ranked/attempt (requires auth).
	SubmitAttempt(c echo.Context) error
	// Me handles GET /v1/koth/ranked/me (requires auth).
	Me(c echo.Context) error
	// Leaderboard handles GET /v1/koth/ranked/leaderboard (public, no auth).
	Leaderboard(c echo.Context) error
}
