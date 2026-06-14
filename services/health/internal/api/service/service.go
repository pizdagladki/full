// Package service holds the health service business logic.
package service

import "github.com/pizdagladki/full/services/health/internal/api/domain"

//go:generate mockgen -source=service.go -destination=mocks/service_mock.go -package=mocks

type (
	// HealthService reports service liveness.
	HealthService interface {
		Check() domain.HealthStatus
	}
)
