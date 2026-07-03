package delivery

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"go.uber.org/mock/gomock"
	"go.uber.org/zap"

	"github.com/pizdagladki/full/services/signaling/internal/api/domain"
	"github.com/pizdagladki/full/services/signaling/internal/api/repository"
	repomocks "github.com/pizdagladki/full/services/signaling/internal/api/repository/mocks"
	"github.com/pizdagladki/full/services/signaling/internal/api/service"
	svcmocks "github.com/pizdagladki/full/services/signaling/internal/api/service/mocks"
)

// makeHTTPHandler wraps a SignalingHandler.ServeWS as an http.HandlerFunc.
func makeHTTPHandler(h SignalingHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		h.ServeWS(w, r)
	}
}

// wsURL builds a ws:// URL from an httptest.Server URL.
func wsURL(srv *httptest.Server) string {
	return "ws" + strings.TrimPrefix(srv.URL, "http") + "/"
}

// dialWithCookie opens a WS connection with the given session cookie value.
func dialWithCookie(t *testing.T, srv *httptest.Server, token string) *websocket.Conn {
	t.Helper()

	opts := &websocket.DialOptions{
		HTTPHeader: map[string][]string{"Cookie": {"session=" + token}},
	}

	c, _, err := websocket.Dial(context.Background(), wsURL(srv), opts)
	if err != nil {
		t.Fatalf("websocket.Dial: %v", err)
	}

	return c
}

// readJSON reads one WS frame and unmarshals it into a string map.
func readJSON(t *testing.T, c *websocket.Conn) map[string]string {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	_, raw, err := c.Read(ctx)
	if err != nil {
		t.Fatalf("readJSON: %v", err)
	}

	var msg map[string]string
	if err := json.Unmarshal(raw, &msg); err != nil {
		t.Fatalf("readJSON unmarshal %q: %v", raw, err)
	}

	return msg
}

// readMsgType reads one WS frame and returns the value of the "type" field.
// Unlike readJSON it tolerates frames with numeric fields (e.g. outcome).
func readMsgType(t *testing.T, c *websocket.Conn) string {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	_, raw, err := c.Read(ctx)
	if err != nil {
		t.Fatalf("readMsgType: %v", err)
	}

	var envelope struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("readMsgType unmarshal %q: %v", raw, err)
	}

	return envelope.Type
}

// readRaw reads one WS frame and returns the raw bytes.
func readRaw(t *testing.T, c *websocket.Conn) []byte {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	_, raw, err := c.Read(ctx)
	if err != nil {
		t.Fatalf("readRaw: %v", err)
	}

	return raw
}

// expectClosed asserts the next read returns an error (connection closed).
func expectClosed(t *testing.T, c *websocket.Conn) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	_, _, err := c.Read(ctx)
	if err == nil {
		t.Fatal("expected connection to be closed, got nil error")
	}
}

// sendFrame writes a raw string as a WS text frame.
func sendFrame(t *testing.T, c *websocket.Conn, msg string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := c.Write(ctx, websocket.MessageText, []byte(msg)); err != nil {
		t.Fatalf("sendFrame: %v", err)
	}
}

// ─── mock-service unit tests ──────────────────────────────────────────────────

func TestSignalingHandler_AuthReject_NoSession(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	sessionRepo := repomocks.NewMockSessionRepository(ctrl)
	svc := svcmocks.NewMockSignalingService(ctrl)

	handler := NewSignalingHandler(zap.NewNop(), sessionRepo, svc, "session", 0, 0)
	srv := httptest.NewServer(makeHTTPHandler(handler))
	t.Cleanup(srv.Close)

	// No cookie — server closes immediately.
	c, _, err := websocket.Dial(context.Background(), wsURL(srv), nil)
	if err != nil {
		return // rejected at HTTP upgrade
	}

	defer c.CloseNow() //nolint:errcheck

	expectClosed(t, c)
}

func TestSignalingHandler_AuthReject_InvalidSession(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	sessionRepo := repomocks.NewMockSessionRepository(ctrl)
	svc := svcmocks.NewMockSignalingService(ctrl)

	sessionRepo.EXPECT().
		UserIDBySession(gomock.Any(), "bad-token").
		Return(int64(0), errors.New("not found"))

	handler := NewSignalingHandler(zap.NewNop(), sessionRepo, svc, "session", 0, 0)
	srv := httptest.NewServer(makeHTTPHandler(handler))
	t.Cleanup(srv.Close)

	c, _, err := websocket.Dial(context.Background(), wsURL(srv), &websocket.DialOptions{
		HTTPHeader: map[string][]string{"Cookie": {"session=bad-token"}},
	})
	if err != nil {
		return
	}

	defer c.CloseNow() //nolint:errcheck

	expectClosed(t, c)
}

func TestSignalingHandler_UnknownType(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	sessionRepo := repomocks.NewMockSessionRepository(ctrl)
	svc := svcmocks.NewMockSignalingService(ctrl)

	sessionRepo.EXPECT().UserIDBySession(gomock.Any(), "tok").Return(int64(1), nil)

	handler := NewSignalingHandler(zap.NewNop(), sessionRepo, svc, "session", 0, 0)
	srv := httptest.NewServer(makeHTTPHandler(handler))
	t.Cleanup(srv.Close)

	c := dialWithCookie(t, srv, "tok")
	defer c.CloseNow() //nolint:errcheck

	sendFrame(t, c, `{"type":"bogus","room_id":"r1"}`)

	msg := readJSON(t, c)
	if msg["type"] != "error" {
		t.Errorf("type = %q, want error", msg["type"])
	}
}

func TestSignalingHandler_InvalidJSON(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	sessionRepo := repomocks.NewMockSessionRepository(ctrl)
	svc := svcmocks.NewMockSignalingService(ctrl)

	sessionRepo.EXPECT().UserIDBySession(gomock.Any(), "tok").Return(int64(1), nil)

	handler := NewSignalingHandler(zap.NewNop(), sessionRepo, svc, "session", 0, 0)
	srv := httptest.NewServer(makeHTTPHandler(handler))
	t.Cleanup(srv.Close)

	c := dialWithCookie(t, srv, "tok")
	defer c.CloseNow() //nolint:errcheck

	sendFrame(t, c, `{not valid json}`)

	msg := readJSON(t, c)
	if msg["type"] != "error" {
		t.Errorf("type = %q, want error for invalid JSON", msg["type"])
	}
}

