package delivery

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/coder/websocket"
	"go.uber.org/zap"

	"github.com/pizdagladki/full/services/signaling/internal/api/domain"
	"github.com/pizdagladki/full/services/signaling/internal/api/repository"
	"github.com/pizdagladki/full/services/signaling/internal/api/service"
)

const (
	// wsReadLimit is generous because SDP offers can be multi-KB.
	wsReadLimit = 64 * 1024
	// sendTimeout caps how long a single Send to a peer may block.
	sendTimeout = 5 * time.Second
	// cleanupTimeout is the budget for Leave / Redis cleanup after disconnect.
	cleanupTimeout = 5 * time.Second
	// internalError is the redacted error message sent to clients for non-sentinel errors.
	internalError = "internal error"
)

// signalingHandler is the WS handler implementation.
type signalingHandler struct {
	logger               *zap.Logger
	sessionRepo          repository.SessionRepository
	svc                  service.SignalingService
	cookieName           string
	keepaliveInterval    time.Duration
	keepalivePingTimeout time.Duration
}

// NewSignalingHandler creates a new SignalingHandler.
// keepaliveInterval controls how often a server-side Ping is sent to detect dead
// connections. Zero disables keepalives (useful in tests).
// keepalivePingTimeout is the per-ping deadline; on failure the connection is
// forcibly closed so the read loop's leaveOnDisconnect runs cleanup.
func NewSignalingHandler(
	logger *zap.Logger,
	sessionRepo repository.SessionRepository,
	svc service.SignalingService,
	cookieName string,
	keepaliveInterval time.Duration,
	keepalivePingTimeout time.Duration,
) SignalingHandler {
	return &signalingHandler{
		logger:               logger,
		sessionRepo:          sessionRepo,
		svc:                  svc,
		cookieName:           cookieName,
		keepaliveInterval:    keepaliveInterval,
		keepalivePingTimeout: keepalivePingTimeout,
	}
}

// ServeWS handles the /ws WebSocket upgrade and drives the SDP/ICE relay loop.
func (h *signalingHandler) ServeWS(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		// Accept already wrote the HTTP error response.
		return
	}

	defer conn.CloseNow() //nolint:errcheck // best-effort close

	// Generous read limit for SDP offers (can be multi-KB).
	conn.SetReadLimit(wsReadLimit)

	ctx := r.Context()

	// Authenticate via the session cookie.
	cookie, cookieErr := r.Cookie(h.cookieName)
	if cookieErr != nil {
		_ = conn.Close(websocket.StatusPolicyViolation, "missing session cookie")

		return
	}

	userID, sessionErr := h.sessionRepo.UserIDBySession(ctx, cookie.Value)
	if sessionErr != nil {
		_ = conn.Close(websocket.StatusPolicyViolation, "invalid or expired session")

		return
	}

	// Wrap the raw WS connection in the service's Conn abstraction.
	adapted := &adaptedConn{userID: userID, conn: conn}

	// Start the keepalive goroutine after successful authentication.
	// The goroutine shares the handler-level context; when the read loop exits
	// (any error), connCtx is canceled and the goroutine stops — no leak.
	connCtx, connCancel := context.WithCancel(ctx)
	defer connCancel()

	if h.keepaliveInterval > 0 {
		go h.runKeepalive(connCtx, conn)
	}

	// Track the room the peer has joined so we can clean up on disconnect.
	var joinedRoomID string

	// Read loop: read raw bytes, parse the envelope for type+room_id, dispatch.
	for {
		_, raw, readErr := conn.Read(ctx)
		if readErr != nil {
			// Any read error (EOF, context cancel, close frame) → leave.
			h.leaveOnDisconnect(ctx, adapted, joinedRoomID)

			return
		}

		var env domain.InboundEnvelope

		parseErr := json.Unmarshal(raw, &env)
		if parseErr != nil {
			sendErrorFrame(ctx, conn, "invalid JSON")

			continue
		}

		switch env.Type {
		case domain.TypeJoin:
			// One-room-per-connection invariant: once a peer is in a room it
			// cannot move to a different room over the same connection.
			if joinedRoomID != "" && env.RoomID != joinedRoomID {
				sendErrorFrame(ctx, conn, domain.ErrAlreadyInRoom.Error())

				continue
			}

			joinErr := h.svc.Join(ctx, adapted, env.RoomID, env.Mode)
			if joinErr != nil {
				h.handleJoinError(ctx, conn, adapted, joinErr, &joinedRoomID)

				continue
			}

			joinedRoomID = env.RoomID

		case domain.TypeCreateRoom:
			h.handleCreateRoom(ctx, conn, adapted, userID, &joinedRoomID)

		case domain.TypeJoinRoom:
			h.handleJoinRoom(ctx, conn, adapted, userID, env.Code, &joinedRoomID)

		case domain.TypeSDP, domain.TypeICE:
			relayErr := h.svc.Relay(ctx, adapted, env.RoomID, raw)
			if relayErr != nil {
				// Non-member / invalid — send error frame but keep connection open.
				h.logger.Debug("relay rejected",
					zap.Int64("user_id", userID),
					zap.String("type", env.Type),
					zap.String("room_id", env.RoomID),
					zap.Error(relayErr),
				)
				sendErrorFrame(ctx, conn, safeRelayErrMsg(relayErr))
			}

		case domain.TypeBlink, domain.TypeFaceLost:
			reportErr := h.svc.ReportEvent(ctx, adapted, env.RoomID, env.Type)
			if reportErr != nil {
				h.logger.Debug("report event rejected",
					zap.Int64("user_id", userID),
					zap.String("type", env.Type),
					zap.String("room_id", env.RoomID),
					zap.Error(reportErr),
				)
				// ErrMatchFinished and ErrNotMember: send error frame but keep connection open.
				sendErrorFrame(ctx, conn, safeReportErrMsg(reportErr))
			}

		default:
			sendErrorFrame(ctx, conn, "unknown message type")
		}
	}
}

