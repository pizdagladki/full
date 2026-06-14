// Package domain holds the auth service domain models, DTOs, and enums. Entity
// types are added here by downstream resource slices via the new-resource skill.
package domain

import "time"

// User is the domain model for an authenticated user persisted in Postgres.
type User struct {
	ID        int64
	GoogleSub string
	Email     string
	CreatedAt time.Time
}

// GoogleLoginRequest is the request body for POST /v1/auth/google.
type GoogleLoginRequest struct {
	Code string `json:"code" validate:"required"`
}

// MeResponse is the JSON body returned by GET /v1/auth/me.
type MeResponse struct {
	ID    int64  `json:"id"`
	Email string `json:"email"`
}