func TestSignalingHandler_Join_RoomFull_ErrorAndClose(t *testing.T) {
	t.Parallel()

	// criterion: 1 — third joiner gets error frame then connection is closed
	ctrl := gomock.NewController(t)
	sessionRepo := repomocks.NewMockSessionRepository(ctrl)
	svc := svcmocks.NewMockSignalingService(ctrl)

	sessionRepo.EXPECT().UserIDBySession(gomock.Any(), "tok").Return(int64(3), nil)
	svc.EXPECT().Join(gomock.Any(), gomock.Any(), "room-full", gomock.Any()).Return(domain.ErrRoomFull)

	handler := NewSignalingHandler(zap.NewNop(), sessionRepo, svc, "session", 0, 0)
	srv := httptest.NewServer(makeHTTPHandler(handler))
	t.Cleanup(srv.Close)

	c := dialWithCookie(t, srv, "tok")
	defer c.CloseNow() //nolint:errcheck

	sendFrame(t, c, `{"type":"join","room_id":"room-full"}`)

	msg := readJSON(t, c)
	if msg["type"] != "error" {
		t.Errorf("type = %q, want error", msg["type"]) // criterion: 1 — fails if error frame not sent
	}

	expectClosed(t, c) // criterion: 1 — fails if connection not closed for full room
}

func TestSignalingHandler_CreateRoom_Success(t *testing.T) {
	t.Parallel()

	// criterion: 7 — create_room returns a room_created frame carrying room_id and code
	ctrl := gomock.NewController(t)
	sessionRepo := repomocks.NewMockSessionRepository(ctrl)
	svc := svcmocks.NewMockSignalingService(ctrl)

	sessionRepo.EXPECT().UserIDBySession(gomock.Any(), "tok").Return(int64(1), nil)
	svc.EXPECT().CreateRoom(gomock.Any(), gomock.Any()).Return("room-new", "CODE123", nil)
	// Deferred CloseNow triggers leaveOnDisconnect asynchronously; absorb it.
	svc.EXPECT().Leave(gomock.Any(), gomock.Any(), "room-new").AnyTimes()

	handler := NewSignalingHandler(zap.NewNop(), sessionRepo, svc, "session", 0, 0)
	srv := httptest.NewServer(makeHTTPHandler(handler))
	t.Cleanup(srv.Close)

	c := dialWithCookie(t, srv, "tok")
	defer c.CloseNow() //nolint:errcheck

	sendFrame(t, c, `{"type":"create_room"}`)

	raw := readRaw(t, c)

	var msg struct {
		Type   string `json:"type"`
		RoomID string `json:"room_id"`
		Code   string `json:"code"`
	}
	if err := json.Unmarshal(raw, &msg); err != nil {
		t.Fatalf("unmarshal room_created frame: %v", err)
	}

	if msg.Type != "room_created" {
		t.Errorf("type = %q, want room_created", msg.Type) // criterion: 7
	}

	if msg.RoomID != "room-new" {
		t.Errorf("room_id = %q, want room-new", msg.RoomID) // criterion: 7 — fails if room_id missing
	}

	if msg.Code != "CODE123" {
		t.Errorf("code = %q, want CODE123", msg.Code) // criterion: 7 — fails if code missing
	}
}

func TestSignalingHandler_CreateRoom_AlreadyInRoom(t *testing.T) {
	t.Parallel()

	// criterion: 7 — create_room while already in a room is rejected, one-room-per-connection
	ctrl := gomock.NewController(t)
	sessionRepo := repomocks.NewMockSessionRepository(ctrl)
	svc := svcmocks.NewMockSignalingService(ctrl)

	sessionRepo.EXPECT().UserIDBySession(gomock.Any(), "tok").Return(int64(1), nil)
	svc.EXPECT().Join(gomock.Any(), gomock.Any(), "room-existing", gomock.Any()).Return(nil)
	// Deferred CloseNow triggers leaveOnDisconnect asynchronously; absorb it.
	svc.EXPECT().Leave(gomock.Any(), gomock.Any(), "room-existing").AnyTimes()

	handler := NewSignalingHandler(zap.NewNop(), sessionRepo, svc, "session", 0, 0)
	srv := httptest.NewServer(makeHTTPHandler(handler))
	t.Cleanup(srv.Close)

	c := dialWithCookie(t, srv, "tok")
	defer c.CloseNow() //nolint:errcheck

	sendFrame(t, c, `{"type":"join","room_id":"room-existing"}`)

	sendFrame(t, c, `{"type":"create_room"}`)

	msg := readJSON(t, c)
	if msg["type"] != "error" {
		t.Errorf("type = %q, want error", msg["type"]) // criterion: 7 — fails if create_room allowed on a second room
	}

	if msg["error"] != domain.ErrAlreadyInRoom.Error() {
		t.Errorf("error = %q, want %q", msg["error"], domain.ErrAlreadyInRoom.Error())
	}
}

func TestSignalingHandler_CreateRoom_ServiceError(t *testing.T) {
	t.Parallel()

	// criterion: 7 — a CreateRoom failure yields a redacted error frame, connection stays open
	ctrl := gomock.NewController(t)
	sessionRepo := repomocks.NewMockSessionRepository(ctrl)
	svc := svcmocks.NewMockSignalingService(ctrl)

	sessionRepo.EXPECT().UserIDBySession(gomock.Any(), "tok").Return(int64(1), nil)
	svc.EXPECT().CreateRoom(gomock.Any(), gomock.Any()).Return("", "", errors.New("redis down"))

	handler := NewSignalingHandler(zap.NewNop(), sessionRepo, svc, "session", 0, 0)
	srv := httptest.NewServer(makeHTTPHandler(handler))
	t.Cleanup(srv.Close)

	c := dialWithCookie(t, srv, "tok")
	defer c.CloseNow() //nolint:errcheck

	sendFrame(t, c, `{"type":"create_room"}`)

	msg := readJSON(t, c)
	if msg["type"] != "error" {
		t.Errorf("type = %q, want error", msg["type"]) // criterion: 7
	}
}

