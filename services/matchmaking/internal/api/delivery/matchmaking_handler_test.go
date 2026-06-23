package delivery

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"go.uber.org/mock/gomock"
	"go.uber.org/zap"

	"github.com/pizdagladki/full/services/matchmaking/internal/api/domain"
	repomocks "github.com/pizdagladki/full/services/matchmaking/internal/api/repository/mocks"
	"github.com/pizdagladki/full/services/matchmaking/internal/api/service"
	svcmocks "github.com/pizdagladki/full/services/matchmaking/internal/api/service/mocks"
)

// makeHTTPHandler wraps a MatchmakingHandler.ServeWS as an http.HandlerFunc.
func makeHTTPHandler(h MatchmakingHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		h.ServeWS(w, r)
	}
}

func TestMatchmakingHandler_AuthReject_NoSession(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	sessionRepo := repomocks.NewMockSessionRepository(ctrl)
	svc := svcmocks.NewMockMatchmakingService(ctrl)

	handler := NewMatchmakingHandler(zap.NewNop(), sessionRepo, svc, "session")

	srv := httptest.NewServer(makeHTTPHandler(handler))
	t.Cleanup(srv.Close)

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/"

	// No cookie — connection should be rejected (close frame).
	c, _, err := websocket.Dial(context.Background(), wsURL, nil)
	if err != nil {
		// Dial itself may fail if the server closes before the handshake — that's fine.
		return
	}
	defer c.CloseNow() //nolint:errcheck // best-effort

	// Expect the server to close the connection.
	_, _, readErr := c.Read(context.Background())
	if readErr == nil {
		t.Fatal("expected connection to be closed by server, got nil error")
	}
}

func TestMatchmakingHandler_AuthReject_InvalidSession(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	sessionRepo := repomocks.NewMockSessionRepository(ctrl)
	svc := svcmocks.NewMockMatchmakingService(ctrl)

	sessionRepo.EXPECT().
		UserIDBySession(gomock.Any(), "bad-token").
		Return(int64(0), errors.New("not found"))

	handler := NewMatchmakingHandler(zap.NewNop(), sessionRepo, svc, "session")

	srv := httptest.NewServer(makeHTTPHandler(handler))
	t.Cleanup(srv.Close)

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/"

	c, _, err := websocket.Dial(context.Background(), wsURL, &websocket.DialOptions{
		HTTPHeader: map[string][]string{"Cookie": {"session=bad-token"}},
	})
	if err != nil {
		return // rejected at HTTP level
	}
	defer c.CloseNow() //nolint:errcheck // best-effort

	_, _, readErr := c.Read(context.Background())
	if readErr == nil {
		t.Fatal("expected connection to be closed by server, got nil error")
	}
}

func TestMatchmakingHandler_Join_ValidationError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	sessionRepo := repomocks.NewMockSessionRepository(ctrl)
	svc := svcmocks.NewMockMatchmakingService(ctrl)

	sessionRepo.EXPECT().
		UserIDBySession(gomock.Any(), "valid-token").
		Return(int64(42), nil)

	// Service returns a validation error → error sent to client.
	svc.EXPECT().
		Join(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(domain.ErrInvalidLevel)

	handler := NewMatchmakingHandler(zap.NewNop(), sessionRepo, svc, "session")

	srv := httptest.NewServer(makeHTTPHandler(handler))
	t.Cleanup(srv.Close)

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/"

	ctx := context.Background()
	c, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{
		HTTPHeader: map[string][]string{"Cookie": {"session=valid-token"}},
	})
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer c.CloseNow() //nolint:errcheck // best-effort

	if writeErr := wsjson.Write(ctx, c, domain.InboundMessage{Type: "join", Mode: "ranked", Level: 0}); writeErr != nil {
		t.Fatalf("Write: %v", writeErr)
	}

	// Expect an error response.
	var resp map[string]string
	if readErr := wsjson.Read(ctx, c, &resp); readErr != nil {
		t.Fatalf("Read error response: %v", readErr)
	}
	if resp["type"] != "error" {
		t.Errorf("response type = %q, want error", resp["type"])
	}
}

func TestMatchmakingHandler_Leave_Message(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	sessionRepo := repomocks.NewMockSessionRepository(ctrl)
	svc := svcmocks.NewMockMatchmakingService(ctrl)

	sessionRepo.EXPECT().
		UserIDBySession(gomock.Any(), "valid-token").
		Return(int64(42), nil)

	svc.EXPECT().
		Join(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil)

	// Leave called once for the "leave" message; after leave, currentMode=""
	// so disconnect should NOT call Leave again.
	svc.EXPECT().
		Leave(gomock.Any(), int64(42), "ranked").
		Times(1)

	handler := NewMatchmakingHandler(zap.NewNop(), sessionRepo, svc, "session")

	srv := httptest.NewServer(makeHTTPHandler(handler))
	t.Cleanup(srv.Close)

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/"

	ctx := context.Background()
	c, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{
		HTTPHeader: map[string][]string{"Cookie": {"session=valid-token"}},
	})
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer c.CloseNow() //nolint:errcheck // best-effort

	if writeErr := wsjson.Write(ctx, c, domain.InboundMessage{Type: "join", Mode: "ranked", Level: 5}); writeErr != nil {
		t.Fatalf("Write join: %v", writeErr)
	}

	if writeErr := wsjson.Write(ctx, c, domain.InboundMessage{Type: "leave"}); writeErr != nil {
		t.Fatalf("Write leave: %v", writeErr)
	}

	// Close cleanly — no second Leave should be triggered.
	_ = c.Close(websocket.StatusNormalClosure, "")
}

