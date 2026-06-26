package service

import (
	"context"
	"errors"
	"fmt"

	"go.uber.org/zap"

	"github.com/pizdagladki/full/services/auth/internal/api/domain"
	"github.com/pizdagladki/full/services/auth/internal/api/repository"
)

type consentService struct {
	repo   repository.ConsentRepository
	logger *zap.Logger
}

// NewConsentService returns a ConsentService backed by the given repository.
func NewConsentService(repo repository.ConsentRepository, logger *zap.Logger) ConsentService {
	return &consentService{repo: repo, logger: logger}
}

func (s *consentService) RecordConsent(
	ctx context.Context,
	userID int64,
	req domain.ConsentRequest,
) (domain.Consent, error) {
	c := domain.Consent{
		IsAdult:          req.IsAdult,
		ConsentRecording: req.ConsentRecording,
		ConsentTos:       req.ConsentTos,
	}

	result, err := s.repo.Upsert(ctx, userID, c)
	if err != nil {
		s.logger.Error("upsert consent", zap.Int64("user_id", userID), zap.Error(err))

		return domain.Consent{}, fmt.Errorf("record consent: %w", err)
	}

	return result, nil
}

func (s *consentService) GetConsent(ctx context.Context, userID int64) (*domain.Consent, error) {
	c, err := s.repo.GetByUserID(ctx, userID)
	if err != nil {
		if errors.Is(err, repository.ErrConsentNotFound) {
			return nil, nil //nolint:nilnil // (nil, nil) is the documented "no consent yet" contract
		}

		s.logger.Error("get consent", zap.Int64("user_id", userID), zap.Error(err))

		return nil, fmt.Errorf("get consent: %w", err)
	}

	return &c, nil
}
