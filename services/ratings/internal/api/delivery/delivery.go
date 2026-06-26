// Package delivery holds the ratings service HTTP handlers (transport layer:
// request parse/validate, status codes, serialization).
package delivery

import "github.com/labstack/echo/v4"

type (
	// RatingsHandler serves the match-result and rating endpoints.
	RatingsHandler interface {
		// PostMatchResult handles POST /v1/matches/result.
		PostMatchResult(c echo.Context) error
		// GetRating handles GET /v1/ratings/:user_id.
		GetRating(c echo.Context) error
	}
)
