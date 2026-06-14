// Package delivery holds the health service HTTP handlers.
package delivery

import "net/http"

type (
	// HealthHandler serves the health endpoint.
	HealthHandler interface {
		Get(w http.ResponseWriter, r *http.Request)
	}
)