func TestSignalingHandler_JoinRoom_Success(t *testing.T) {
	t.Parallel()

	// criterion: 8 — join_room returns a room_joined frame carrying room_id
	ctrl := gomock.NewController(t)
	sessionRepo := repomocks.NewMockSessionRepository(ctrl)
	svc := svcmocks.NewMockSignalingService(ctrl)

	sessionRepo.EXPECT().UserIDBySession(gomock.Any(), "tok").Return(int64(2), nil)
	svc.EXPECT().JoinByCode(gomock.Any(), gomock.Any(), "CODE123").Return("room-priv", nil)
	// Deferred CloseNow triggers leaveOnDisconnect asynchronously; absorb it.
	svc.EXPECT().Leave(gomock.Any(), gomock.Any(), "room-priv").AnyTimes()

	handler := NewSignalingHandler(zap.NewNop(), sessionRepo, svc, "session", 0, 0)
	srv := httptest.NewServer(makeHTTPHandler(handler))
	t.Cleanup(srv.Close)

	c := dialWithCookie(t, srv, "tok")
	defer c.CloseNow() //nolint:errcheck

	sendFrame(t, c, `{"type":"join_room","code":"CODE123"}`)

	msg := readJSON(t, c)
	if msg["type"] != "room_joined" {
		t.Errorf("type = %q, want room_joined", msg["type"]) // criterion: 8
	}

	if msg["room_id"] != "room-priv" {
		t.Errorf("room_id = %q, want room-priv", msg["room_id"]) // criterion: 8 — fails if room_id missing
	}
}

func TestSignalingHandler_JoinRoom_AlreadyInRoom(t *testing.T) {
	t.Parallel()

	// criterion: 8 — join_room while already in a room is rejected
	ctrl := gomock.NewController(t)
	sessionRepo := repomocks.NewMockSessionRepository(ctrl)
	svc := svcmocks.NewMockSignalingService(ctrl)

	sessionRepo.EXPECT().UserIDBySession(gomock.Any(), "tok").Return(int64(1), nil)
	svc.EXPECT().Join(gomock.Any(), gomock.Any(), "room-existing", gomock.Any()).Return(nil)
	// Deferred CloseNow triggers leaveOnDisconnect asynchronously; absorb it.
	svc.EXPECT().Leave(gomock.Any(), gomock.Any(), "room-existing").AnyTimes()

	handler := NewSignalingHandler(zap.NewNop(), sessionRepo, svc, "session", 0, 0)
	srv := httptest.NewServer(makeHTTPHandler(handler))
	t.Cleanup(srv.Close)

	c := dialWithCookie(t, srv, "tok")
	defer c.CloseNow() //nolint:errcheck

	sendFrame(t, c, `{"type":"join","room_id":"room-existing"}`)

	sendFrame(t, c, `{"type":"join_room","code":"CODE123"}`)

	msg := readJSON(t, c)
	if msg["type"] != "error" {
		t.Errorf("type = %q, want error", msg["type"]) // criterion: 8 — fails if join_room allowed on a second room
	}

	if msg["error"] != domain.ErrAlreadyInRoom.Error() {
		t.Errorf("error = %q, want %q", msg["error"], domain.ErrAlreadyInRoom.Error())
	}
}

func TestSignalingHandler_JoinRoom_InvalidCode_KeepsConnOpen(t *testing.T) {
	t.Parallel()

	// criterion: 9 — an invalid/expired code gets an error frame; the connection stays open
	ctrl := gomock.NewController(t)
	sessionRepo := repomocks.NewMockSessionRepository(ctrl)
	svc := svcmocks.NewMockSignalingService(ctrl)

	sessionRepo.EXPECT().UserIDBySession(gomock.Any(), "tok").Return(int64(2), nil)
	svc.EXPECT().JoinByCode(gomock.Any(), gomock.Any(), "BADCODE").Return("", domain.ErrInvalidCode)

	handler := NewSignalingHandler(zap.NewNop(), sessionRepo, svc, "session", 0, 0)
	srv := httptest.NewServer(makeHTTPHandler(handler))
	t.Cleanup(srv.Close)

	c := dialWithCookie(t, srv, "tok")
	defer c.CloseNow() //nolint:errcheck

	sendFrame(t, c, `{"type":"join_room","code":"BADCODE"}`)

	msg := readJSON(t, c)
	if msg["type"] != "error" {
		t.Errorf("type = %q, want error", msg["type"]) // criterion: 9
	}

	if msg["error"] != domain.ErrInvalidCode.Error() {
		t.Errorf("error = %q, want %q", msg["error"], domain.ErrInvalidCode.Error()) // criterion: 9 — fails if wrong error text
	}

	// The connection must remain open — the client may retry with a corrected code.
	sendFrame(t, c, `{"type":"bogus"}`)

	msg2 := readJSON(t, c)
	if msg2["type"] != "error" {
		t.Errorf("second frame type = %q, want error (connection must stay open)", msg2["type"]) // criterion: 9
	}
}

func TestSignalingHandler_JoinRoom_RoomFull_ErrorAndClose(t *testing.T) {
	t.Parallel()

	// criterion: 10 — join_room to a full room gets an error frame then the connection closes
	ctrl := gomock.NewController(t)
	sessionRepo := repomocks.NewMockSessionRepository(ctrl)
	svc := svcmocks.NewMockSignalingService(ctrl)

	sessionRepo.EXPECT().UserIDBySession(gomock.Any(), "tok").Return(int64(3), nil)
	svc.EXPECT().JoinByCode(gomock.Any(), gomock.Any(), "FULLCODE").Return("", domain.ErrRoomFull)

	handler := NewSignalingHandler(zap.NewNop(), sessionRepo, svc, "session", 0, 0)
	srv := httptest.NewServer(makeHTTPHandler(handler))
	t.Cleanup(srv.Close)

	c := dialWithCookie(t, srv, "tok")
	defer c.CloseNow() //nolint:errcheck

	sendFrame(t, c, `{"type":"join_room","code":"FULLCODE"}`)

	msg := readJSON(t, c)
	if msg["type"] != "error" {
		t.Errorf("type = %q, want error", msg["type"]) // criterion: 10 — fails if error frame not sent
	}

	expectClosed(t, c) // criterion: 10 — fails if connection not closed for a full room
}

