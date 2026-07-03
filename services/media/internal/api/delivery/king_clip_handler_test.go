package delivery_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"go.uber.org/mock/gomock"
	"go.uber.org/zap"

	"github.com/pizdagladki/full/services/media/internal/api/delivery"
	"github.com/pizdagladki/full/services/media/internal/api/domain"
	"github.com/pizdagladki/full/services/media/internal/api/repository"
	svcmocks "github.com/pizdagladki/full/services/media/internal/api/service/mocks"
)

func TestKingClipHandler_Upload(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		contentType string
		hillType    string
		blinkTsMs   string
		bodySize    int64
		body        string
		setUserID   bool
		setupSvc    func(m *svcmocks.MockKingClipService)
		wantStatus  int
	}{
		{
			// criterion: 1 — valid webm upload with hill_type + blink_ts_ms
			// returns 201 with the clip id.
			name:        "201 on valid webm upload",
			contentType: "video/webm",
			hillType:    domain.HillTypeDaily,
			blinkTsMs:   "1500",
			bodySize:    9,
			body:        "fakevideo",
			setUserID:   true,
			setupSvc: func(m *svcmocks.MockKingClipService) {
				m.EXPECT().Upload(gomock.Any(), testUserID, domain.HillTypeDaily, int64(1500), "video/webm", int64(9), gomock.Any()).
					Return(domain.KingClip{ID: 5, UserID: testUserID}, nil)
			},
			wantStatus: http.StatusCreated,
		},
		{
			// criterion: 5 — non-WebM upload → 400.
			name:        "400 on non-webm content type",
			contentType: "video/mp4",
			hillType:    domain.HillTypeDaily,
			blinkTsMs:   "0",
			bodySize:    100,
			body:        "data",
			setUserID:   true,
			setupSvc:    func(_ *svcmocks.MockKingClipService) {},
			wantStatus:  http.StatusBadRequest,
		},
		{
			// criterion: 5 — oversized upload (zero content length here stands
			// in for a rejected size) → 400.
			name:        "400 on zero content length",
			contentType: "video/webm",
			hillType:    domain.HillTypeDaily,
			blinkTsMs:   "0",
			bodySize:    0,
			body:        "",
			setUserID:   true,
			setupSvc:    func(_ *svcmocks.MockKingClipService) {},
			wantStatus:  http.StatusBadRequest,
		},
		{
			// criterion: 5 — oversized upload → 400.
			name:        "400 on content length exceeding max",
			contentType: "video/webm",
			hillType:    domain.HillTypeDaily,
			blinkTsMs:   "0",
			bodySize:    testMaxBytes + 1,
			body:        "data",
			setUserID:   true,
			setupSvc:    func(_ *svcmocks.MockKingClipService) {},
			wantStatus:  http.StatusBadRequest,
		},
		{
			// criterion: 5 — unknown hill_type → 400 (rejected before calling the
			// service).
			name:        "400 on unknown hill_type",
			contentType: "video/webm",
			hillType:    "weekly",
			blinkTsMs:   "0",
			bodySize:    9,
			body:        "fakevideo",
			setUserID:   true,
			setupSvc:    func(_ *svcmocks.MockKingClipService) {},
			wantStatus:  http.StatusBadRequest,
		},
		{
			// criterion: 5 — malformed blink_ts_ms → 400.
			name:        "400 on malformed blink_ts_ms",
			contentType: "video/webm",
			hillType:    domain.HillTypeDaily,
			blinkTsMs:   "not-a-number",
			bodySize:    9,
			body:        "fakevideo",
			setUserID:   true,
			setupSvc:    func(_ *svcmocks.MockKingClipService) {},
			wantStatus:  http.StatusBadRequest,
		},
		{
			// criterion: 5 — negative blink_ts_ms → 400.
			name:        "400 on negative blink_ts_ms",
			contentType: "video/webm",
			hillType:    domain.HillTypeDaily,
			blinkTsMs:   "-5",
			bodySize:    9,
			body:        "fakevideo",
			setUserID:   true,
			setupSvc:    func(_ *svcmocks.MockKingClipService) {},
			wantStatus:  http.StatusBadRequest,
		},
		{
			name:        "400 when service returns ErrInvalidHillType",
			contentType: "video/webm",
			hillType:    domain.HillTypeDaily,
			blinkTsMs:   "0",
			bodySize:    9,
			body:        "fakevideo",
			setUserID:   true,
			setupSvc: func(m *svcmocks.MockKingClipService) {
				m.EXPECT().Upload(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(domain.KingClip{}, domain.ErrInvalidHillType)
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:        "400 when service returns ErrTooLarge",
			contentType: "video/webm",
			hillType:    domain.HillTypeDaily,
			blinkTsMs:   "0",
			bodySize:    9,
			body:        "fakevideo",
			setUserID:   true,
			setupSvc: func(m *svcmocks.MockKingClipService) {
				m.EXPECT().Upload(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(domain.KingClip{}, domain.ErrTooLarge)
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:        "500 on unexpected service error",
			contentType: "video/webm",
			hillType:    domain.HillTypeDaily,
			blinkTsMs:   "0",
			bodySize:    9,
			body:        "fakevideo",
			setUserID:   true,
			setupSvc: func(m *svcmocks.MockKingClipService) {
				m.EXPECT().Upload(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(domain.KingClip{}, errors.New("minio down"))
			},
			wantStatus: http.StatusInternalServerError,
		},
		{
			// criterion: 5 — unauthenticated → 401.
			name:        "401 when user id not in context",
			contentType: "video/webm",
			hillType:    domain.HillTypeDaily,
			blinkTsMs:   "0",
			bodySize:    9,
			body:        "fakevideo",
			setUserID:   false,
			setupSvc:    func(_ *svcmocks.MockKingClipService) {},
			wantStatus:  http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			svcMock := svcmocks.NewMockKingClipService(ctrl)
			tt.setupSvc(svcMock)

			h := delivery.NewKingClipHandler(svcMock, testMaxBytes, zap.NewNop())

			e := newEcho()
			body := strings.NewReader(tt.body)

			q := url.Values{}
			q.Set("hill_type", tt.hillType)
			q.Set("blink_ts_ms", tt.blinkTsMs)

			req := httptest.NewRequest(http.MethodPost, "/v1/king-clips?"+q.Encode(), body)
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

func TestKingClipHandler_Current(t *testing.T) {
	t.Parallel()

	const wantURL = "https://minio.example.com/king-presigned"

	tests := []struct {
		name       string
		hillType   string
		setUserID  bool
		setupSvc   func(m *svcmocks.MockKingClipService)
		wantStatus int
		wantURL    string
		wantBlink  int64
	}{
		{
			// criterion: 3 — current non-expired king clip returns a presigned
			// URL plus blink_ts_ms.
			name:      "200 with current king clip URL and blink_ts_ms",
			hillType:  domain.HillTypeDaily,
			setUserID: true,
			setupSvc: func(m *svcmocks.MockKingClipService) {
				m.EXPECT().CurrentURL(gomock.Any(), domain.HillTypeDaily).Return(wantURL, int64(777), nil)
			},
			wantStatus: http.StatusOK,
			wantURL:    wantURL,
			wantBlink:  777,
		},
		{
			// criterion: 3, 4(c) — none available (including all-expired) → 404.
			name:      "404 when no current king clip",
			hillType:  domain.HillTypeMonthly,
			setUserID: true,
			setupSvc: func(m *svcmocks.MockKingClipService) {
				m.EXPECT().CurrentURL(gomock.Any(), domain.HillTypeMonthly).
					Return("", int64(0), repository.ErrKingClipNotFound)
			},
			wantStatus: http.StatusNotFound,
		},
		{
			// criterion: 5 — unknown hill_type → 400.
			name:      "400 on unknown hill_type",
			hillType:  "weekly",
			setUserID: true,
			setupSvc: func(m *svcmocks.MockKingClipService) {
				m.EXPECT().CurrentURL(gomock.Any(), "weekly").Return("", int64(0), domain.ErrInvalidHillType)
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:      "500 on unexpected service error",
			hillType:  domain.HillTypeRanked,
			setUserID: true,
			setupSvc: func(m *svcmocks.MockKingClipService) {
				m.EXPECT().CurrentURL(gomock.Any(), domain.HillTypeRanked).
					Return("", int64(0), errors.New("db error"))
			},
			wantStatus: http.StatusInternalServerError,
		},
		{
			// criterion: 5 — unauthenticated → 401.
			name:       "401 when user id not in context",
			hillType:   domain.HillTypeDaily,
			setUserID:  false,
			setupSvc:   func(_ *svcmocks.MockKingClipService) {},
			wantStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			svcMock := svcmocks.NewMockKingClipService(ctrl)
			tt.setupSvc(svcMock)

			h := delivery.NewKingClipHandler(svcMock, testMaxBytes, zap.NewNop())

			e := newEcho()
			req := httptest.NewRequest(http.MethodGet, "/v1/king-clips/current?hill_type="+tt.hillType, nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			if tt.setUserID {
				ctxWithUser(c, testUserID)
			}

			_ = h.Current(c)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d (body: %s)", rec.Code, tt.wantStatus, rec.Body.String())
			}

			if tt.wantURL != "" {
				var resp map[string]interface{}
				if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
					t.Fatalf("decode body: %v", err)
				}

				if resp["download_url"] != tt.wantURL {
					t.Errorf("download_url = %v, want %q", resp["download_url"], tt.wantURL)
				}

				if int64(resp["blink_ts_ms"].(float64)) != tt.wantBlink {
					t.Errorf("blink_ts_ms = %v, want %d", resp["blink_ts_ms"], tt.wantBlink)
				}
			}
		})
	}
}

func TestKingClipHandler_Delete(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		idParam    string
		setUserID  bool
		setupSvc   func(m *svcmocks.MockKingClipService)
		wantStatus int
	}{
		{
			// criterion: 4(a) — owner DELETE removes object + metadata; 204.
			name:      "204 when owner deletes",
			idParam:   "1",
			setUserID: true,
			setupSvc: func(m *svcmocks.MockKingClipService) {
				m.EXPECT().Delete(gomock.Any(), testUserID, int64(1)).Return(nil)
			},
			wantStatus: http.StatusNoContent,
		},
		{
			name:      "404 when clip not found or foreign",
			idParam:   "999",
			setUserID: true,
			setupSvc: func(m *svcmocks.MockKingClipService) {
				m.EXPECT().Delete(gomock.Any(), testUserID, int64(999)).Return(repository.ErrKingClipNotFound)
			},
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "400 on bad id param",
			idParam:    "notanumber",
			setUserID:  true,
			setupSvc:   func(_ *svcmocks.MockKingClipService) {},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:      "500 on unexpected service error",
			idParam:   "1",
			setUserID: true,
			setupSvc: func(m *svcmocks.MockKingClipService) {
				m.EXPECT().Delete(gomock.Any(), gomock.Any(), gomock.Any()).Return(errors.New("db error"))
			},
			wantStatus: http.StatusInternalServerError,
		},
		{
			// criterion: 5 — unauthenticated → 401.
			name:       "401 when user id not in context",
			idParam:    "1",
			setUserID:  false,
			setupSvc:   func(_ *svcmocks.MockKingClipService) {},
			wantStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			svcMock := svcmocks.NewMockKingClipService(ctrl)
			tt.setupSvc(svcMock)

			h := delivery.NewKingClipHandler(svcMock, testMaxBytes, zap.NewNop())

			e := newEcho()
			req := httptest.NewRequest(http.MethodDelete, "/v1/king-clips/"+tt.idParam, nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.SetParamNames("id")
			c.SetParamValues(tt.idParam)

			if tt.setUserID {
				ctxWithUser(c, testUserID)
			}

			_ = h.Delete(c)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d (body: %s)", rec.Code, tt.wantStatus, rec.Body.String())
			}
		})
	}
}