func TestMatchmakingHandler_Join_InternalErrorRedacted(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	sessionRepo := repomocks.NewMockSessionRepository(ctrl)
	svc := svcmocks.NewMockMatchmakingService(ctrl)

	sessionRepo.EXPECT().
		UserIDBySession(gomock.Any(), "valid-token").
		Return(int64(42), nil)

	// Service returns a non-sentinel error (e.g. Redis down).
	svc.EXPECT().
		Join(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(errors.New("redis: connection refused"))

	handler := NewMatchmakingHandler(zap.NewNop(), sessionRepo, svc, "session")

	srv := httptest.NewServer(makeHTTPHandler(handler))
	t.Cleanup(srv.Close)

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/"

	ctx := context.Background()
	c, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{
		HTTPHeader: map[string][]string{"Cookie": {"session=valid-token"}},
	})
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer c.CloseNow() //nolint:errcheck // best-effort

	if writeErr := wsjson.Write(ctx, c, domain.InboundMessage{Type: "join", Mode: "ranked", Level: 5}); writeErr != nil {
		t.Fatalf("Write: %v", writeErr)
	}

	var resp map[string]string
	if readErr := wsjson.Read(ctx, c, &resp); readErr != nil {
		t.Fatalf("Read error response: %v", readErr)
	}
	if resp["type"] != "error" {
		t.Errorf("response type = %q, want error", resp["type"])
	}
	// Internal redis error must not be forwarded to the client.
	if resp["error"] == "redis: connection refused" {
		t.Errorf("internal error leaked to client: %q", resp["error"])
	}
	if resp["error"] != "internal error" {
		t.Errorf("error = %q, want %q", resp["error"], "internal error")
	}
}

func TestSafeErrMsg(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want string
	}{
		{"ErrInvalidLevel forwarded", domain.ErrInvalidLevel, domain.ErrInvalidLevel.Error()},
		{"ErrInvalidMode forwarded", domain.ErrInvalidMode, domain.ErrInvalidMode.Error()},
		{"generic error redacted", errors.New("redis: dial tcp refused"), "internal error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := safeErrMsg(tt.err)
			if got != tt.want {
				t.Errorf("safeErrMsg(%v) = %q, want %q", tt.err, got, tt.want)
			}
		})
	}
}

func TestMatchmakingHandler_UnknownType(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	sessionRepo := repomocks.NewMockSessionRepository(ctrl)
	svc := svcmocks.NewMockMatchmakingService(ctrl)

	sessionRepo.EXPECT().
		UserIDBySession(gomock.Any(), "valid-token").
		Return(int64(42), nil)

	handler := NewMatchmakingHandler(zap.NewNop(), sessionRepo, svc, "session")

	srv := httptest.NewServer(makeHTTPHandler(handler))
	t.Cleanup(srv.Close)

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/"

	ctx := context.Background()
	c, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{
		HTTPHeader: map[string][]string{"Cookie": {"session=valid-token"}},
	})
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer c.CloseNow() //nolint:errcheck // best-effort

	if writeErr := wsjson.Write(ctx, c, domain.InboundMessage{Type: "unknown"}); writeErr != nil {
		t.Fatalf("Write: %v", writeErr)
	}

	var resp map[string]string
	if readErr := wsjson.Read(ctx, c, &resp); readErr != nil {
		t.Fatalf("Read error response: %v", readErr)
	}
	if resp["type"] != "error" {
		t.Errorf("response type = %q, want error", resp["type"])
	}
}

func TestMatchmakingHandler_Disconnect_Cleanup(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	sessionRepo := repomocks.NewMockSessionRepository(ctrl)
	svc := svcmocks.NewMockMatchmakingService(ctrl)

	sessionRepo.EXPECT().
		UserIDBySession(gomock.Any(), "valid-token").
		Return(int64(42), nil)

	joinDone := make(chan struct{})
	svc.EXPECT().
		Join(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ service.Conn, _ domain.Player) error {
			close(joinDone)
			return nil
		})

	leaveDone := make(chan struct{})
	// Disconnect without leave → Leave should be called once during cleanup.
	svc.EXPECT().
		Leave(gomock.Any(), int64(42), "ranked").
		DoAndReturn(func(_ context.Context, _ int64, _ string) {
			close(leaveDone)
		})

	handler := NewMatchmakingHandler(zap.NewNop(), sessionRepo, svc, "session")

	srv := httptest.NewServer(makeHTTPHandler(handler))
	t.Cleanup(srv.Close)

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/"

	ctx := context.Background()
	c, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{
		HTTPHeader: map[string][]string{"Cookie": {"session=valid-token"}},
	})
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}

	if writeErr := wsjson.Write(ctx, c, domain.InboundMessage{Type: "join", Mode: "ranked", Level: 5}); writeErr != nil {
		t.Fatalf("Write join: %v", writeErr)
	}

	// Wait for the server to process the join before closing.
	select {
	case <-joinDone:
	case <-time.After(2 * time.Second):
		t.Fatal("join not processed within 2s")
	}

	// Abruptly close — server should trigger Leave on disconnect.
	c.CloseNow() //nolint:errcheck // intentional abrupt close

	// Wait for the server to call Leave before the test ends (and gomock checks).
	select {
	case <-leaveDone:
	case <-time.After(2 * time.Second):
		t.Fatal("Leave not called within 2s after disconnect")
	}
}
