package service_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"go.uber.org/mock/gomock"
	"go.uber.org/zap"

	"github.com/pizdagladki/full/services/reports/internal/api/domain"
	repomocks "github.com/pizdagladki/full/services/reports/internal/api/repository/mocks"
	"github.com/pizdagladki/full/services/reports/internal/api/service"
	svcmocks "github.com/pizdagladki/full/services/reports/internal/api/service/mocks"
)

func TestBugReportService_ReportBug(t *testing.T) {
	t.Parallel()

	webmBytes := []byte("WEBM_CONTENT")

	tests := []struct {
		name        string
		userID      int64
		device      string
		description string
		recording   []byte
		setupMocks  func(
			bugRepo *repomocks.MockBugReportsRepository,
			storage *repomocks.MockBugRecordingStorage,
			tg *svcmocks.MockTelegramNotifier,
		)
		wantErr bool
		// criterion tag for auditor lookup
		criterion string
	}{
		{
			// criterion: 1 — mobile report: no storage call, row inserted, telegram called
			name:        "mobile report: no storage call, row inserted, telegram called",
			userID:      42,
			device:      "mobile",
			description: "app crash",
			recording:   nil,
			criterion:   "AC1+AC3",
			setupMocks: func(
				bugRepo *repomocks.MockBugReportsRepository,
				storage *repomocks.MockBugRecordingStorage,
				tg *svcmocks.MockTelegramNotifier,
			) {
				// storage must NOT be called for mobile
				storage.EXPECT().StoreRecording(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

				bugRepo.EXPECT().InsertBugReport(gomock.Any(), gomock.AssignableToTypeOf(domain.BugReport{})).
					DoAndReturn(func(_ context.Context, r domain.BugReport) error {
						if r.ObjectKey != "" {
							t.Errorf("mobile report: ObjectKey = %q, want empty", r.ObjectKey)
						}
						if r.Device != "mobile" {
							t.Errorf("Device = %q, want mobile", r.Device)
						}
						return nil
					})

				tg.EXPECT().Notify(gomock.Any(), int64(42), "mobile", "app crash", "").Return(nil)
			},
		},
		{
			// criterion: 2 — pc report with recording: storage called with key, row inserted with key, telegram called with object ref
			name:        "pc report with recording: storage called with key, row inserted with key, telegram called with object ref",
			userID:      7,
			device:      "pc",
			description: "freeze",
			recording:   webmBytes,
			criterion:   "AC2+AC3",
			setupMocks: func(
				bugRepo *repomocks.MockBugReportsRepository,
				storage *repomocks.MockBugRecordingStorage,
				tg *svcmocks.MockTelegramNotifier,
			) {
				var capturedKey string

				storage.EXPECT().StoreRecording(gomock.Any(), gomock.Any(), webmBytes).
					DoAndReturn(func(_ context.Context, key string, _ []byte) error {
						capturedKey = key
						if !strings.HasSuffix(key, ".webm") {
							t.Errorf("storage key %q does not end with .webm", key)
						}
						return nil
					})

				bugRepo.EXPECT().InsertBugReport(gomock.Any(), gomock.AssignableToTypeOf(domain.BugReport{})).
					DoAndReturn(func(_ context.Context, r domain.BugReport) error {
						if r.ObjectKey == "" {
							t.Error("pc report: ObjectKey is empty, want the stored key")
						}
						if r.ObjectKey != capturedKey {
							t.Errorf("ObjectKey = %q, want %q", r.ObjectKey, capturedKey)
						}
						return nil
					})

				tg.EXPECT().Notify(gomock.Any(), int64(7), "pc", "freeze", gomock.Not("")).
					DoAndReturn(func(_ context.Context, _ int64, _, _, objectRef string) error {
						if objectRef == "" {
							t.Error("telegram: objectRef is empty for pc report")
						}
						return nil
					})
			},
		},
		{
			// criterion: 4 — telegram failure does not fail request; row still persisted
			name:        "telegram failure does not fail request",
			userID:      99,
			device:      "mobile",
			description: "slow",
			recording:   nil,
			criterion:   "AC4",
			setupMocks: func(
				bugRepo *repomocks.MockBugReportsRepository,
				storage *repomocks.MockBugRecordingStorage,
				tg *svcmocks.MockTelegramNotifier,
			) {
				storage.EXPECT().StoreRecording(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
				bugRepo.EXPECT().InsertBugReport(gomock.Any(), gomock.Any()).Return(nil)
				tg.EXPECT().Notify(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(errors.New("telegram unreachable"))
			},
			wantErr: false, // AC4: telegram failure must NOT propagate
		},
		{
			// criterion: 2 (edge) — storage failure returns error
			name:        "storage failure returns error",
			userID:      1,
			device:      "pc",
			description: "crash",
			recording:   webmBytes,
			criterion:   "AC2-edge",
			setupMocks: func(
				bugRepo *repomocks.MockBugReportsRepository,
				storage *repomocks.MockBugRecordingStorage,
				tg *svcmocks.MockTelegramNotifier,
			) {
				storage.EXPECT().StoreRecording(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(errors.New("minio down"))
				// repo and telegram must NOT be called after storage failure
				bugRepo.EXPECT().InsertBugReport(gomock.Any(), gomock.Any()).Times(0)
				tg.EXPECT().Notify(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
			},
			wantErr: true,
		},
		{
			// criterion: 1 (edge) — repo failure returns error
			name:        "repo failure returns error",
			userID:      2,
			device:      "mobile",
			description: "ui bug",
			recording:   nil,
			criterion:   "AC1-edge",
			setupMocks: func(
				bugRepo *repomocks.MockBugReportsRepository,
				storage *repomocks.MockBugRecordingStorage,
				tg *svcmocks.MockTelegramNotifier,
			) {
				storage.EXPECT().StoreRecording(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
				bugRepo.EXPECT().InsertBugReport(gomock.Any(), gomock.Any()).
					Return(errors.New("db down"))
				// telegram must NOT be called after repo failure
				tg.EXPECT().Notify(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			bugRepo := repomocks.NewMockBugReportsRepository(ctrl)
			storage := repomocks.NewMockBugRecordingStorage(ctrl)
			tg := svcmocks.NewMockTelegramNotifier(ctrl)

			tt.setupMocks(bugRepo, storage, tg)

			svc := service.NewBugReportService(bugRepo, storage, tg, zap.NewNop(), "bug-reports/")

			err := svc.ReportBug(context.Background(), tt.userID, tt.device, tt.description, tt.recording)

			if tt.wantErr && err == nil {
				t.Error("ReportBug() error = nil, want error")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("ReportBug() unexpected error = %v", err)
			}
		})
	}
}