func TestSignalingHandler_Relay_NonMember_ErrorFrame(t *testing.T) {
	t.Parallel()

	// criterion: 4 — relay from non-member gets error frame, connection stays open
	ctrl := gomock.NewController(t)
	sessionRepo := repomocks.NewMockSessionRepository(ctrl)
	svc := svcmocks.NewMockSignalingService(ctrl)

	sessionRepo.EXPECT().UserIDBySession(gomock.Any(), "tok").Return(int64(99), nil)
	svc.EXPECT().
		Relay(gomock.Any(), gomock.Any(), "room-x", gomock.Any()).
		Return(domain.ErrNotMember)

	handler := NewSignalingHandler(zap.NewNop(), sessionRepo, svc, "session", 0, 0)
	srv := httptest.NewServer(makeHTTPHandler(handler))
	t.Cleanup(srv.Close)

	c := dialWithCookie(t, srv, "tok")
	defer c.CloseNow() //nolint:errcheck

	sendFrame(t, c, `{"type":"sdp","room_id":"room-x","sdp":"v=0..."}`)

	msg := readJSON(t, c)
	if msg["type"] != "error" {
		t.Errorf("type = %q, want error", msg["type"]) // criterion: 4
	}
}

func TestSignalingHandler_Disconnect_TriggersLeave(t *testing.T) {
	t.Parallel()

	// criterion: 5 — disconnect triggers Leave (which sends peer_left and cleans up Redis)
	ctrl := gomock.NewController(t)
	sessionRepo := repomocks.NewMockSessionRepository(ctrl)
	svc := svcmocks.NewMockSignalingService(ctrl)

	sessionRepo.EXPECT().UserIDBySession(gomock.Any(), "tok").Return(int64(1), nil)

	joinDone := make(chan struct{})
	svc.EXPECT().
		Join(gomock.Any(), gomock.Any(), "room-dc", gomock.Any()).
		DoAndReturn(func(_ context.Context, _ service.Conn, _ string, _ string) error {
			close(joinDone)

			return nil
		})

	leaveDone := make(chan struct{})
	svc.EXPECT().
		Leave(gomock.Any(), gomock.Any(), "room-dc").
		DoAndReturn(func(_ context.Context, _ service.Conn, _ string) {
			close(leaveDone)
		})

	handler := NewSignalingHandler(zap.NewNop(), sessionRepo, svc, "session", 0, 0)
	srv := httptest.NewServer(makeHTTPHandler(handler))
	t.Cleanup(srv.Close)

	c := dialWithCookie(t, srv, "tok")

	sendFrame(t, c, `{"type":"join","room_id":"room-dc"}`)

	select {
	case <-joinDone:
	case <-time.After(2 * time.Second):
		t.Fatal("join not processed within 2s")
	}

	c.CloseNow() //nolint:errcheck

	select {
	case <-leaveDone:
	case <-time.After(2 * time.Second):
		t.Fatal("Leave not called within 2s after disconnect") // criterion: 5
	}
}

// ─── integration tests: real SignalingService + fake session repo ─────────────

// fakeSessionRepo is an in-memory SessionRepository for integration tests.
type fakeSessionRepo struct {
	mu       sync.Mutex
	sessions map[string]int64
}

func newFakeSessionRepo() *fakeSessionRepo {
	return &fakeSessionRepo{sessions: make(map[string]int64)}
}

func (r *fakeSessionRepo) add(token string, userID int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sessions[token] = userID
}

func (r *fakeSessionRepo) UserIDBySession(_ context.Context, sessionID string) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	id, ok := r.sessions[sessionID]
	if !ok {
		return 0, repository.ErrSessionNotFound
	}

	return id, nil
}

// fakeRoomRepo is an in-memory RoomRepository for integration tests.
type fakeRoomRepo struct {
	mu    sync.Mutex
	rooms map[string]map[int64]struct{}
}

func newFakeRoomRepo() *fakeRoomRepo {
	return &fakeRoomRepo{rooms: make(map[string]map[int64]struct{})}
}

func (r *fakeRoomRepo) Join(_ context.Context, roomID string, userID int64) (repository.JoinResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.rooms[roomID] == nil {
		r.rooms[roomID] = make(map[int64]struct{})
	}

	members := r.rooms[roomID]

	if _, ok := members[userID]; ok {
		return repository.JoinResultAlreadyMember, nil
	}

	if len(members) >= 2 {
		return repository.JoinResultFull, nil
	}

	members[userID] = struct{}{}

	return repository.JoinResultJoined, nil
}

func (r *fakeRoomRepo) IsMember(_ context.Context, roomID string, userID int64) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	_, ok := r.rooms[roomID][userID]

	return ok, nil
}

func (r *fakeRoomRepo) RemoveRoom(_ context.Context, roomID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.rooms, roomID)

	return nil
}

// nopRatingsClient is a no-op RatingsClient for integration tests.
type nopRatingsClient struct{}

func (n *nopRatingsClient) ApplyResult(_ context.Context, _ service.ApplyResultRequest) error {
	return nil
}

// fakeRoomCodeRepo is an in-memory RoomCodeRepository for integration tests.
type fakeRoomCodeRepo struct {
	mu      sync.Mutex
	nextID  int
	byCode  map[string]string
	removed []string
}

func newFakeRoomCodeRepo() *fakeRoomCodeRepo {
	return &fakeRoomCodeRepo{byCode: make(map[string]string)}
}

func (r *fakeRoomCodeRepo) CreateCode(_ context.Context, roomID string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.nextID++
	code := fmt.Sprintf("CODE%d", r.nextID)
	r.byCode[code] = roomID

	return code, nil
}

func (r *fakeRoomCodeRepo) ResolveCode(_ context.Context, code string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	roomID, ok := r.byCode[code]
	if !ok {
		return "", repository.ErrCodeNotFound
	}

	return roomID, nil
}

func (r *fakeRoomCodeRepo) RemoveCode(_ context.Context, code string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.byCode, code)
	r.removed = append(r.removed, code)

	return nil
}

