package delivery_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"go.uber.org/mock/gomock"
	"go.uber.org/zap"

	"github.com/pizdagladki/full/services/media/internal/api/delivery"
	"github.com/pizdagladki/full/services/media/internal/api/domain"
	"github.com/pizdagladki/full/services/media/internal/api/repository"
	svcmocks "github.com/pizdagladki/full/services/media/internal/api/service/mocks"
)

const (
	testMaxBytes = int64(10 * 1024 * 1024) // 10 MiB
	testUserID   = int64(42)
)

func newEcho() *echo.Echo {
	e := echo.New()
	e.HideBanner = true

	return e
}

func ctxWithUser(c echo.Context, userID int64) {
	c.Set(delivery.UserIDContextKey, userID)
}

func TestClipHandler_Upload(t *testing.T) {
	t.Parallel()

	now := time.Now()

	tests := []struct {
		name        string
		contentType string
		bodySize    int64
		body        string
		setUserID   bool
		setupSvc    func(m *svcmocks.MockClipService)
		wantStatus  int
	}{
		{
			name:        "201 on valid webm upload",
			contentType: "video/webm",
			bodySize:    9,
			body:        "fakevideo",
			setUserID:   true,
			setupSvc: func(m *svcmocks.MockClipService) {
				m.EXPECT().Upload(gomock.Any(), testUserID, "video/webm", int64(9), gomock.Any()).
					Return(domain.Clip{ID: 5, UserID: testUserID, CreatedAt: now}, nil)
			},
			wantStatus: http.StatusCreated,
		},
		{
			name:        "400 on non-webm content type",
			contentType: "video/mp4",
			bodySize:    100,
			body:        "data",
			setUserID:   true,
			setupSvc:    func(_ *svcmocks.MockClipService) {},
			wantStatus:  http.StatusBadRequest,
		},
		{
			name:        "400 on zero content length",
			contentType: "video/webm",
			bodySize:    0,
			body:        "",
			setUserID:   true,
			setupSvc:    func(_ *svcmocks.MockClipService) {},
			wantStatus:  http.StatusBadRequest,
		},
		{
			name:        "400 on content length exceeding max",
			contentType: "video/webm",
			bodySize:    testMaxBytes + 1,
			body:        "data",
			setUserID:   true,
			setupSvc:    func(_ *svcmocks.MockClipService) {},
			wantStatus:  http.StatusBadRequest,
		},
		{
			name:        "400 when service returns ErrInvalidContentType",
			contentType: "video/webm",
			bodySize:    9,
			body:        "fakevideo",
			setUserID:   true,
			setupSvc: func(m *svcmocks.MockClipService) {
				m.EXPECT().Upload(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(domain.Clip{}, domain.ErrInvalidContentType)
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:        "400 when service returns ErrTooLarge",
			contentType: "video/webm",
			bodySize:    9,
			body:        "fakevideo",
			setUserID:   true,
			setupSvc: func(m *svcmocks.MockClipService) {
				m.EXPECT().Upload(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(domain.Clip{}, domain.ErrTooLarge)
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:        "500 on unexpected service error",
			contentType: "video/webm",
			bodySize:    9,
			body:        "fakevideo",
			setUserID:   true,
			setupSvc: func(m *svcmocks.MockClipService) {
				m.EXPECT().Upload(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(domain.Clip{}, errors.New("minio down"))
			},
			wantStatus: http.StatusInternalServerError,
		},
		{
			name:        "401 when user id not in context",
			contentType: "video/webm",
			bodySize:    9,
			body:        "fakevideo",
			setUserID:   false,
			setupSvc:    func(_ *svcmocks.MockClipService) {},
			wantStatus:  http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			svcMock := svcmocks.NewMockClipService(ctrl)
			tt.setupSvc(svcMock)

			h := delivery.NewClipHandler(svcMock, testMaxBytes, zap.NewNop())

			e := newEcho()
			body := strings.NewReader(tt.body)
			req := httptest.NewRequest(http.MethodPost, "/v1/clips", body)
			req.Header.Set(echo.HeaderContentType, tt.contentType)
			req.ContentLength = tt.bodySize
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			if tt.setUserID {
				ctxWithUser(c, testUserID)
			}

			_ = h.Upload(c)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d (body: %s)", rec.Code, tt.wantStatus, rec.Body.String())
			}
		})
	}
}

func TestClipHandler_List(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC().Truncate(time.Second)

	tests := []struct {
		name       string
		setUserID  bool
		setupSvc   func(m *svcmocks.MockClipService)
		wantStatus int
		wantLen    int
	}{
		{
			name:      "200 with empty array when no clips",
			setUserID: true,
			setupSvc: func(m *svcmocks.MockClipService) {
				m.EXPECT().List(gomock.Any(), testUserID).Return([]domain.Clip{}, nil)
			},
			wantStatus: http.StatusOK,
			wantLen:    0,
		},
		{
			name:      "200 with clips newest first",
			setUserID: true,
			setupSvc: func(m *svcmocks.MockClipService) {
				m.EXPECT().List(gomock.Any(), testUserID).Return([]domain.Clip{
					{ID: 3, Mode: "default", Result: "win", CreatedAt: now},
					{ID: 1, Mode: "default", Result: "win", CreatedAt: now.Add(-time.Minute)},
				}, nil)
			},
			wantStatus: http.StatusOK,
			wantLen:    2,
		},
		{
			name:      "500 on service error",
			setUserID: true,
			setupSvc: func(m *svcmocks.MockClipService) {
				m.EXPECT().List(gomock.Any(), testUserID).Return(nil, errors.New("db error"))
			},
			wantStatus: http.StatusInternalServerError,
		},
		{
			name:       "401 when user id not in context",
			setUserID:  false,
			setupSvc:   func(_ *svcmocks.MockClipService) {},
			wantStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			svcMock := svcmocks.NewMockClipService(ctrl)
			tt.setupSvc(svcMock)

			h := delivery.NewClipHandler(svcMock, testMaxBytes, zap.NewNop())

			e := newEcho()
			req := httptest.NewRequest(http.MethodGet, "/v1/clips", nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			if tt.setUserID {
				ctxWithUser(c, testUserID)
			}

			_ = h.List(c)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d (body: %s)", rec.Code, tt.wantStatus, rec.Body.String())
			}

			if tt.wantStatus == http.StatusOK {
				// Verify array (never null) and ordering.
				var resp []map[string]interface{}
				if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
					t.Fatalf("decode body: %v (body=%q)", err, rec.Body.String())
				}

				if resp == nil {
					t.Error("response is null, want []")
				}

				if len(resp) != tt.wantLen {
					t.Errorf("len(resp) = %d, want %d", len(resp), tt.wantLen)
				}

				if tt.wantLen >= 2 {
					// Verify newest first: first item should have larger id.
					id0 := resp[0]["id"].(float64)
					id1 := resp[1]["id"].(float64)

					if id0 <= id1 {
						t.Errorf("clips not ordered newest first: id[0]=%v id[1]=%v", id0, id1)
					}
				}
			}
		})
	}
}

func TestClipHandler_Download(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		clipIDParam string
		setUserID   bool
		setupSvc    func(m *svcmocks.MockClipService)
		wantStatus  int
		wantURL     string
	}{
		{
			name:        "200 with download URL for owner",
			clipIDParam: "1",
			setUserID:   true,
			setupSvc: func(m *svcmocks.MockClipService) {
				m.EXPECT().DownloadURL(gomock.Any(), testUserID, int64(1)).
					Return("https://minio.example.com/presigned", nil)
			},
			wantStatus: http.StatusOK,
			wantURL:    "https://minio.example.com/presigned",
		},
		{
			name:        "404 when clip belongs to another user",
			clipIDParam: "2",
			setUserID:   true,
			setupSvc: func(m *svcmocks.MockClipService) {
				m.EXPECT().DownloadURL(gomock.Any(), testUserID, int64(2)).
					Return("", repository.ErrClipNotFound)
			},
			wantStatus: http.StatusNotFound,
		},
		{
			name:        "404 when clip not found",
			clipIDParam: "999",
			setUserID:   true,
			setupSvc: func(m *svcmocks.MockClipService) {
				m.EXPECT().DownloadURL(gomock.Any(), testUserID, int64(999)).
					Return("", repository.ErrClipNotFound)
			},
			wantStatus: http.StatusNotFound,
		},
		{
			name:        "400 on bad clip id param",
			clipIDParam: "notanumber",
			setUserID:   true,
			setupSvc:    func(_ *svcmocks.MockClipService) {},
			wantStatus:  http.StatusBadRequest,
		},
		{
			name:        "500 on unexpected service error",
			clipIDParam: "1",
			setUserID:   true,
			setupSvc: func(m *svcmocks.MockClipService) {
				m.EXPECT().DownloadURL(gomock.Any(), gomock.Any(), gomock.Any()).
					Return("", errors.New("minio error"))
			},
			wantStatus: http.StatusInternalServerError,
		},
		{
			name:        "401 when user id not in context",
			clipIDParam: "1",
			setUserID:   false,
			setupSvc:    func(_ *svcmocks.MockClipService) {},
			wantStatus:  http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			svcMock := svcmocks.NewMockClipService(ctrl)
			tt.setupSvc(svcMock)

			h := delivery.NewClipHandler(svcMock, testMaxBytes, zap.NewNop())

			e := newEcho()
			req := httptest.NewRequest(http.MethodGet, "/v1/clips/"+tt.clipIDParam+"/download", nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.SetParamNames("id")
			c.SetParamValues(tt.clipIDParam)

			if tt.setUserID {
				ctxWithUser(c, testUserID)
			}

			_ = h.Download(c)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d (body: %s)", rec.Code, tt.wantStatus, rec.Body.String())
			}

			if tt.wantURL != "" {
				var resp map[string]string
				if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
					t.Fatalf("decode body: %v", err)
				}

				if resp["download_url"] != tt.wantURL {
					t.Errorf("download_url = %q, want %q", resp["download_url"], tt.wantURL)
				}
			}
		})
	}
}
