package service

import (
	"context"

	"github.com/pizdagladki/full/services/koth/internal/api/repository"
)

type sessionService struct {
	repo repository.SessionRepository
}

// NewSessionService returns a SessionService wired to the given SessionRepository.
func NewSessionService(repo repository.SessionRepository) SessionService {
	return &sessionService{repo: repo}
}

func (s *sessionService) ResolveSession(ctx context.Context, sessionID string) (int64, error) {
	return s.repo.UserIDBySession(ctx, sessionID)
}