// runKeepalive periodically sends a Ping to detect dead connections.
// On ping failure it closes the raw connection, which unblocks the read loop
// so the existing leaveOnDisconnect cleanup runs. The goroutine exits when ctx
// is canceled (which happens when the read loop returns).
func (h *signalingHandler) runKeepalive(ctx context.Context, conn *websocket.Conn) {
	ticker := time.NewTicker(h.keepaliveInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			pingCtx, cancel := context.WithTimeout(ctx, h.keepalivePingTimeout)
			pingErr := conn.Ping(pingCtx)
			cancel()

			if pingErr != nil {
				// The peer is unresponsive. Close the connection so the read
				// loop in ServeWS unblocks and leaveOnDisconnect runs.
				conn.CloseNow() //nolint:errcheck // best-effort

				return
			}
		}
	}
}

// handleJoinError sends an appropriate error frame and, for ErrRoomFull, closes
// the connection. Validation errors are forwarded as-is; others are redacted.
func (h *signalingHandler) handleJoinError(
	ctx context.Context,
	conn *websocket.Conn,
	_ service.Conn,
	joinErr error,
	joinedRoomID *string,
) {
	var msg string

	switch {
	case errors.Is(joinErr, domain.ErrRoomFull):
		msg = domain.ErrRoomFull.Error()
	case errors.Is(joinErr, domain.ErrInvalidRoomID):
		msg = domain.ErrInvalidRoomID.Error()
	default:
		msg = internalError
	}

	sendErrorFrame(ctx, conn, msg)

	if errors.Is(joinErr, domain.ErrRoomFull) {
		_ = conn.Close(websocket.StatusPolicyViolation, "room full")
	}

	_ = joinedRoomID // joinedRoomID unchanged on failure
}

// handleCreateRoom processes a create_room message: enforces the
// one-room-per-connection invariant, calls svc.CreateRoom, and on success
// writes a room_created frame and records the new joinedRoomID.
func (h *signalingHandler) handleCreateRoom(
	ctx context.Context,
	conn *websocket.Conn,
	adapted service.Conn,
	userID int64,
	joinedRoomID *string,
) {
	if *joinedRoomID != "" {
		sendErrorFrame(ctx, conn, domain.ErrAlreadyInRoom.Error())

		return
	}

	roomID, code, createErr := h.svc.CreateRoom(ctx, adapted)
	if createErr != nil {
		h.logger.Error("create_room failed",
			zap.Int64("user_id", userID),
			zap.Error(createErr),
		)
		sendErrorFrame(ctx, conn, internalError)

		return
	}

	*joinedRoomID = roomID

	writeErr := conn.Write(ctx, websocket.MessageText, domain.RoomCreatedBytes(roomID, code))
	if writeErr != nil {
		h.logger.Debug("write room_created failed",
			zap.Int64("user_id", userID),
			zap.Error(writeErr),
		)
	}
}

