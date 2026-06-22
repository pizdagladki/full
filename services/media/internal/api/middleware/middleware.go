// Package middleware holds the media service cross-cutting request logic.
// Auth middleware (session validation) is added here by downstream slices via
// the new-resource skill once the service exposes protected routes.
package middleware