func (r *fakeRoomCodeRepo) hasCode(code string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	_, ok := r.byCode[code]

	return ok
}

// testRoomIDGen returns a deterministic room-id generator for integration tests.
func testRoomIDGen() func() (string, error) {
	var n int64

	return func() (string, error) {
		n++

		return fmt.Sprintf("private-room-%d", n), nil
	}
}

// newIntegrationServer wires a real SignalingService + fake repos into an httptest.Server.
// Keepalive is disabled (interval=0) to keep integration tests deterministic.
func newIntegrationServer(t *testing.T, sessionRepo repository.SessionRepository, roomRepo repository.RoomRepository) *httptest.Server {
	t.Helper()

	return newIntegrationServerWithCodes(t, sessionRepo, roomRepo, newFakeRoomCodeRepo())
}

// newIntegrationServerWithCodes wires a real SignalingService + fake repos,
// including an explicit RoomCodeRepository, into an httptest.Server.
func newIntegrationServerWithCodes(
	t *testing.T,
	sessionRepo repository.SessionRepository,
	roomRepo repository.RoomRepository,
	roomCodeRepo repository.RoomCodeRepository,
) *httptest.Server {
	t.Helper()

	svc := service.NewSignalingService(
		zap.NewNop(), roomRepo, time.Now, time.AfterFunc, 150*time.Millisecond, &nopRatingsClient{},
		roomCodeRepo, testRoomIDGen(),
	)
	handler := NewSignalingHandler(zap.NewNop(), sessionRepo, svc, "session", 0, 0)
	srv := httptest.NewServer(makeHTTPHandler(handler))
	t.Cleanup(srv.Close)

	return srv
}

func TestIntegration_SDPRelayOfferAnswer(t *testing.T) {
	t.Parallel()

	// criterion: 2 — SDP offer from A forwarded verbatim to B; answer from B to A
	sessionRepo := newFakeSessionRepo()
	sessionRepo.add("tokA", 1)
	sessionRepo.add("tokB", 2)

	roomRepo := newFakeRoomRepo()
	srv := newIntegrationServer(t, sessionRepo, roomRepo)

	cA := dialWithCookie(t, srv, "tokA")
	defer cA.CloseNow() //nolint:errcheck

	cB := dialWithCookie(t, srv, "tokB")
	defer cB.CloseNow() //nolint:errcheck

	// Both join the same room.
	sendFrame(t, cA, `{"type":"join","room_id":"room-1"}`)
	sendFrame(t, cB, `{"type":"join","room_id":"room-1"}`)

	// Small pause so both joins are processed before relaying.
	time.Sleep(50 * time.Millisecond)

	// A sends SDP offer — B must receive verbatim; A must NOT receive it (no echo).
	offer := `{"type":"sdp","room_id":"room-1","sdp":"v=0 offer..."}`
	sendFrame(t, cA, offer)

	gotOffer := readRaw(t, cB)
	if string(gotOffer) != offer {
		t.Errorf("B received %q, want %q (verbatim)", gotOffer, offer) // criterion: 2 — fails if not verbatim
	}

	// B sends SDP answer — A must receive verbatim.
	answer := `{"type":"sdp","room_id":"room-1","sdp":"v=0 answer..."}`
	sendFrame(t, cB, answer)

	gotAnswer := readRaw(t, cA)
	if string(gotAnswer) != answer {
		t.Errorf("A received %q, want %q (verbatim)", gotAnswer, answer) // criterion: 2
	}
}

func TestIntegration_ICETrickle(t *testing.T) {
	t.Parallel()

	// criterion: 3 — ICE trickle (multiple candidates) both directions
	sessionRepo := newFakeSessionRepo()
	sessionRepo.add("tokA", 1)
	sessionRepo.add("tokB", 2)

	roomRepo := newFakeRoomRepo()
	srv := newIntegrationServer(t, sessionRepo, roomRepo)

	cA := dialWithCookie(t, srv, "tokA")
	defer cA.CloseNow() //nolint:errcheck

	cB := dialWithCookie(t, srv, "tokB")
	defer cB.CloseNow() //nolint:errcheck

	sendFrame(t, cA, `{"type":"join","room_id":"room-ice"}`)
	sendFrame(t, cB, `{"type":"join","room_id":"room-ice"}`)

	time.Sleep(50 * time.Millisecond)

	// A → B trickle ICE
	iceFromA := []string{
		`{"type":"ice","room_id":"room-ice","candidate":"a1"}`,
		`{"type":"ice","room_id":"room-ice","candidate":"a2"}`,
		`{"type":"ice","room_id":"room-ice","candidate":"a3"}`,
	}

	for _, ice := range iceFromA {
		sendFrame(t, cA, ice)
	}

	for i, want := range iceFromA {
		got := readRaw(t, cB)
		if string(got) != want {
			t.Errorf("B ICE[%d] = %q, want %q", i, got, want) // criterion: 3 — fails if ICE not forwarded
		}
	}

	// B → A trickle ICE
	iceFromB := []string{
		`{"type":"ice","room_id":"room-ice","candidate":"b1"}`,
		`{"type":"ice","room_id":"room-ice","candidate":"b2"}`,
	}

	for _, ice := range iceFromB {
		sendFrame(t, cB, ice)
	}

	for i, want := range iceFromB {
		got := readRaw(t, cA)
		if string(got) != want {
			t.Errorf("A ICE[%d] = %q, want %q", i, got, want) // criterion: 3
		}
	}
}

