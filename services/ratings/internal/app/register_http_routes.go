package app

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

// registerHTTPRoutes builds the service's Echo router.
func (a *App) registerHTTPRoutes() *echo.Echo {
	e := echo.New()
	e.HideBanner = true
	e.HidePort = true
	e.Validator = a.validator

	// Cap request bodies so an unauthenticated POST cannot stream an arbitrarily
	// large payload into memory.
	e.Use(middleware.BodyLimit("16K"))

	e.GET("/healthz", handleHealthz)

	// Ratings resource routes — only registered when the handler is wired
	// (it is nil in newTestApp which exercises only /healthz).
	if a.ratingsHandler != nil {
		e.POST("/v1/matches/result", a.ratingsHandler.PostMatchResult)
		e.GET("/v1/ratings/:user_id", a.ratingsHandler.GetRating)
		e.GET("/v1/matches/history", a.ratingsHandler.GetMatchHistory)
	}

	return e
}

// handleHealthz is the liveness probe: it reports that the process is up.
func handleHealthz(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}
