package service_test

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"go.uber.org/mock/gomock"
	"go.uber.org/zap"

	"github.com/pizdagladki/full/services/media/internal/api/domain"
	"github.com/pizdagladki/full/services/media/internal/api/repository"
	repomocks "github.com/pizdagladki/full/services/media/internal/api/repository/mocks"
	"github.com/pizdagladki/full/services/media/internal/api/service"
	svcmocks "github.com/pizdagladki/full/services/media/internal/api/service/mocks"
)

func testClipCfg() service.ClipServiceConfig {
	return service.ClipServiceConfig{
		MaxUploadBytes: 10 * 1024 * 1024, // 10 MiB
		DownloadURLTTL: 15 * time.Minute,
	}
}

func TestClipService_Upload(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		userID      int64
		contentType string
		size        int64
		body        io.Reader
		setupRepo   func(m *repomocks.MockClipRepository)
		setupStore  func(m *svcmocks.MockObjectStore)
		wantErr     error
		wantClipID  int64
	}{
		{
			name:        "happy path: Put then Create then evict",
			userID:      42,
			contentType: "video/webm",
			size:        1000,
			body:        strings.NewReader("fakevideo"),
			setupRepo: func(m *repomocks.MockClipRepository) {
				m.EXPECT().Create(gomock.Any(), gomock.Any()).Return(domain.Clip{
					ID:        7,
					UserID:    42,
					ObjectKey: "clips/42/uuid.webm",
				}, nil)
				m.EXPECT().DeleteOldestBeyondLimit(gomock.Any(), int64(42), domain.MaxClipsPerUser).
					Return([]string{}, nil)
			},
			setupStore: func(m *svcmocks.MockObjectStore) {
				m.EXPECT().Put(gomock.Any(), gomock.Any(), gomock.Any(), int64(1000), domain.ContentTypeWebM).
					Return(nil)
			},
			wantClipID: 7,
		},
		{
			name:        "non-webm content type returns ErrInvalidContentType",
			userID:      1,
			contentType: "video/mp4",
			size:        1000,
			body:        strings.NewReader("data"),
			setupRepo:   func(_ *repomocks.MockClipRepository) {},
			setupStore:  func(_ *svcmocks.MockObjectStore) {},
			wantErr:     domain.ErrInvalidContentType,
		},
		{
			name:        "zero size returns ErrTooLarge",
			userID:      1,
			contentType: "video/webm",
			size:        0,
			body:        strings.NewReader(""),
			setupRepo:   func(_ *repomocks.MockClipRepository) {},
			setupStore:  func(_ *svcmocks.MockObjectStore) {},
			wantErr:     domain.ErrTooLarge,
		},
		{
			name:        "size exceeding max returns ErrTooLarge",
			userID:      1,
			contentType: "video/webm",
			size:        11 * 1024 * 1024,
			body:        strings.NewReader("data"),
			setupRepo:   func(_ *repomocks.MockClipRepository) {},
			setupStore:  func(_ *svcmocks.MockObjectStore) {},
			wantErr:     domain.ErrTooLarge,
		},
		{
			name:        "store.Put fails returns error",
			userID:      1,
			contentType: "video/webm",
			size:        100,
			body:        strings.NewReader("data"),
			setupRepo:   func(_ *repomocks.MockClipRepository) {},
			setupStore: func(m *svcmocks.MockObjectStore) {
				m.EXPECT().Put(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(errors.New("minio down"))
			},
			wantErr: errors.New("store clip"),
		},
		{
			name:        "repo.Create fails triggers best-effort Remove",
			userID:      1,
			contentType: "video/webm",
			size:        100,
			body:        strings.NewReader("data"),
			setupRepo: func(m *repomocks.MockClipRepository) {
				m.EXPECT().Create(gomock.Any(), gomock.Any()).Return(domain.Clip{}, errors.New("db error"))
			},
			setupStore: func(m *svcmocks.MockObjectStore) {
				m.EXPECT().Put(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil)
				// Best-effort remove after create failure.
				m.EXPECT().Remove(gomock.Any(), gomock.Any()).Return(nil)
			},
			wantErr: errors.New("record clip metadata"),
		},
		{
			name:        "eviction calls Remove for each evicted key",
			userID:      42,
			contentType: "video/webm",
			size:        1000,
			body:        strings.NewReader("data"),
			setupRepo: func(m *repomocks.MockClipRepository) {
				m.EXPECT().Create(gomock.Any(), gomock.Any()).Return(domain.Clip{ID: 11, UserID: 42}, nil)
				m.EXPECT().DeleteOldestBeyondLimit(gomock.Any(), int64(42), domain.MaxClipsPerUser).
					Return([]string{"clips/42/old1.webm", "clips/42/old2.webm"}, nil)
			},
			setupStore: func(m *svcmocks.MockObjectStore) {
				m.EXPECT().Put(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
				m.EXPECT().Remove(gomock.Any(), "clips/42/old1.webm").Return(nil)
				m.EXPECT().Remove(gomock.Any(), "clips/42/old2.webm").Return(nil)
			},
			wantClipID: 11,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			repoMock := repomocks.NewMockClipRepository(ctrl)
			storeMock := svcmocks.NewMockObjectStore(ctrl)

			tt.setupRepo(repoMock)
			tt.setupStore(storeMock)

			svc := service.NewClipService(repoMock, storeMock, testClipCfg(), zap.NewNop())

			got, err := svc.Upload(context.Background(), tt.userID, tt.contentType, tt.size, tt.body)

			if tt.wantErr != nil {
				if err == nil {
					t.Fatal("Upload() error = nil, want error")
				}

				return
			}

			if err != nil {
				t.Fatalf("Upload() unexpected error = %v", err)
			}

			if got.ID != tt.wantClipID {
				t.Errorf("clip.ID = %d, want %d", got.ID, tt.wantClipID)
			}
		})
	}
}