func TestIntegration_ThirdPeerRejected(t *testing.T) {
	t.Parallel()

	// criterion: 1 — third peer to a full room gets error+close
	sessionRepo := newFakeSessionRepo()
	sessionRepo.add("tokA", 1)
	sessionRepo.add("tokB", 2)
	sessionRepo.add("tokC", 3)

	roomRepo := newFakeRoomRepo()
	srv := newIntegrationServer(t, sessionRepo, roomRepo)

	cA := dialWithCookie(t, srv, "tokA")
	defer cA.CloseNow() //nolint:errcheck

	cB := dialWithCookie(t, srv, "tokB")
	defer cB.CloseNow() //nolint:errcheck

	sendFrame(t, cA, `{"type":"join","room_id":"room-3"}`)
	sendFrame(t, cB, `{"type":"join","room_id":"room-3"}`)

	time.Sleep(50 * time.Millisecond)

	cC := dialWithCookie(t, srv, "tokC")
	defer cC.CloseNow() //nolint:errcheck

	sendFrame(t, cC, `{"type":"join","room_id":"room-3"}`)

	msg := readJSON(t, cC)
	if msg["type"] != "error" {
		t.Errorf("third peer got %q, want error", msg["type"]) // criterion: 1 — fails if no error
	}

	expectClosed(t, cC) // criterion: 1 — fails if connection not closed
}

func TestIntegration_NonMemberRelayRejected(t *testing.T) {
	t.Parallel()

	// criterion: 4 — a peer sending sdp/ice without joining gets error, no relay
	sessionRepo := newFakeSessionRepo()
	sessionRepo.add("tokA", 1)
	sessionRepo.add("tokB", 2)
	sessionRepo.add("tokX", 99) // stranger — never joins

	roomRepo := newFakeRoomRepo()
	srv := newIntegrationServer(t, sessionRepo, roomRepo)

	cA := dialWithCookie(t, srv, "tokA")
	defer cA.CloseNow() //nolint:errcheck

	cB := dialWithCookie(t, srv, "tokB")
	defer cB.CloseNow() //nolint:errcheck

	sendFrame(t, cA, `{"type":"join","room_id":"room-nm"}`)
	sendFrame(t, cB, `{"type":"join","room_id":"room-nm"}`)

	time.Sleep(50 * time.Millisecond)

	cX := dialWithCookie(t, srv, "tokX")
	defer cX.CloseNow() //nolint:errcheck

	// Stranger tries to relay SDP to room-nm without joining.
	sendFrame(t, cX, `{"type":"sdp","room_id":"room-nm","sdp":"injected"}`)

	msg := readJSON(t, cX)
	if msg["type"] != "error" {
		t.Errorf("stranger got %q, want error", msg["type"]) // criterion: 4 — fails if non-member not rejected
	}
}

func TestIntegration_PeerLeft(t *testing.T) {
	t.Parallel()

	// criterion: 5 — when A disconnects, B receives peer_left
	sessionRepo := newFakeSessionRepo()
	sessionRepo.add("tokA", 1)
	sessionRepo.add("tokB", 2)

	roomRepo := newFakeRoomRepo()
	srv := newIntegrationServer(t, sessionRepo, roomRepo)

	cA := dialWithCookie(t, srv, "tokA")
	cB := dialWithCookie(t, srv, "tokB")
	defer cB.CloseNow() //nolint:errcheck

	sendFrame(t, cA, `{"type":"join","room_id":"room-pl"}`)
	sendFrame(t, cB, `{"type":"join","room_id":"room-pl"}`)

	time.Sleep(50 * time.Millisecond)

	// A disconnects abruptly.
	cA.CloseNow() //nolint:errcheck

	// B should receive outcome (forfeit) then peer_left.
	// criterion: 5 — fails if peer_left not sent after forfeit outcome
	outcomeType := readMsgType(t, cB)
	if outcomeType != "outcome" {
		t.Errorf("B first frame type = %q, want outcome", outcomeType)
	}

	peerLeftType := readMsgType(t, cB)
	if peerLeftType != "peer_left" {
		t.Errorf("B second frame type = %q, want peer_left", peerLeftType) // criterion: 5
	}
}

// ─── FIX 1 tests: one-room-per-connection ────────────────────────────────────

func TestIntegration_JoinDifferentRoom_Rejected(t *testing.T) {
	t.Parallel()

	// criterion: fix1 — joining a second different room is rejected with error frame;
	// the connection stays on the original room.
	// Fails if the handler allows multi-room registration.
	sessionRepo := newFakeSessionRepo()
	sessionRepo.add("tokA", 1)
	sessionRepo.add("tokB", 2)

	roomRepo := newFakeRoomRepo()
	srv := newIntegrationServer(t, sessionRepo, roomRepo)

	cA := dialWithCookie(t, srv, "tokA")
	defer cA.CloseNow() //nolint:errcheck

	cB := dialWithCookie(t, srv, "tokB")
	defer cB.CloseNow() //nolint:errcheck

	// A joins room-first.
	sendFrame(t, cA, `{"type":"join","room_id":"room-first"}`)
	time.Sleep(50 * time.Millisecond)

	// A tries to join room-second (different) — must be rejected.
	sendFrame(t, cA, `{"type":"join","room_id":"room-second"}`)

	msg := readJSON(t, cA)
	if msg["type"] != "error" {
		t.Errorf("got %q, want error for multi-room join attempt", msg["type"]) // fix1 — fails if no error
	}

	if msg["error"] != domain.ErrAlreadyInRoom.Error() {
		t.Errorf("error = %q, want %q", msg["error"], domain.ErrAlreadyInRoom.Error())
	}

	// B joins room-first too (A should still be there).
	sendFrame(t, cB, `{"type":"join","room_id":"room-first"}`)
	time.Sleep(50 * time.Millisecond)

	// A relays SDP to room-first — must succeed (A's room is intact).
	offer := `{"type":"sdp","room_id":"room-first","sdp":"v=0 offer"}`
	sendFrame(t, cA, offer)

	got := readRaw(t, cB)
	if string(got) != offer {
		t.Errorf("B received %q, want %q (A still active in room-first)", got, offer) // fix1
	}
}

