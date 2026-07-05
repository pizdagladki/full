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

// KingClipServiceConfig holds the king-clip-related limits and per-hill terms
// injected from config. Term lengths are config-driven (not hardcoded in
// business logic) so that ops can retune daily/monthly/ranked windows without
// a code change.
type KingClipServiceConfig struct {
	MaxUploadBytes int64
	DownloadURLTTL time.Duration
	DailyTTL       time.Duration
	MonthlyTTL     time.Duration
	RankedTTL      time.Duration
}

type kingClipService struct {
	repo   repository.KingClipRepository
	store  ObjectStore
	cfg    KingClipServiceConfig
	logger *zap.Logger

	// now is injected for deterministic expiry in tests; defaults to time.Now.
	now func() time.Time
}

// NewKingClipService returns a KingClipService wired to the given repository,
// object store, config, and logger.
func NewKingClipService(
	repo repository.KingClipRepository,
	store ObjectStore,
	cfg KingClipServiceConfig,
	logger *zap.Logger,
) KingClipService {
	return &kingClipService{
		repo:   repo,
		store:  store,
		cfg:    cfg,
		logger: logger,
		now:    time.Now,
	}
}

func (s *kingClipService) Upload(
	ctx context.Context, userID int64, hillType string, blinkTsMs int64,
	contentType string, size int64, r io.Reader,
) (domain.KingClip, error) {
	if !domain.ValidHillType(hillType) {
		return domain.KingClip{}, domain.ErrInvalidHillType
	}

	if !domain.ValidContentType(contentType) {
		return domain.KingClip{}, domain.ErrInvalidContentType
	}

	if size <= 0 || size > s.cfg.MaxUploadBytes {
		return domain.KingClip{}, domain.ErrTooLarge
	}

	if blinkTsMs < 0 {
		return domain.KingClip{}, domain.ErrInvalidBlinkTs
	}

	id := uuid.NewString()
	key := domain.BuildKingClipObjectKey(hillType, userID, id)

	putErr := s.store.Put(ctx, key, r, size, domain.ContentTypeWebM)
	if putErr != nil {
		return domain.KingClip{}, fmt.Errorf("store king clip: %w", putErr)
	}

	clip := domain.KingClip{
		UserID:    userID,
		HillType:  hillType,
		ObjectKey: key,
		BlinkTsMs: blinkTsMs,
		ExpiresAt: s.now().UTC().Add(s.termFor(hillType)),
	}

	created, err := s.repo.Create(ctx, clip)
	if err != nil {
		// Best-effort cleanup: remove the already-stored object.
		removeErr := s.store.Remove(ctx, key)
		if removeErr != nil {
			s.logger.Warn("remove orphaned king clip object after create failure",
				zap.String("key", key),
				zap.Error(removeErr),
			)
		}

		return domain.KingClip{}, fmt.Errorf("record king clip metadata: %w", err)
	}

	// Evict superseded king clip(s) for this hill (best-effort). King clips are
	// never touched by the win-clip FIFO (ClipRepository.DeleteOldestBeyondLimit)
	// and vice versa — the two categories are fully independent.
	keys, err := s.repo.DeleteSupersededByHill(ctx, hillType, created.ID)
	if err != nil {
		s.logger.Warn("delete superseded king clips", zap.Error(err))
	}

	for _, k := range keys {
		removeErr := s.store.Remove(ctx, k)
		if removeErr != nil {
			s.logger.Warn("remove superseded king clip object",
				zap.String("key", k),
				zap.Error(removeErr),
			)
		}
	}

	return created, nil
}

func (s *kingClipService) CurrentURL(ctx context.Context, hillType string) (string, int64, error) {
	if !domain.ValidHillType(hillType) {
		return "", 0, domain.ErrInvalidHillType
	}

	clip, err := s.repo.GetCurrent(ctx, hillType)
	if err != nil {
		if errors.Is(err, repository.ErrKingClipNotFound) {
			return "", 0, repository.ErrKingClipNotFound
		}

		return "", 0, fmt.Errorf("get current king clip: %w", err)
	}

	url, err := s.store.PresignedGetURL(ctx, clip.ObjectKey, s.cfg.DownloadURLTTL)
	if err != nil {
		return "", 0, fmt.Errorf("presign url: %w", err)
	}

	return url, clip.BlinkTsMs, nil
}

func (s *kingClipService) Delete(ctx context.Context, userID, id int64) error {
	clip, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrKingClipNotFound) {
			return repository.ErrKingClipNotFound
		}

		return fmt.Errorf("get king clip: %w", err)
	}

	if clip.UserID != userID {
		// Do not leak the existence of another user's clip.
		return repository.ErrKingClipNotFound
	}

	key, err := s.repo.Delete(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrKingClipNotFound) {
			return repository.ErrKingClipNotFound
		}

		return fmt.Errorf("delete king clip: %w", err)
	}

	removeErr := s.store.Remove(ctx, key)
	if removeErr != nil {
		s.logger.Warn("remove deleted king clip object", zap.String("key", key), zap.Error(removeErr))
	}

	return nil
}

// ExpireByID removes the king clip identified by id (object + metadata)
// WITHOUT an ownership check — for trusted internal callers only (the koth
// reset worker expiring a hill's king clip on a daily/monthly reset).
func (s *kingClipService) ExpireByID(ctx context.Context, id int64) error {
	key, err := s.repo.Delete(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrKingClipNotFound) {
			return repository.ErrKingClipNotFound
		}

		return fmt.Errorf("expire king clip: %w", err)
	}

	removeErr := s.store.Remove(ctx, key)
	if removeErr != nil {
		s.logger.Warn("remove expired king clip object", zap.String("key", key), zap.Error(removeErr))
	}

	return nil
}

// termFor returns the configured lifetime for hillType. Callers must validate
// hillType via domain.ValidHillType before calling this.
func (s *kingClipService) termFor(hillType string) time.Duration {
	switch hillType {
	case domain.HillTypeDaily:
		return s.cfg.DailyTTL
	case domain.HillTypeMonthly:
		return s.cfg.MonthlyTTL
	case domain.HillTypeRanked:
		return s.cfg.RankedTTL
	default:
		return 0
	}
}