func TestClipService_List(t *testing.T) {
	t.Parallel()

	now := time.Now()

	tests := []struct {
		name      string
		userID    int64
		setupRepo func(m *repomocks.MockClipRepository)
		wantLen   int
		wantErr   bool
	}{
		{
			name:   "passthrough to repo",
			userID: 42,
			setupRepo: func(m *repomocks.MockClipRepository) {
				m.EXPECT().ListByUser(gomock.Any(), int64(42)).Return([]domain.Clip{
					{ID: 2, UserID: 42, CreatedAt: now},
					{ID: 1, UserID: 42, CreatedAt: now.Add(-time.Minute)},
				}, nil)
			},
			wantLen: 2,
		},
		{
			name:   "empty returns empty slice",
			userID: 99,
			setupRepo: func(m *repomocks.MockClipRepository) {
				m.EXPECT().ListByUser(gomock.Any(), int64(99)).Return([]domain.Clip{}, nil)
			},
			wantLen: 0,
		},
		{
			name:   "repo error propagated",
			userID: 1,
			setupRepo: func(m *repomocks.MockClipRepository) {
				m.EXPECT().ListByUser(gomock.Any(), int64(1)).Return(nil, errors.New("db error"))
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			repoMock := repomocks.NewMockClipRepository(ctrl)
			storeMock := svcmocks.NewMockObjectStore(ctrl)

			tt.setupRepo(repoMock)

			svc := service.NewClipService(repoMock, storeMock, testClipCfg(), zap.NewNop())

			got, err := svc.List(context.Background(), tt.userID)

			if tt.wantErr {
				if err == nil {
					t.Fatal("List() error = nil, want error")
				}

				return
			}

			if err != nil {
				t.Fatalf("List() unexpected error = %v", err)
			}

			if len(got) != tt.wantLen {
				t.Errorf("len(clips) = %d, want %d", len(got), tt.wantLen)
			}
		})
	}
}

func TestClipService_DownloadURL(t *testing.T) {
	t.Parallel()

	const (
		ownerID     = int64(42)
		otherUserID = int64(99)
		clipID      = int64(1)
		wantURL     = "https://minio.example.com/presigned"
	)

	tests := []struct {
		name       string
		userID     int64
		clipID     int64
		setupRepo  func(m *repomocks.MockClipRepository)
		setupStore func(m *svcmocks.MockObjectStore)
		wantURL    string
		wantErr    error
	}{
		{
			name:   "owner gets presigned URL",
			userID: ownerID,
			clipID: clipID,
			setupRepo: func(m *repomocks.MockClipRepository) {
				m.EXPECT().GetByID(gomock.Any(), clipID).Return(domain.Clip{
					ID:        clipID,
					UserID:    ownerID,
					ObjectKey: "clips/42/a.webm",
				}, nil)
			},
			setupStore: func(m *svcmocks.MockObjectStore) {
				m.EXPECT().PresignedGetURL(gomock.Any(), "clips/42/a.webm", 15*time.Minute).Return(wantURL, nil)
			},
			wantURL: wantURL,
		},
		{
			name:   "non-owner gets ErrClipNotFound (no leak)",
			userID: otherUserID,
			clipID: clipID,
			setupRepo: func(m *repomocks.MockClipRepository) {
				m.EXPECT().GetByID(gomock.Any(), clipID).Return(domain.Clip{
					ID:     clipID,
					UserID: ownerID, // belongs to ownerID, not otherUserID
				}, nil)
			},
			setupStore: func(_ *svcmocks.MockObjectStore) {},
			wantErr:    repository.ErrClipNotFound,
		},
		{
			name:   "missing clip returns ErrClipNotFound",
			userID: ownerID,
			clipID: 999,
			setupRepo: func(m *repomocks.MockClipRepository) {
				m.EXPECT().GetByID(gomock.Any(), int64(999)).Return(domain.Clip{}, repository.ErrClipNotFound)
			},
			setupStore: func(_ *svcmocks.MockObjectStore) {},
			wantErr:    repository.ErrClipNotFound,
		},
		{
			name:   "repo error propagated",
			userID: ownerID,
			clipID: clipID,
			setupRepo: func(m *repomocks.MockClipRepository) {
				m.EXPECT().GetByID(gomock.Any(), clipID).Return(domain.Clip{}, errors.New("db error"))
			},
			setupStore: func(_ *svcmocks.MockObjectStore) {},
			wantErr:    errors.New("db error"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			repoMock := repomocks.NewMockClipRepository(ctrl)
			storeMock := svcmocks.NewMockObjectStore(ctrl)

			tt.setupRepo(repoMock)
			tt.setupStore(storeMock)

			svc := service.NewClipService(repoMock, storeMock, testClipCfg(), zap.NewNop())

			got, err := svc.DownloadURL(context.Background(), tt.userID, tt.clipID)

			if tt.wantErr != nil {
				if err == nil {
					t.Fatal("DownloadURL() error = nil, want error")
				}

				if errors.Is(tt.wantErr, repository.ErrClipNotFound) && !errors.Is(err, repository.ErrClipNotFound) {
					t.Errorf("DownloadURL() error = %v, want ErrClipNotFound", err)
				}

				return
			}

			if err != nil {
				t.Fatalf("DownloadURL() unexpected error = %v", err)
			}

			if got != tt.wantURL {
				t.Errorf("url = %q, want %q", got, tt.wantURL)
			}
		})
	}
}