func TestIntegration_JoinSameRoom_Idempotent(t *testing.T) {
	t.Parallel()

	// criterion: fix1 idempotent — re-joining the SAME room is allowed silently.
	// Fails if idempotent re-join causes an error or breaks relay.
	sessionRepo := newFakeSessionRepo()
	sessionRepo.add("tokA", 1)
	sessionRepo.add("tokB", 2)

	roomRepo := newFakeRoomRepo()
	srv := newIntegrationServer(t, sessionRepo, roomRepo)

	cA := dialWithCookie(t, srv, "tokA")
	defer cA.CloseNow() //nolint:errcheck

	cB := dialWithCookie(t, srv, "tokB")
	defer cB.CloseNow() //nolint:errcheck

	sendFrame(t, cA, `{"type":"join","room_id":"room-idem"}`)
	sendFrame(t, cB, `{"type":"join","room_id":"room-idem"}`)
	time.Sleep(50 * time.Millisecond)

	// A re-joins the SAME room — must not produce an error frame.
	sendFrame(t, cA, `{"type":"join","room_id":"room-idem"}`)
	time.Sleep(30 * time.Millisecond)

	// Relay must still work: A sends SDP and B receives it.
	offer := `{"type":"sdp","room_id":"room-idem","sdp":"v=0 idempotent"}`
	sendFrame(t, cA, offer)

	got := readRaw(t, cB)
	if string(got) != offer {
		t.Errorf("B received %q, want %q after idempotent re-join", got, offer)
	}
}

func TestIntegration_MultiRoomDisconnectCleansOriginalRoom(t *testing.T) {
	t.Parallel()

	// criterion: fix1/5 — after a rejected multi-room attempt, disconnect still
	// cleans up the original room (peer_left sent to room-first's peer).
	// Fails if the leak prevents cleanup.
	sessionRepo := newFakeSessionRepo()
	sessionRepo.add("tokA", 1)
	sessionRepo.add("tokB", 2)

	roomRepo := newFakeRoomRepo()
	srv := newIntegrationServer(t, sessionRepo, roomRepo)

	cA := dialWithCookie(t, srv, "tokA")
	cB := dialWithCookie(t, srv, "tokB")
	defer cB.CloseNow() //nolint:errcheck

	sendFrame(t, cA, `{"type":"join","room_id":"room-orig"}`)
	sendFrame(t, cB, `{"type":"join","room_id":"room-orig"}`)
	time.Sleep(50 * time.Millisecond)

	// A tries (and fails) to join another room.
	sendFrame(t, cA, `{"type":"join","room_id":"room-other"}`)

	// Drain the error frame so it does not interfere.
	readJSON(t, cA)

	// Now A disconnects — B should get outcome (forfeit) then peer_left.
	cA.CloseNow() //nolint:errcheck

	// criterion: fix1/5 — fails if peer_left not sent after A disconnect
	outcomeType := readMsgType(t, cB)
	if outcomeType != "outcome" {
		t.Errorf("B first frame type = %q, want outcome after A disconnect", outcomeType)
	}

	peerLeftType := readMsgType(t, cB)
	if peerLeftType != "peer_left" {
		t.Errorf("B second frame type = %q, want peer_left after A disconnect", peerLeftType) // fix1/5
	}
}

// ─── Private rooms: invite-a-friend (issue #96) integration tests ────────────

func TestIntegration_CreateAndJoinByCode_RelaysBetweenPeers(t *testing.T) {
	t.Parallel()

	// criterion: 7,8 — create_room mints a room+code; join_room admits the
	// second peer into the SAME room, and SDP relay flows between them.
	sessionRepo := newFakeSessionRepo()
	sessionRepo.add("tokCreator", 1)
	sessionRepo.add("tokJoiner", 2)

	roomRepo := newFakeRoomRepo()
	roomCodeRepo := newFakeRoomCodeRepo()
	srv := newIntegrationServerWithCodes(t, sessionRepo, roomRepo, roomCodeRepo)

	creator := dialWithCookie(t, srv, "tokCreator")
	defer creator.CloseNow() //nolint:errcheck

	sendFrame(t, creator, `{"type":"create_room"}`)

	var created struct {
		Type   string `json:"type"`
		RoomID string `json:"room_id"`
		Code   string `json:"code"`
	}

	rawCreated := readRaw(t, creator)
	if err := json.Unmarshal(rawCreated, &created); err != nil {
		t.Fatalf("unmarshal room_created: %v", err)
	}

	if created.Type != "room_created" {
		t.Fatalf("type = %q, want room_created", created.Type) // criterion: 7
	}

	if created.RoomID == "" || created.Code == "" {
		t.Fatalf("room_created missing room_id/code: %+v", created) // criterion: 7
	}

	joiner := dialWithCookie(t, srv, "tokJoiner")
	defer joiner.CloseNow() //nolint:errcheck

	sendFrame(t, joiner, fmt.Sprintf(`{"type":"join_room","code":%q}`, created.Code))

	var joined struct {
		Type   string `json:"type"`
		RoomID string `json:"room_id"`
	}

	rawJoined := readRaw(t, joiner)
	if err := json.Unmarshal(rawJoined, &joined); err != nil {
		t.Fatalf("unmarshal room_joined: %v", err)
	}

	if joined.Type != "room_joined" {
		t.Fatalf("type = %q, want room_joined", joined.Type) // criterion: 8
	}

	if joined.RoomID != created.RoomID {
		t.Errorf("joiner's room_id = %q, want %q (same room as creator)", joined.RoomID, created.RoomID) // criterion: 8 — fails if not the same room
	}

	time.Sleep(50 * time.Millisecond)

	// SDP relay must flow between creator and joiner — proves both are wired
	// into the existing #24 relay hub via the SAME room_id.
	offer := fmt.Sprintf(`{"type":"sdp","room_id":%q,"sdp":"v=0 private offer"}`, created.RoomID)
	sendFrame(t, creator, offer)

	got := readRaw(t, joiner)
	if string(got) != offer {
		t.Errorf("joiner received %q, want %q (verbatim relay via private room)", got, offer) // criterion: 8
	}
}

func TestIntegration_JoinRoom_InvalidCode(t *testing.T) {
	t.Parallel()

	// criterion: 9 — an unknown/expired invite code is rejected, connection stays open
	sessionRepo := newFakeSessionRepo()
	sessionRepo.add("tokJoiner", 2)

	roomRepo := newFakeRoomRepo()
	roomCodeRepo := newFakeRoomCodeRepo()
	srv := newIntegrationServerWithCodes(t, sessionRepo, roomRepo, roomCodeRepo)

	c := dialWithCookie(t, srv, "tokJoiner")
	defer c.CloseNow() //nolint:errcheck

	sendFrame(t, c, `{"type":"join_room","code":"NOSUCHCODE"}`)

	msg := readJSON(t, c)
	if msg["type"] != "error" {
		t.Errorf("type = %q, want error", msg["type"]) // criterion: 9 — fails if invalid code accepted
	}

	if msg["error"] != domain.ErrInvalidCode.Error() {
		t.Errorf("error = %q, want %q", msg["error"], domain.ErrInvalidCode.Error())
	}
}

