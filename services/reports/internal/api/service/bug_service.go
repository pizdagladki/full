package service

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/pizdagladki/full/services/reports/internal/api/domain"
	"github.com/pizdagladki/full/services/reports/internal/api/repository"
)

type bugReportService struct {
	bugRepo   repository.BugReportsRepository
	storage   repository.BugRecordingStorage
	telegram  TelegramNotifier
	logger    *zap.Logger
	keyPrefix string
}

// NewBugReportService constructs a BugReportService. keyPrefix is prepended to
// generated object keys before they are handed to storage.
func NewBugReportService(
	bugRepo repository.BugReportsRepository,
	storage repository.BugRecordingStorage,
	telegram TelegramNotifier,
	logger *zap.Logger,
	keyPrefix string,
) BugReportService {
	return &bugReportService{
		bugRepo:   bugRepo,
		storage:   storage,
		telegram:  telegram,
		logger:    logger,
		keyPrefix: keyPrefix,
	}
}

// ReportBug records a bug report. For pc device with a non-empty recording the
// bytes are stored in object storage first; the resulting key is persisted in
// the bug_reports row. A Telegram send failure is logged but does not roll back
// the stored report.
func (s *bugReportService) ReportBug(
	ctx context.Context,
	userID int64,
	device, description string,
	recording []byte,
) error {
	objectKey := ""

	if device == "pc" && len(recording) > 0 {
		key := fmt.Sprintf("%d-%d.webm", userID, time.Now().UnixNano())

		err := s.storage.StoreRecording(ctx, key, recording)
		if err != nil {
			return fmt.Errorf("store recording: %w", err)
		}

		objectKey = key
	}

	report := domain.BugReport{
		UserID:      userID,
		Device:      device,
		Description: description,
		ObjectKey:   objectKey,
	}

	err := s.bugRepo.InsertBugReport(ctx, report)
	if err != nil {
		return fmt.Errorf("insert bug report: %w", err)
	}

	tgErr := s.telegram.Notify(ctx, userID, device, description, objectKey)
	if tgErr != nil {
		s.logger.Error("telegram notify failed", zap.Error(tgErr))
	}

	return nil
}
