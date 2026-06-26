package service

import (
	"context"

	"github.com/pizdagladki/full/services/reports/internal/api/repository"
)

type sessionService struct {
	repo repository.SessionRepository
}

// NewSessionService constructs a SessionService backed by the given repository.
func NewSessionService(repo repository.SessionRepository) SessionService {
	return &sessionService{repo: repo}
}

// ResolveSession returns the user ID for the given session token.
func (s *sessionService) ResolveSession(ctx context.Context, sessionID string) (int64, error) {
	return s.repo.UserIDBySession(ctx, sessionID)
}
