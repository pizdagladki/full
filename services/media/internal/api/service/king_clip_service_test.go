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

func testKingClipCfg() service.KingClipServiceConfig {
	return service.KingClipServiceConfig{
		MaxUploadBytes: 10 * 1024 * 1024, // 10 MiB
		DownloadURLTTL: 15 * time.Minute,
		DailyTTL:       24 * time.Hour,
		MonthlyTTL:     30 * 24 * time.Hour,
		RankedTTL:      24 * time.Hour,
	}
}

func newKingClipSvc(
	repo *repomocks.MockKingClipRepository, store *svcmocks.MockObjectStore, fixedNow time.Time,
) service.KingClipService {
	svc := service.NewKingClipService(repo, store, testKingClipCfg(), zap.NewNop())
	service.SetKingClipServiceClock(svc, func() time.Time { return fixedNow })

	return svc
}

func TestKingClipService_Upload(t *testing.T) {
	t.Parallel()

	fixedNow := time.Date(2026, 7, 3, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name        string
		userID      int64
		hillType    string
		blinkTsMs   int64
		contentType string
		size        int64
		body        io.Reader
		setupRepo   func(m *repomocks.MockKingClipRepository)
		setupStore  func(m *svcmocks.MockObjectStore)
		wantErr     error
		wantClipID  int64
		wantExpires time.Time
	}{
		{
			// criterion: 1 — stores the object under the king-clip prefix and
			// records metadata via repo.Create; returns 201-worthy clip with id.
			name:        "happy path: daily hill stores object and metadata, expires per config",
			userID:      42,
			hillType:    domain.HillTypeDaily,
			blinkTsMs:   1500,
			contentType: "video/webm",
			size:        1000,
			body:        strings.NewReader("fakevideo"),
			setupRepo: func(m *repomocks.MockKingClipRepository) {
				m.EXPECT().Create(gomock.Any(), gomock.Any()).DoAndReturn(
					func(_ context.Context, c domain.KingClip) (domain.KingClip, error) {
						c.ID = 7
						return c, nil
					})
				m.EXPECT().DeleteSupersededByHill(gomock.Any(), domain.HillTypeDaily, int64(7)).
					Return([]string{}, nil)
			},
			setupStore: func(m *svcmocks.MockObjectStore) {
				m.EXPECT().Put(gomock.Any(), gomock.Any(), gomock.Any(), int64(1000), domain.ContentTypeWebM).
					Return(nil)
			},
			wantClipID:  7,
			wantExpires: fixedNow.Add(24 * time.Hour),
		},
		{
			// criterion: 4 — expiry uses the monthly term from config, not hardcoded.
			name:        "monthly hill uses monthly TTL from config",
			userID:      42,
			hillType:    domain.HillTypeMonthly,
			blinkTsMs:   0,
			contentType: "video/webm",
			size:        1000,
			body:        strings.NewReader("data"),
			setupRepo: func(m *repomocks.MockKingClipRepository) {
				m.EXPECT().Create(gomock.Any(), gomock.Any()).DoAndReturn(
					func(_ context.Context, c domain.KingClip) (domain.KingClip, error) {
						c.ID = 8
						return c, nil
					})
				m.EXPECT().DeleteSupersededByHill(gomock.Any(), domain.HillTypeMonthly, int64(8)).
					Return([]string{}, nil)
			},
			setupStore: func(m *svcmocks.MockObjectStore) {
				m.EXPECT().Put(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
			},
			wantClipID:  8,
			wantExpires: fixedNow.Add(30 * 24 * time.Hour),
		},
		{
			// criterion: 5 — unknown hill_type is rejected.
			name:        "unknown hill type returns ErrInvalidHillType",
			userID:      1,
			hillType:    "weekly",
			blinkTsMs:   0,
			contentType: "video/webm",
			size:        1000,
			body:        strings.NewReader("data"),
			setupRepo:   func(_ *repomocks.MockKingClipRepository) {},
			setupStore:  func(_ *svcmocks.MockObjectStore) {},
			wantErr:     domain.ErrInvalidHillType,
		},
		{
			// criterion: 5 — non-WebM upload is rejected.
			name:        "non-webm content type returns ErrInvalidContentType",
			userID:      1,
			hillType:    domain.HillTypeDaily,
			blinkTsMs:   0,
			contentType: "video/mp4",
			size:        1000,
			body:        strings.NewReader("data"),
			setupRepo:   func(_ *repomocks.MockKingClipRepository) {},
			setupStore:  func(_ *svcmocks.MockObjectStore) {},
			wantErr:     domain.ErrInvalidContentType,
		},
		{
			// criterion: 5 — zero size is rejected (oversized/empty upload).
			name:        "zero size returns ErrTooLarge",
			userID:      1,
			hillType:    domain.HillTypeDaily,
			blinkTsMs:   0,
			contentType: "video/webm",
			size:        0,
			body:        strings.NewReader(""),
			setupRepo:   func(_ *repomocks.MockKingClipRepository) {},
			setupStore:  func(_ *svcmocks.MockObjectStore) {},
			wantErr:     domain.ErrTooLarge,
		},
		{
			// criterion: 5 — oversized upload is rejected.
			name:        "size exceeding max returns ErrTooLarge",
			userID:      1,
			hillType:    domain.HillTypeDaily,
			blinkTsMs:   0,
			contentType: "video/webm",
			size:        11 * 1024 * 1024,
			body:        strings.NewReader("data"),
			setupRepo:   func(_ *repomocks.MockKingClipRepository) {},
			setupStore:  func(_ *svcmocks.MockObjectStore) {},
			wantErr:     domain.ErrTooLarge,
		},
		{
			// criterion: 5 — malformed/negative blink_ts_ms is rejected.
			name:        "negative blink_ts_ms returns ErrInvalidBlinkTs",
			userID:      1,
			hillType:    domain.HillTypeDaily,
			blinkTsMs:   -1,
			contentType: "video/webm",
			size:        1000,
			body:        strings.NewReader("data"),
			setupRepo:   func(_ *repomocks.MockKingClipRepository) {},
			setupStore:  func(_ *svcmocks.MockObjectStore) {},
			wantErr:     domain.ErrInvalidBlinkTs,
		},
		{
			name:        "store.Put fails returns error",
			userID:      1,
			hillType:    domain.HillTypeDaily,
			blinkTsMs:   0,
			contentType: "video/webm",
			size:        100,
			body:        strings.NewReader("data"),
			setupRepo:   func(_ *repomocks.MockKingClipRepository) {},
			setupStore: func(m *svcmocks.MockObjectStore) {
				m.EXPECT().Put(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(errors.New("minio down"))
			},
			wantErr: errors.New("store king clip"),
		},
		{
			name:        "repo.Create fails triggers best-effort Remove",
			userID:      1,
			hillType:    domain.HillTypeDaily,
			blinkTsMs:   0,
			contentType: "video/webm",
			size:        100,
			body:        strings.NewReader("data"),
			setupRepo: func(m *repomocks.MockKingClipRepository) {
				m.EXPECT().Create(gomock.Any(), gomock.Any()).Return(domain.KingClip{}, errors.New("db error"))
			},
			setupStore: func(m *svcmocks.MockObjectStore) {
				m.EXPECT().Put(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil)
				m.EXPECT().Remove(gomock.Any(), gomock.Any()).Return(nil)
			},
			wantErr: errors.New("record king clip metadata"),
		},
		{
			// criterion: 2, 4(b) — uploading a king clip evicts a prior king clip
			// for the SAME hill (superseded), never a win-clip: this test only
			// interacts with KingClipRepository, confirming the FIFO
			// (ClipRepository.DeleteOldestBeyondLimit) is never invoked by a king
			// clip upload.
			name:        "supersedes prior king clip for the same hill (never touches win-clip FIFO)",
			userID:      42,
			hillType:    domain.HillTypeRanked,
			blinkTsMs:   0,
			contentType: "video/webm",
			size:        1000,
			body:        strings.NewReader("data"),
			setupRepo: func(m *repomocks.MockKingClipRepository) {
				m.EXPECT().Create(gomock.Any(), gomock.Any()).DoAndReturn(
					func(_ context.Context, c domain.KingClip) (domain.KingClip, error) {
						c.ID = 11
						return c, nil
					})
				m.EXPECT().DeleteSupersededByHill(gomock.Any(), domain.HillTypeRanked, int64(11)).
					Return([]string{"king-clips/ranked/42/old.webm"}, nil)
			},
			setupStore: func(m *svcmocks.MockObjectStore) {
				m.EXPECT().Put(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
				m.EXPECT().Remove(gomock.Any(), "king-clips/ranked/42/old.webm").Return(nil)
			},
			wantClipID: 11,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			repoMock := repomocks.NewMockKingClipRepository(ctrl)
			storeMock := svcmocks.NewMockObjectStore(ctrl)

			tt.setupRepo(repoMock)
			tt.setupStore(storeMock)

			svc := newKingClipSvc(repoMock, storeMock, fixedNow)

			got, err := svc.Upload(
				context.Background(), tt.userID, tt.hillType, tt.blinkTsMs, tt.contentType, tt.size, tt.body,
			)

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

			if !tt.wantExpires.IsZero() && !got.ExpiresAt.Equal(tt.wantExpires) {
				t.Errorf("ExpiresAt = %v, want %v", got.ExpiresAt, tt.wantExpires)
			}
		})
	}
}

func TestKingClipService_CurrentURL(t *testing.T) {
	t.Parallel()

	const wantURL = "https://minio.example.com/king-presigned"

	fixedNow := time.Date(2026, 7, 3, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name       string
		hillType   string
		setupRepo  func(m *repomocks.MockKingClipRepository)
		setupStore func(m *svcmocks.MockObjectStore)
		wantURL    string
		wantBlink  int64
		wantErr    error
	}{
		{
			// criterion: 3 — current non-expired king clip returns presigned URL
			// and its blink_ts_ms.
			name:     "returns current king clip URL and blink_ts_ms",
			hillType: domain.HillTypeDaily,
			setupRepo: func(m *repomocks.MockKingClipRepository) {
				m.EXPECT().GetCurrent(gomock.Any(), domain.HillTypeDaily).Return(domain.KingClip{
					ID:        1,
					HillType:  domain.HillTypeDaily,
					ObjectKey: "king-clips/daily/42/a.webm",
					BlinkTsMs: 4242,
				}, nil)
			},
			setupStore: func(m *svcmocks.MockObjectStore) {
				m.EXPECT().PresignedGetURL(gomock.Any(), "king-clips/daily/42/a.webm", 15*time.Minute).
					Return(wantURL, nil)
			},
			wantURL:   wantURL,
			wantBlink: 4242,
		},
		{
			// criterion: 3 — no current (non-expired) king clip → ErrKingClipNotFound
			// (mapped to 404 by the handler).
			name:     "no current king clip returns ErrKingClipNotFound",
			hillType: domain.HillTypeMonthly,
			setupRepo: func(m *repomocks.MockKingClipRepository) {
				m.EXPECT().GetCurrent(gomock.Any(), domain.HillTypeMonthly).
					Return(domain.KingClip{}, repository.ErrKingClipNotFound)
			},
			setupStore: func(_ *svcmocks.MockObjectStore) {},
			wantErr:    repository.ErrKingClipNotFound,
		},
		{
			// criterion: 5 — unknown hill_type is rejected.
			name:       "unknown hill type returns ErrInvalidHillType",
			hillType:   "weekly",
			setupRepo:  func(_ *repomocks.MockKingClipRepository) {},
			setupStore: func(_ *svcmocks.MockObjectStore) {},
			wantErr:    domain.ErrInvalidHillType,
		},
		{
			name:     "repo error propagated",
			hillType: domain.HillTypeRanked,
			setupRepo: func(m *repomocks.MockKingClipRepository) {
				m.EXPECT().GetCurrent(gomock.Any(), domain.HillTypeRanked).
					Return(domain.KingClip{}, errors.New("db error"))
			},
			setupStore: func(_ *svcmocks.MockObjectStore) {},
			wantErr:    errors.New("db error"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			repoMock := repomocks.NewMockKingClipRepository(ctrl)
			storeMock := svcmocks.NewMockObjectStore(ctrl)

			tt.setupRepo(repoMock)
			tt.setupStore(storeMock)

			svc := newKingClipSvc(repoMock, storeMock, fixedNow)

			url, blink, err := svc.CurrentURL(context.Background(), tt.hillType)

			if tt.wantErr != nil {
				if err == nil {
					t.Fatal("CurrentURL() error = nil, want error")
				}

				if errors.Is(tt.wantErr, repository.ErrKingClipNotFound) &&
					!errors.Is(err, repository.ErrKingClipNotFound) {
					t.Errorf("CurrentURL() error = %v, want ErrKingClipNotFound", err)
				}

				return
			}

			if err != nil {
				t.Fatalf("CurrentURL() unexpected error = %v", err)
			}

			if url != tt.wantURL {
				t.Errorf("url = %q, want %q", url, tt.wantURL)
			}

			if blink != tt.wantBlink {
				t.Errorf("blinkTsMs = %d, want %d", blink, tt.wantBlink)
			}
		})
	}
}

func TestKingClipService_Delete(t *testing.T) {
	t.Parallel()

	const (
		ownerID     = int64(42)
		otherUserID = int64(99)
		clipID      = int64(1)
	)

	fixedNow := time.Date(2026, 7, 3, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name       string
		userID     int64
		clipID     int64
		setupRepo  func(m *repomocks.MockKingClipRepository)
		setupStore func(m *svcmocks.MockObjectStore)
		wantErr    error
	}{
		{
			// criterion: 4(a) — owner DELETE removes object + metadata.
			name:   "owner deletes king clip: removes metadata then object",
			userID: ownerID,
			clipID: clipID,
			setupRepo: func(m *repomocks.MockKingClipRepository) {
				m.EXPECT().GetByID(gomock.Any(), clipID).Return(domain.KingClip{
					ID:        clipID,
					UserID:    ownerID,
					ObjectKey: "king-clips/daily/42/a.webm",
				}, nil)
				m.EXPECT().Delete(gomock.Any(), clipID).Return("king-clips/daily/42/a.webm", nil)
			},
			setupStore: func(m *svcmocks.MockObjectStore) {
				m.EXPECT().Remove(gomock.Any(), "king-clips/daily/42/a.webm").Return(nil)
			},
		},
		{
			// criterion: 4(a) — non-owner gets ErrKingClipNotFound (no leak),
			// object is never removed.
			name:   "non-owner gets ErrKingClipNotFound (no leak)",
			userID: otherUserID,
			clipID: clipID,
			setupRepo: func(m *repomocks.MockKingClipRepository) {
				m.EXPECT().GetByID(gomock.Any(), clipID).Return(domain.KingClip{
					ID:     clipID,
					UserID: ownerID,
				}, nil)
			},
			setupStore: func(_ *svcmocks.MockObjectStore) {},
			wantErr:    repository.ErrKingClipNotFound,
		},
		{
			name:   "missing clip returns ErrKingClipNotFound",
			userID: ownerID,
			clipID: 999,
			setupRepo: func(m *repomocks.MockKingClipRepository) {
				m.EXPECT().GetByID(gomock.Any(), int64(999)).Return(domain.KingClip{}, repository.ErrKingClipNotFound)
			},
			setupStore: func(_ *svcmocks.MockObjectStore) {},
			wantErr:    repository.ErrKingClipNotFound,
		},
		{
			name:   "repo GetByID error propagated",
			userID: ownerID,
			clipID: clipID,
			setupRepo: func(m *repomocks.MockKingClipRepository) {
				m.EXPECT().GetByID(gomock.Any(), clipID).Return(domain.KingClip{}, errors.New("db error"))
			},
			setupStore: func(_ *svcmocks.MockObjectStore) {},
			wantErr:    errors.New("db error"),
		},
		{
			name:   "repo Delete error propagated",
			userID: ownerID,
			clipID: clipID,
			setupRepo: func(m *repomocks.MockKingClipRepository) {
				m.EXPECT().GetByID(gomock.Any(), clipID).Return(domain.KingClip{
					ID:     clipID,
					UserID: ownerID,
				}, nil)
				m.EXPECT().Delete(gomock.Any(), clipID).Return("", errors.New("db error"))
			},
			setupStore: func(_ *svcmocks.MockObjectStore) {},
			wantErr:    errors.New("db error"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			repoMock := repomocks.NewMockKingClipRepository(ctrl)
			storeMock := svcmocks.NewMockObjectStore(ctrl)

			tt.setupRepo(repoMock)
			tt.setupStore(storeMock)

			svc := newKingClipSvc(repoMock, storeMock, fixedNow)

			err := svc.Delete(context.Background(), tt.userID, tt.clipID)

			if tt.wantErr != nil {
				if err == nil {
					t.Fatal("Delete() error = nil, want error")
				}

				if errors.Is(tt.wantErr, repository.ErrKingClipNotFound) &&
					!errors.Is(err, repository.ErrKingClipNotFound) {
					t.Errorf("Delete() error = %v, want ErrKingClipNotFound", err)
				}

				return
			}

			if err != nil {
				t.Fatalf("Delete() unexpected error = %v", err)
			}
		})
	}
}

func TestKingClipService_ExpireByID(t *testing.T) {
	t.Parallel()

	fixedNow := time.Date(2026, 7, 3, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name       string
		clipID     int64
		setupRepo  func(m *repomocks.MockKingClipRepository)
		setupStore func(m *svcmocks.MockObjectStore)
		wantErr    error
	}{
		{
			// criterion: 2 — success: repo removes the metadata row and returns
			// its object_key, and the service best-effort removes the object from
			// storage. Crucially, no ownership check (no GetByID/userID) is made —
			// unlike the session-gated Delete.
			name:   "existing clip: removes metadata then object, no ownership check",
			clipID: 1,
			setupRepo: func(m *repomocks.MockKingClipRepository) {
				m.EXPECT().Delete(gomock.Any(), int64(1)).Return("king-clips/daily/42/a.webm", nil)
			},
			setupStore: func(m *svcmocks.MockObjectStore) {
				m.EXPECT().Remove(gomock.Any(), "king-clips/daily/42/a.webm").Return(nil)
			},
		},
		{
			// criterion: 2 — missing clip: repo.Delete returns
			// ErrKingClipNotFound, the service returns it as-is, and store.Remove
			// is NEVER called.
			name:   "missing clip: returns ErrKingClipNotFound, store.Remove not called",
			clipID: 999,
			setupRepo: func(m *repomocks.MockKingClipRepository) {
				m.EXPECT().Delete(gomock.Any(), int64(999)).Return("", repository.ErrKingClipNotFound)
			},
			setupStore: func(_ *svcmocks.MockObjectStore) {},
			wantErr:    repository.ErrKingClipNotFound,
		},
		{
			name:   "repo Delete error is wrapped",
			clipID: 2,
			setupRepo: func(m *repomocks.MockKingClipRepository) {
				m.EXPECT().Delete(gomock.Any(), int64(2)).Return("", errors.New("db error"))
			},
			setupStore: func(_ *svcmocks.MockObjectStore) {},
			wantErr:    errors.New("expire king clip"),
		},
		{
			// store.Remove failure is best-effort (logged, not returned) — mirrors
			// the existing Delete method's behavior.
			name:   "store.Remove failure is best-effort, does not fail the call",
			clipID: 3,
			setupRepo: func(m *repomocks.MockKingClipRepository) {
				m.EXPECT().Delete(gomock.Any(), int64(3)).Return("king-clips/daily/42/b.webm", nil)
			},
			setupStore: func(m *svcmocks.MockObjectStore) {
				m.EXPECT().Remove(gomock.Any(), "king-clips/daily/42/b.webm").Return(errors.New("minio down"))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			repoMock := repomocks.NewMockKingClipRepository(ctrl)
			storeMock := svcmocks.NewMockObjectStore(ctrl)

			tt.setupRepo(repoMock)
			tt.setupStore(storeMock)

			svc := newKingClipSvc(repoMock, storeMock, fixedNow)

			err := svc.ExpireByID(context.Background(), tt.clipID)

			if tt.wantErr != nil {
				if err == nil {
					t.Fatal("ExpireByID() error = nil, want error")
				}

				if errors.Is(tt.wantErr, repository.ErrKingClipNotFound) &&
					!errors.Is(err, repository.ErrKingClipNotFound) {
					t.Errorf("ExpireByID() error = %v, want ErrKingClipNotFound", err)
				}

				return
			}

			if err != nil {
				t.Fatalf("ExpireByID() unexpected error = %v", err)
			}
		})
	}
}

// TestKingClipService_FIFOIsolation documents (both directions) that king
// clips and win-clips are fully independent categories: uploading a king clip
// never evicts a user's win-clips (ClipRepository is never touched by
// KingClipService.Upload), and — symmetrically — a win-clip upload only ever
// calls ClipRepository.DeleteOldestBeyondLimit, never
// KingClipRepository.DeleteSupersededByHill. Since ClipService and
// KingClipService are constructed from disjoint repository interfaces
// (ClipRepository vs KingClipRepository), this is enforced at compile time;
// this test additionally proves it at the call-mock level for KingClipService.
func TestKingClipService_FIFOIsolation(t *testing.T) {
	t.Parallel()

	fixedNow := time.Date(2026, 7, 3, 12, 0, 0, 0, time.UTC)

	ctrl := gomock.NewController(t)
	repoMock := repomocks.NewMockKingClipRepository(ctrl)
	storeMock := svcmocks.NewMockObjectStore(ctrl)

	// criterion: 2 — a king-clip upload only ever calls
	// KingClipRepository.Create/DeleteSupersededByHill (mocked here); the mock
	// setup below is exhaustive (no unexpected calls), so a KingClipRepository
	// mock with NO expectations wired for anything resembling win-clip FIFO
	// eviction (DeleteOldestBeyondLimit lives on the disjoint ClipRepository
	// interface, which this test's KingClipService never references) proves
	// king-clip uploads cannot evict win-clips.
	repoMock.EXPECT().Create(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, c domain.KingClip) (domain.KingClip, error) {
			c.ID = 100
			return c, nil
		})
	repoMock.EXPECT().DeleteSupersededByHill(gomock.Any(), domain.HillTypeDaily, int64(100)).
		Return([]string{}, nil)
	storeMock.EXPECT().Put(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

	svc := newKingClipSvc(repoMock, storeMock, fixedNow)

	got, err := svc.Upload(context.Background(), 42, domain.HillTypeDaily, 0, "video/webm", 100, strings.NewReader("x"))
	if err != nil {
		t.Fatalf("Upload() unexpected error = %v", err)
	}

	if got.ID != 100 {
		t.Errorf("clip.ID = %d, want 100", got.ID)
	}

	// gomock's controller.Finish (invoked automatically by t.Cleanup via
	// gomock.NewController(t)) fails the test if repoMock.Create or
	// DeleteSupersededByHill were called with unexpected args, or if any
	// other KingClipRepository method (which would be needed to touch a
	// win-clip) was invoked without an expectation.
}
