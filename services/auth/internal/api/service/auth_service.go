package service

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/pizdagladki/full/services/auth/internal/api/domain"
	"github.com/pizdagladki/full/services/auth/internal/api/repository"
)

type authService struct {
	repo    repository.UserRepository
	oauth   OAuthExchanger
	session SessionStore
	logger  *zap.Logger
}

// NewAuthService returns an AuthService that orchestrates the repository,
// OAuth exchanger, and session store.
func NewAuthService(
	repo repository.UserRepository,
	oauth OAuthExchanger,
	session SessionStore,
	logger *zap.Logger,
) AuthService {
	return &authService{
		repo:    repo,
		oauth:   oauth,
		session: session,
		logger:  logger,
	}
}

func (s *authService) LoginGoogle(ctx context.Context, code string) (string, domain.User, error) {
	gUser, err := s.oauth.ExchangeCode(ctx, code)
	if err != nil {
		// ErrInvalidCode already set by the exchanger; wrap unknown errors too
		// so the handler can identify invalid-code by errors.Is.
		s.logger.Warn("oauth exchange failed", zap.Error(err))

		return "", domain.User{}, fmt.Errorf("%w: %w", ErrInvalidCode, err)
	}

	user, err := s.repo.UpsertByGoogleSub(ctx, gUser.Sub, gUser.Email)
	if err != nil {
		s.logger.Error("upsert user", zap.Error(err))

		return "", domain.User{}, fmt.Errorf("login google: %w", err)
	}

	sessionID, err := s.session.Create(ctx, user.ID)
	if err != nil {
		s.logger.Error("create session", zap.Error(err))

		return "", domain.User{}, fmt.Errorf("login google: %w", err)
	}

	return sessionID, user, nil
}

func (s *authService) Authenticate(ctx context.Context, sessionID string) (domain.User, error) {
	userID, err := s.session.Get(ctx, sessionID)
	if err != nil {
		return domain.User{}, fmt.Errorf("authenticate: %w", err)
	}

	user, err := s.repo.GetByID(ctx, userID)
	if err != nil {
		return domain.User{}, fmt.Errorf("authenticate: %w", err)
	}

	return user, nil
}
