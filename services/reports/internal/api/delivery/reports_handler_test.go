package delivery_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"

	"github.com/go-playground/validator/v10"
	"github.com/labstack/echo/v4"
	"go.uber.org/mock/gomock"
	"go.uber.org/zap"

	"github.com/pizdagladki/full/services/reports/internal/api/delivery"
	"github.com/pizdagladki/full/services/reports/internal/api/domain"
	"github.com/pizdagladki/full/services/reports/internal/api/service"
	svcmocks "github.com/pizdagladki/full/services/reports/internal/api/service/mocks"
)

// echoValidator wraps validator/v10 to satisfy echo.Validator.
type echoValidator struct{ v *validator.Validate }

func (ev *echoValidator) Validate(i any) error { return ev.v.Struct(i) }

func newEcho(h delivery.ReportsHandler) *echo.Echo {
	e := echo.New()
	e.Validator = &echoValidator{v: validator.New()}
	e.HideBanner = true

	e.POST("/v1/reports/cheat", h.PostCheatReport)
	e.GET("/v1/reports/cooldown/:user_id", h.GetCooldown)
	// Bug report route is protected; tests set the context key manually.
	e.POST("/v1/reports/bug", h.PostBugReport)

	return e
}

func newHandler(t *testing.T) (*svcmocks.MockReportsService, *svcmocks.MockBugReportService, delivery.ReportsHandler) {
	t.Helper()

	ctrl := gomock.NewController(t)
	svc := svcmocks.NewMockReportsService(ctrl)
	bugSvc := svcmocks.NewMockBugReportService(ctrl)
	h := delivery.NewReportsHandler(svc, bugSvc, 500*1024*1024, zap.NewNop())

	return svc, bugSvc, h
}

func TestPostCheatReport(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		body       string
		setupMock  func(svc *svcmocks.MockReportsService)
		wantStatus int
	}{
		{
			// criterion: 1 — valid report returns 201
			name: "valid report returns 201",
			body: `{"reporter_id":1,"reported_id":2,"match_id":"m1"}`,
			setupMock: func(svc *svcmocks.MockReportsService) {
				svc.EXPECT().
					ReportCheat(gomock.Any(), int64(1), int64(2), "m1").
					Return(nil)
			},
			wantStatus: http.StatusCreated,
		},
		{
			// criterion: 1 — documented body without reporter_id returns 201
			name: "documented body without reporter_id returns 201",
			body: `{"reported_id":2,"match_id":"m1"}`,
			setupMock: func(svc *svcmocks.MockReportsService) {
				svc.EXPECT().
					ReportCheat(gomock.Any(), int64(0), int64(2), "m1").
					Return(nil)
			},
			wantStatus: http.StatusCreated,
		},
		{
			// criterion: 4 — self-report returns 400
			name: "self-report returns 400",
			body: `{"reporter_id":1,"reported_id":1,"match_id":"m1"}`,
			setupMock: func(svc *svcmocks.MockReportsService) {
				svc.EXPECT().
					ReportCheat(gomock.Any(), int64(1), int64(1), "m1").
					Return(service.ErrSelfReport)
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			// criterion: 4 — malformed JSON returns 400
			name:       "malformed body returns 400",
			body:       `{not valid json`,
			setupMock:  func(svc *svcmocks.MockReportsService) {},
			wantStatus: http.StatusBadRequest,
		},
		{
			// criterion: 4 — missing required field returns 400
			name:       "missing match_id returns 400",
			body:       `{"reporter_id":1,"reported_id":2}`,
			setupMock:  func(svc *svcmocks.MockReportsService) {},
			wantStatus: http.StatusBadRequest,
		},
		{
			// criterion: 1 — idempotent re-report returns 201
			name: "idempotent re-report returns 201",
			body: `{"reporter_id":1,"reported_id":2,"match_id":"m1"}`,
			setupMock: func(svc *svcmocks.MockReportsService) {
				svc.EXPECT().
					ReportCheat(gomock.Any(), int64(1), int64(2), "m1").
					Return(nil) // ON CONFLICT DO NOTHING — service returns nil
			},
			wantStatus: http.StatusCreated,
		},
		{
			// criterion: 5 — internal service error returns 500
			name: "service error returns 500",
			body: `{"reporter_id":1,"reported_id":2,"match_id":"m1"}`,
			setupMock: func(svc *svcmocks.MockReportsService) {
				svc.EXPECT().
					ReportCheat(gomock.Any(), int64(1), int64(2), "m1").
					Return(errors.New("db down"))
			},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			svc, _, h := newHandler(t)
			tt.setupMock(svc)

			e := newEcho(h)
			req := httptest.NewRequest(http.MethodPost, "/v1/reports/cheat", strings.NewReader(tt.body))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d; body = %s", rec.Code, tt.wantStatus, rec.Body.String())
			}
		})
	}
}

