package service

import "github.com/google/uuid"

// newUUID returns a new random UUID string (v4). Extracted for testability.
func newUUID() string {
	return uuid.NewString()
}
