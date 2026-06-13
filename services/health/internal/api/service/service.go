// Package service holds the health service business logic.
package service

import "github.com/pizdagladki/full/services/health/internal/api/domain"

type (
	// HealthService reports service liveness.
	HealthService interface {
		Check() domain.HealthStatus
	}
)