func TestGetCooldown(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		userID     string
		setupMock  func(svc *svcmocks.MockReportsService)
		wantStatus int
		wantActive bool
		wantSecs   int
	}{
		{
			// criterion: 3 — cooldown active returns 200 with seconds
			name:   "cooldown active returns 200",
			userID: "42",
			setupMock: func(svc *svcmocks.MockReportsService) {
				svc.EXPECT().
					GetCooldown(gomock.Any(), int64(42)).
					Return(domain.CooldownStatus{Active: true, SecondsRemaining: 500}, nil)
			},
			wantStatus: http.StatusOK,
			wantActive: true,
			wantSecs:   500,
		},
		{
			// criterion: 3 — cooldown inactive returns 200 with active=false
			name:   "cooldown inactive returns 200",
			userID: "42",
			setupMock: func(svc *svcmocks.MockReportsService) {
				svc.EXPECT().
					GetCooldown(gomock.Any(), int64(42)).
					Return(domain.CooldownStatus{Active: false, SecondsRemaining: 0}, nil)
			},
			wantStatus: http.StatusOK,
			wantActive: false,
			wantSecs:   0,
		},
		{
			// criterion: 3 — invalid user_id returns 400
			name:       "invalid user_id returns 400",
			userID:     "not-a-number",
			setupMock:  func(svc *svcmocks.MockReportsService) {},
			wantStatus: http.StatusBadRequest,
		},
		{
			// criterion: 3 — service error returns 500
			name:   "service error returns 500",
			userID: "42",
			setupMock: func(svc *svcmocks.MockReportsService) {
				svc.EXPECT().
					GetCooldown(gomock.Any(), int64(42)).
					Return(domain.CooldownStatus{}, errors.New("redis down"))
			},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			svc, _, h := newHandler(t)
			tt.setupMock(svc)

			e := newEcho(h)
			req := httptest.NewRequest(http.MethodGet, "/v1/reports/cooldown/"+tt.userID, nil)
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d; body = %s", rec.Code, tt.wantStatus, rec.Body.String())
			}

			if tt.wantStatus == http.StatusOK {
				var resp struct {
					Active           bool `json:"active"`
					SecondsRemaining int  `json:"seconds_remaining"`
				}
				if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
					t.Fatalf("decode response: %v", err)
				}

				if resp.Active != tt.wantActive {
					t.Errorf("active = %v, want %v", resp.Active, tt.wantActive)
				}

				if resp.SecondsRemaining != tt.wantSecs {
					t.Errorf("seconds_remaining = %d, want %d", resp.SecondsRemaining, tt.wantSecs)
				}
			}
		})
	}
}

