package app

import "net/http"

// registerHTTPRoutes builds the service's HTTP router (Go 1.22+ method routing).
func (a *App) registerHTTPRoutes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/health", a.healthHandler.Get)

	return mux
}
