package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
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
	runner FFmpegRunner
	cfg    ClipServiceConfig
	logger *zap.Logger
}

// NewClipService returns a ClipService wired to the given repository, object
// store, ffmpeg runner, config, and logger.
func NewClipService(
	repo repository.ClipRepository,
	store ObjectStore,
	runner FFmpegRunner,
	cfg ClipServiceConfig,
	logger *zap.Logger,
) ClipService {
	return &clipService{
		repo:   repo,
		store:  store,
		runner: runner,
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

func (s *clipService) RequestConvert(ctx context.Context, userID, clipID int64) (string, error) {
	clip, err := s.repo.GetByID(ctx, clipID)
	if err != nil {
		if errors.Is(err, repository.ErrClipNotFound) {
			return "", repository.ErrClipNotFound
		}

		return "", fmt.Errorf("get clip: %w", err)
	}

	if clip.UserID != userID {
		return "", repository.ErrClipNotFound
	}

	// Idempotent: already done → return immediately without re-encoding.
	if clip.ConversionStatus == domain.ConversionStatusDone {
		return domain.ConversionStatusDone, nil
	}

	// Already in progress → return current status without starting a new goroutine.
	if clip.ConversionStatus == domain.ConversionStatusPending {
		return domain.ConversionStatusPending, nil
	}

	mp4Key := domain.BuildMP4Key(clip.UserID, strconv.FormatInt(clipID, 10))

	claimed, err := s.repo.ClaimConversion(ctx, clipID, mp4Key)
	if err != nil {
		return "", fmt.Errorf("claim conversion: %w", err)
	}

	if !claimed {
		// Another concurrent request already claimed it.
		return domain.ConversionStatusPending, nil
	}

	go s.doConvert(clip.ObjectKey, mp4Key, clipID) //nolint:contextcheck,gosec // intentional: outlives HTTP request

	return domain.ConversionStatusPending, nil
}

func (s *clipService) GetMP4URL(ctx context.Context, userID, clipID int64) (string, error) {
	clip, err := s.repo.GetByID(ctx, clipID)
	if err != nil {
		if errors.Is(err, repository.ErrClipNotFound) {
			return "", repository.ErrClipNotFound
		}

		return "", fmt.Errorf("get clip: %w", err)
	}

	if clip.UserID != userID {
		return "", repository.ErrClipNotFound
	}

	switch clip.ConversionStatus {
	case domain.ConversionStatusDone:
		url, err := s.store.PresignedGetURL(ctx, clip.MP4ObjectKey, s.cfg.DownloadURLTTL)
		if err != nil {
			return "", fmt.Errorf("presign mp4 url: %w", err)
		}

		return url, nil
	case domain.ConversionStatusFailed:
		return "", domain.ErrConversionFailed
	default:
		return "", domain.ErrConversionNotDone
	}
}

// doConvert runs the WebM→MP4 conversion pipeline in a goroutine. It downloads
// the WebM from the object store, shells out to ffmpeg, and uploads the MP4.
// It uses context.Background() so it is not canceled when the HTTP handler
// returns. A 30-minute deadline bounds the total conversion time so that a
// malformed WebM cannot hang ffmpeg indefinitely.
func (s *clipService) doConvert(webmKey, mp4Key string, clipID int64) {
	const conversionTimeout = 30 * time.Minute

	//nolint:contextcheck // intentional: runs after HTTP handler returns
	ctx, cancel := context.WithTimeout(context.Background(), conversionTimeout)
	defer cancel()

	// Download WebM to temp file.
	rc, err := s.store.Get(ctx, webmKey)
	if err != nil {
		s.logger.Error("ffmpeg: download webm", zap.String("key", webmKey), zap.Error(err))
		_ = s.repo.UpdateConversion(ctx, clipID, "", domain.ConversionStatusFailed)

		return
	}
	defer rc.Close()

	tmpIn, err := os.CreateTemp("", "media-in-*.webm")
	if err != nil {
		s.logger.Error("ffmpeg: create temp input", zap.Error(err))
		_ = s.repo.UpdateConversion(ctx, clipID, "", domain.ConversionStatusFailed)

		return
	}
	defer os.Remove(tmpIn.Name())

	_, copyErr := io.Copy(tmpIn, rc)
	tmpIn.Close()

	if copyErr != nil {
		s.logger.Error("ffmpeg: write temp input", zap.Error(copyErr))
		_ = s.repo.UpdateConversion(ctx, clipID, "", domain.ConversionStatusFailed)

		return
	}

	tmpOut, err := os.CreateTemp("", "media-out-*.mp4")
	if err != nil {
		s.logger.Error("ffmpeg: create temp output", zap.Error(err))
		_ = s.repo.UpdateConversion(ctx, clipID, "", domain.ConversionStatusFailed)

		return
	}
	tmpOut.Close()
	defer os.Remove(tmpOut.Name())

	err = s.runner.Convert(ctx, tmpIn.Name(), tmpOut.Name())
	if err != nil {
		s.logger.Error("ffmpeg: convert", zap.Error(err))
		_ = s.repo.UpdateConversion(ctx, clipID, "", domain.ConversionStatusFailed)

		return
	}

	// Upload MP4.
	f, err := os.Open(tmpOut.Name())
	if err != nil {
		s.logger.Error("ffmpeg: open output", zap.Error(err))
		_ = s.repo.UpdateConversion(ctx, clipID, "", domain.ConversionStatusFailed)

		return
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		s.logger.Error("ffmpeg: stat output", zap.Error(err))
		_ = s.repo.UpdateConversion(ctx, clipID, "", domain.ConversionStatusFailed)

		return
	}

	err = s.store.Put(ctx, mp4Key, f, info.Size(), domain.ContentTypeMP4)
	if err != nil {
		s.logger.Error("ffmpeg: upload mp4", zap.String("key", mp4Key), zap.Error(err))
		_ = s.repo.UpdateConversion(ctx, clipID, "", domain.ConversionStatusFailed)

		return
	}

	err = s.repo.UpdateConversion(ctx, clipID, mp4Key, domain.ConversionStatusDone)
	if err != nil {
		s.logger.Error("ffmpeg: record done", zap.Error(err))
	}
}
