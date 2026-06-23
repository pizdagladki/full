package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/pizdagladki/full/services/media/internal/api/domain"
	"github.com/pizdagladki/full/services/media/internal/api/repository"
)

// ClipServiceConfig holds the clip-related limits injected from config.
type ClipServiceConfig struct {
	MaxUploadBytes int64
	DownloadURLTTL time.Duration
}

type clipService struct {
	repo   repository.ClipRepository
	store  ObjectStore
	cfg    ClipServiceConfig
	logger *zap.Logger
}

// NewClipService returns a ClipService wired to the given repository, object
// store, config, and logger.
func NewClipService(
	repo repository.ClipRepository,
	store ObjectStore,
	cfg ClipServiceConfig,
	logger *zap.Logger,
) ClipService {
	return &clipService{
		repo:   repo,
		store:  store,
		cfg:    cfg,
		logger: logger,
	}
}

func (s *clipService) Upload(
	ctx context.Context, userID int64, contentType string, size int64, r io.Reader,
) (domain.Clip, error) {
	if !domain.ValidContentType(contentType) {
		return domain.Clip{}, domain.ErrInvalidContentType
	}

	if size <= 0 || size > s.cfg.MaxUploadBytes {
		return domain.Clip{}, domain.ErrTooLarge
	}

	id := uuid.NewString()
	key := domain.BuildObjectKey(userID, id)

	putErr := s.store.Put(ctx, key, r, size, domain.ContentTypeWebM)
	if putErr != nil {
		return domain.Clip{}, fmt.Errorf("store clip: %w", putErr)
	}

	clip := domain.Clip{
		UserID:      userID,
		ObjectKey:   key,
		Mode:        "default",
		Result:      "win",
		ContentType: domain.ContentTypeWebM,
		SizeBytes:   size,
	}

	created, err := s.repo.Create(ctx, clip)
	if err != nil {
		// Best-effort cleanup: remove the already-stored object.
		removeErr := s.store.Remove(ctx, key)
		if removeErr != nil {
			s.logger.Warn("remove orphaned clip object after create failure",
				zap.String("key", key),
				zap.Error(removeErr),
			)
		}

		return domain.Clip{}, fmt.Errorf("record clip metadata: %w", err)
	}

	// Evict oldest clips beyond the limit (best-effort).
	keys, err := s.repo.DeleteOldestBeyondLimit(ctx, userID, domain.MaxClipsPerUser)
	if err != nil {
		s.logger.Warn("delete oldest clips", zap.Error(err))
	}

	for _, k := range keys {
		removeErr := s.store.Remove(ctx, k)
		if removeErr != nil {
			s.logger.Warn("remove evicted clip object",
				zap.String("key", k),
				zap.Error(removeErr),
			)
		}
	}

	return created, nil
}

func (s *clipService) List(ctx context.Context, userID int64) ([]domain.Clip, error) {
	return s.repo.ListByUser(ctx, userID)
}

func (s *clipService) DownloadURL(ctx context.Context, userID, clipID int64) (string, error) {
	clip, err := s.repo.GetByID(ctx, clipID)
	if err != nil {
		if errors.Is(err, repository.ErrClipNotFound) {
			return "", repository.ErrClipNotFound
		}

		return "", fmt.Errorf("get clip: %w", err)
	}

	if clip.UserID != userID {
		// Do not leak the existence of another user's clip.
		return "", repository.ErrClipNotFound
	}

	url, err := s.store.PresignedGetURL(ctx, clip.ObjectKey, s.cfg.DownloadURLTTL)
	if err != nil {
		return "", fmt.Errorf("presign url: %w", err)
	}

	return url, nil
}