func TestIntegration_JoinRoom_ThirdPeerToFullPrivateRoomRejected(t *testing.T) {
	t.Parallel()

	// criterion: 10 — a third join_room to an already-full private room is
	// rejected with an error frame and the connection is closed.
	sessionRepo := newFakeSessionRepo()
	sessionRepo.add("tokCreator", 1)
	sessionRepo.add("tokJoiner", 2)
	sessionRepo.add("tokStranger", 3)

	roomRepo := newFakeRoomRepo()
	roomCodeRepo := newFakeRoomCodeRepo()
	srv := newIntegrationServerWithCodes(t, sessionRepo, roomRepo, roomCodeRepo)

	creator := dialWithCookie(t, srv, "tokCreator")
	defer creator.CloseNow() //nolint:errcheck

	sendFrame(t, creator, `{"type":"create_room"}`)

	var created struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(readRaw(t, creator), &created); err != nil {
		t.Fatalf("unmarshal room_created: %v", err)
	}

	joiner := dialWithCookie(t, srv, "tokJoiner")
	defer joiner.CloseNow() //nolint:errcheck

	sendFrame(t, joiner, fmt.Sprintf(`{"type":"join_room","code":%q}`, created.Code))
	readRaw(t, joiner) // drain room_joined

	time.Sleep(30 * time.Millisecond)

	stranger := dialWithCookie(t, srv, "tokStranger")
	defer stranger.CloseNow() //nolint:errcheck

	sendFrame(t, stranger, fmt.Sprintf(`{"type":"join_room","code":%q}`, created.Code))

	msg := readJSON(t, stranger)
	if msg["type"] != "error" {
		t.Errorf("type = %q, want error", msg["type"]) // criterion: 10 — fails if third peer admitted
	}

	expectClosed(t, stranger) // criterion: 10 — fails if connection not closed for a full private room
}

func TestIntegration_CreateRoom_CreatorLeaveCleansUpCode(t *testing.T) {
	t.Parallel()

	// criterion: 11 — when the creator disconnects before anyone joins, the
	// room AND its invite code are cleaned up (RemoveCode called / mapping gone).
	sessionRepo := newFakeSessionRepo()
	sessionRepo.add("tokCreator", 1)

	roomRepo := newFakeRoomRepo()
	roomCodeRepo := newFakeRoomCodeRepo()
	srv := newIntegrationServerWithCodes(t, sessionRepo, roomRepo, roomCodeRepo)

	creator := dialWithCookie(t, srv, "tokCreator")

	sendFrame(t, creator, `{"type":"create_room"}`)

	var created struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(readRaw(t, creator), &created); err != nil {
		t.Fatalf("unmarshal room_created: %v", err)
	}

	if !roomCodeRepo.hasCode(created.Code) {
		t.Fatalf("code %q not stored right after create_room", created.Code)
	}

	// Creator disconnects before anyone joins.
	creator.CloseNow() //nolint:errcheck

	// Poll briefly for the async Leave cleanup to run.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && roomCodeRepo.hasCode(created.Code) {
		time.Sleep(10 * time.Millisecond)
	}

	if roomCodeRepo.hasCode(created.Code) {
		t.Errorf("code %q still present after creator disconnect, want removed", created.Code) // criterion: 11
	}
}

// ─── FIX 2 test: keepalive closes dead connections ────────────────────────────

func TestSignalingHandler_Keepalive_ClosesDeadConn(t *testing.T) {
	t.Parallel()

	// criterion: fix2 — when the keepalive goroutine's Ping fails (dead peer),
	// the connection is closed and leaveOnDisconnect runs.
	// We use a very short interval and ping timeout so the test is fast.
	ctrl := gomock.NewController(t)
	sessionRepo := repomocks.NewMockSessionRepository(ctrl)
	svc := svcmocks.NewMockSignalingService(ctrl)

	sessionRepo.EXPECT().UserIDBySession(gomock.Any(), "tok").Return(int64(1), nil)

	joinDone := make(chan struct{})
	svc.EXPECT().
		Join(gomock.Any(), gomock.Any(), "room-ka", gomock.Any()).
		DoAndReturn(func(_ context.Context, _ service.Conn, _ string, _ string) error {
			close(joinDone)
			return nil
		})

	leaveDone := make(chan struct{})
	svc.EXPECT().
		Leave(gomock.Any(), gomock.Any(), "room-ka").
		DoAndReturn(func(_ context.Context, _ service.Conn, _ string) {
			close(leaveDone)
		})

	// Very short keepalive so the test does not take long.
	const (
		kaInterval = 20 * time.Millisecond
		kaPingTO   = 5 * time.Millisecond
	)

	handler := NewSignalingHandler(zap.NewNop(), sessionRepo, svc, "session", kaInterval, kaPingTO)
	srv := httptest.NewServer(makeHTTPHandler(handler))
	t.Cleanup(srv.Close)

	c := dialWithCookie(t, srv, "tok")

	// Join a room so leaveOnDisconnect has a room to clean up.
	sendFrame(t, c, `{"type":"join","room_id":"room-ka"}`)

	select {
	case <-joinDone:
	case <-time.After(2 * time.Second):
		t.Fatal("join not processed")
	}

	// Simulate a dead peer: stop reading from the client so the server-side
	// conn.Ping will time out (the pong won't arrive in kaPingTO).
	// We forcibly close the underlying network connection via CloseNow so the
	// server's Ping errors out immediately.
	c.CloseNow() //nolint:errcheck

	// The keepalive goroutine should detect the dead conn and trigger Leave.
	select {
	case <-leaveDone:
		// fix2 — Leave was called after keepalive detected dead connection
	case <-time.After(3 * time.Second):
		t.Fatal("Leave not called after keepalive detected dead connection") // fix2 — fails if keepalive does not trigger cleanup
	}
}
