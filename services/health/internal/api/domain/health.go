// Package domain holds the health service domain models and DTOs.
package domain

// HealthStatus is the response DTO for the health endpoint.
type HealthStatus struct {
	Status string `json:"status"`
}