// buildWebmMultipart creates a multipart form body with a "device=pc" field
// and a "recording" file part whose Content-Type is set to ct. Returns the
// body bytes and the multipart content-type header value (including boundary).
func buildWebmMultipart(t *testing.T, fieldName, ct string, content []byte) (*bytes.Buffer, string) {
	t.Helper()

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	// Add device field.
	if err := w.WriteField("device", "pc"); err != nil {
		t.Fatalf("write device field: %v", err)
	}

	// Create the file part with an explicit Content-Type header.
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", `form-data; name="`+fieldName+`"; filename="rec.webm"`)
	h.Set("Content-Type", ct)

	part, err := w.CreatePart(h)
	if err != nil {
		t.Fatalf("create part: %v", err)
	}

	if _, err = part.Write(content); err != nil {
		t.Fatalf("write part: %v", err)
	}

	if err = w.Close(); err != nil {
		t.Fatalf("close multipart: %v", err)
	}

	return &buf, w.FormDataContentType()
}

func TestPostBugReport(t *testing.T) {
	t.Parallel()

	webmContent := []byte("RIFF....WEBM")
	const maxBytes = int64(10 * 1024 * 1024) // 10 MiB for tests

	tests := []struct {
		name       string
		setupReq   func(t *testing.T) (*http.Request, int64 /* maxBytes override */)
		setupCtx   func(c echo.Context)
		setupMock  func(bugSvc *svcmocks.MockBugReportService)
		wantStatus int
	}{
		{
			// criterion: 1 — mobile report authenticated → 201
			name: "mobile report authenticated returns 201",
			setupReq: func(t *testing.T) (*http.Request, int64) {
				t.Helper()
				var buf bytes.Buffer
				w := multipart.NewWriter(&buf)
				_ = w.WriteField("device", "mobile")
				_ = w.WriteField("description", "app crash")
				_ = w.Close()
				req := httptest.NewRequest(http.MethodPost, "/v1/reports/bug", &buf)
				req.Header.Set(echo.HeaderContentType, w.FormDataContentType())
				return req, maxBytes
			},
			setupCtx: func(c echo.Context) { c.Set(delivery.UserIDContextKey, int64(42)) },
			setupMock: func(bugSvc *svcmocks.MockBugReportService) {
				bugSvc.EXPECT().ReportBug(gomock.Any(), int64(42), "mobile", "app crash", []byte(nil)).Return(nil)
			},
			wantStatus: http.StatusCreated,
		},
		{
			// criterion: 2 — pc report with webm → 201
			name: "pc report with webm returns 201",
			setupReq: func(t *testing.T) (*http.Request, int64) {
				t.Helper()
				body, ct := buildWebmMultipart(t, "recording", "video/webm", webmContent)
				req := httptest.NewRequest(http.MethodPost, "/v1/reports/bug", body)
				req.Header.Set(echo.HeaderContentType, ct)
				return req, maxBytes
			},
			setupCtx: func(c echo.Context) { c.Set(delivery.UserIDContextKey, int64(7)) },
			setupMock: func(bugSvc *svcmocks.MockBugReportService) {
				bugSvc.EXPECT().ReportBug(gomock.Any(), int64(7), "pc", "", webmContent).Return(nil)
			},
			wantStatus: http.StatusCreated,
		},
		{
			// criterion: 5 — unauthenticated request returns 401
			name: "unauthenticated returns 401",
			setupReq: func(t *testing.T) (*http.Request, int64) {
				t.Helper()
				req := httptest.NewRequest(http.MethodPost, "/v1/reports/bug", strings.NewReader("device=mobile"))
				req.Header.Set(echo.HeaderContentType, "application/x-www-form-urlencoded")
				return req, maxBytes
			},
			setupCtx:   func(c echo.Context) { /* no user_id set */ },
			setupMock:  func(bugSvc *svcmocks.MockBugReportService) {},
			wantStatus: http.StatusUnauthorized,
		},
		{
			// criterion: 5 — invalid device returns 400
			name: "invalid device returns 400",
			setupReq: func(t *testing.T) (*http.Request, int64) {
				t.Helper()
				req := httptest.NewRequest(http.MethodPost, "/v1/reports/bug", strings.NewReader("device=tablet"))
				req.Header.Set(echo.HeaderContentType, "application/x-www-form-urlencoded")
				return req, maxBytes
			},
			setupCtx:   func(c echo.Context) { c.Set(delivery.UserIDContextKey, int64(1)) },
			setupMock:  func(bugSvc *svcmocks.MockBugReportService) {},
			wantStatus: http.StatusBadRequest,
		},
		{
			// criterion: 5 — pc missing recording returns 400
			name: "pc missing recording returns 400",
			setupReq: func(t *testing.T) (*http.Request, int64) {
				t.Helper()
				req := httptest.NewRequest(http.MethodPost, "/v1/reports/bug", strings.NewReader("device=pc"))
				req.Header.Set(echo.HeaderContentType, "application/x-www-form-urlencoded")
				return req, maxBytes
			},
			setupCtx:   func(c echo.Context) { c.Set(delivery.UserIDContextKey, int64(1)) },
			setupMock:  func(bugSvc *svcmocks.MockBugReportService) {},
			wantStatus: http.StatusBadRequest,
		},
		{
			// criterion: 5 — pc non-webm returns 400
			name: "pc non-webm returns 400",
			setupReq: func(t *testing.T) (*http.Request, int64) {
				t.Helper()
				body, ct := buildWebmMultipart(t, "recording", "video/mp4", webmContent)
				req := httptest.NewRequest(http.MethodPost, "/v1/reports/bug", body)
				req.Header.Set(echo.HeaderContentType, ct)
				return req, maxBytes
			},
			setupCtx:   func(c echo.Context) { c.Set(delivery.UserIDContextKey, int64(1)) },
			setupMock:  func(bugSvc *svcmocks.MockBugReportService) {},
			wantStatus: http.StatusBadRequest,
		},
		{
			// criterion: 5 — pc oversized returns 400
			name: "pc oversized returns 400",
			setupReq: func(t *testing.T) (*http.Request, int64) {
				t.Helper()
				// 5 bytes content but limit is 4 bytes → oversized
				body, ct := buildWebmMultipart(t, "recording", "video/webm", []byte("12345"))
				req := httptest.NewRequest(http.MethodPost, "/v1/reports/bug", body)
				req.Header.Set(echo.HeaderContentType, ct)
				return req, 4
			},
			setupCtx:   func(c echo.Context) { c.Set(delivery.UserIDContextKey, int64(1)) },
			setupMock:  func(bugSvc *svcmocks.MockBugReportService) {},
			wantStatus: http.StatusBadRequest,
		},
		{
			// service error returns 500
			name: "service error returns 500",
			setupReq: func(t *testing.T) (*http.Request, int64) {
				t.Helper()
				var buf bytes.Buffer
				w := multipart.NewWriter(&buf)
				_ = w.WriteField("device", "mobile")
				_ = w.Close()
				req := httptest.NewRequest(http.MethodPost, "/v1/reports/bug", &buf)
				req.Header.Set(echo.HeaderContentType, w.FormDataContentType())
				return req, maxBytes
			},
			setupCtx: func(c echo.Context) { c.Set(delivery.UserIDContextKey, int64(1)) },
			setupMock: func(bugSvc *svcmocks.MockBugReportService) {
				bugSvc.EXPECT().ReportBug(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(errors.New("db down"))
			},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			svc := svcmocks.NewMockReportsService(ctrl)
			bugSvc := svcmocks.NewMockBugReportService(ctrl)

			req, maxB := tt.setupReq(t)
			tt.setupMock(bugSvc)

			h := delivery.NewReportsHandler(svc, bugSvc, maxB, zap.NewNop())

			e := echo.New()
			e.Validator = &echoValidator{v: validator.New()}
			e.HideBanner = true

			// Register the route and inject context key before the handler runs.
			e.POST("/v1/reports/bug", func(c echo.Context) error {
				tt.setupCtx(c)
				return h.PostBugReport(c)
			})

			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d; body = %s", rec.Code, tt.wantStatus, rec.Body.String())
			}
		})
	}
}