// handleJoinRoom processes a join_room message: enforces the
// one-room-per-connection invariant, calls svc.JoinByCode, and on success
// writes a room_joined frame and records the new joinedRoomID.
func (h *signalingHandler) handleJoinRoom(
	ctx context.Context,
	conn *websocket.Conn,
	adapted service.Conn,
	userID int64,
	code string,
	joinedRoomID *string,
) {
	if *joinedRoomID != "" {
		sendErrorFrame(ctx, conn, domain.ErrAlreadyInRoom.Error())

		return
	}

	roomID, joinErr := h.svc.JoinByCode(ctx, adapted, code)
	if joinErr != nil {
		h.handleJoinByCodeError(ctx, conn, joinErr)

		return
	}

	*joinedRoomID = roomID

	writeErr := conn.Write(ctx, websocket.MessageText, domain.RoomJoinedBytes(roomID))
	if writeErr != nil {
		h.logger.Debug("write room_joined failed",
			zap.Int64("user_id", userID),
			zap.Error(writeErr),
		)
	}
}

// handleJoinByCodeError sends an appropriate error frame for a join_room
// failure. ErrInvalidCode keeps the connection open (the client may retry
// with a corrected code); ErrRoomFull closes it, mirroring handleJoinError.
func (h *signalingHandler) handleJoinByCodeError(ctx context.Context, conn *websocket.Conn, joinErr error) {
	var msg string

	switch {
	case errors.Is(joinErr, domain.ErrInvalidCode):
		msg = domain.ErrInvalidCode.Error()
	case errors.Is(joinErr, domain.ErrRoomFull):
		msg = domain.ErrRoomFull.Error()
	default:
		msg = internalError
	}

	sendErrorFrame(ctx, conn, msg)

	if errors.Is(joinErr, domain.ErrRoomFull) {
		_ = conn.Close(websocket.StatusPolicyViolation, "room full")
	}
}

// leaveOnDisconnect calls svc.Leave with a fresh context detached from the
// request context (which is already canceled on disconnect).
func (h *signalingHandler) leaveOnDisconnect(_ context.Context, conn service.Conn, roomID string) {
	if roomID == "" {
		return
	}

	leaveCtx, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
	defer cancel()

	h.svc.Leave(leaveCtx, conn, roomID) //nolint:contextcheck // intentional detach from caller ctx
}

// safeRelayErrMsg returns a client-safe error string for relay failures.
func safeRelayErrMsg(err error) string {
	if errors.Is(err, domain.ErrNotMember) {
		return domain.ErrNotMember.Error()
	}

	return internalError
}

// safeReportErrMsg returns a client-safe error string for blink/face_lost report failures.
func safeReportErrMsg(err error) string {
	switch {
	case errors.Is(err, domain.ErrNotMember):
		return domain.ErrNotMember.Error()
	case errors.Is(err, domain.ErrMatchFinished):
		return domain.ErrMatchFinished.Error()
	default:
		return internalError
	}
}

// sendErrorFrame writes a JSON {"type":"error","error":"<reason>"} frame.
// Best-effort — errors are silently ignored.
func sendErrorFrame(ctx context.Context, conn *websocket.Conn, reason string) {
	_ = conn.Write(ctx, websocket.MessageText, domain.ErrorBytes(reason))
}

// adaptedConn wraps *websocket.Conn to implement service.Conn.
type adaptedConn struct {
	userID int64
	conn   *websocket.Conn
}

func (c *adaptedConn) UserID() int64 { return c.userID }

// Send writes raw bytes as a single WS text frame with a bounded timeout.
func (c *adaptedConn) Send(raw []byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), sendTimeout)
	defer cancel()

	return c.conn.Write(ctx, websocket.MessageText, raw)
}

// Close sends a close frame with the given reason.
func (c *adaptedConn) Close(reason string) {
	_ = c.conn.Close(websocket.StatusInternalError, reason)
}
