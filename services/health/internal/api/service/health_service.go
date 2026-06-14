package service

import "github.com/pizdagladki/full/services/health/internal/api/domain"

type healthService struct{}

// NewHealthService builds a HealthService.
func NewHealthService() HealthService {
	return &healthService{}
}

func (s *healthService) Check() domain.HealthStatus {
	return domain.HealthStatus{Status: "ok"}
}
