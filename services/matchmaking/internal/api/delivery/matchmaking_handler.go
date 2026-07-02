package delivery

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"go.uber.org/zap"

	"github.com/pizdagladki/full/services/matchmaking/internal/api/domain"
	"github.com/pizdagladki/full/services/matchmaking/internal/api/repository"
	"github.com/pizdagladki/full/services/matchmaking/internal/api/service"
)

const (
	// wsReadLimit caps inbound WS frames to 4 KiB — join/leave envelopes are tiny.
	wsReadLimit = 4096
	// cleanupTimeout is the budget for the disconnect-path Leave / HDEL calls.
	// A fresh context is used here — r.Context() is already canceled when the
	// connection drops, so reusing it would make queueRepo.Remove (HDEL) fail
	// with context.Canceled and leave a ghost entry in mm:queue:<mode>.
	cleanupTimeout = 5 * time.Second
	// sendTimeout caps how long a single Send to a matched player may block,
	// preventing a wedged client from stalling the single matcher goroutine.
	sendTimeout = 5 * time.Second
)

// matchmakingHandler is the WS handler implementation.
type matchmakingHandler struct {
	logger      *zap.Logger
	sessionRepo repository.SessionRepository
	svc         service.MatchmakingService
	cookieName  string
}

// NewMatchmakingHandler creates a new MatchmakingHandler.
func NewMatchmakingHandler(
	logger *zap.Logger,
	sessionRepo repository.SessionRepository,
	svc service.MatchmakingService,
	cookieName string,
) MatchmakingHandler {
	return &matchmakingHandler{
		logger:      logger,
		sessionRepo: sessionRepo,
		svc:         svc,
		cookieName:  cookieName,
	}
}

// ServeWS handles the /ws WebSocket upgrade and drives the read loop.
func (h *matchmakingHandler) ServeWS(w http.ResponseWriter, r *http.Request) {
	// Accept the WS connection first (InsecureSkipVerify intentionally NOT set).
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		// Accept already wrote the HTTP error response.
		return
	}

	defer conn.CloseNow() //nolint:errcheck // best-effort close

	// Limit inbound frame size: join/leave envelopes are tiny.
	conn.SetReadLimit(wsReadLimit)

	ctx := r.Context()

	// Authenticate via the session cookie.
	cookie, err := r.Cookie(h.cookieName)
	if err != nil {
		_ = conn.Close(websocket.StatusPolicyViolation, "missing session cookie")

		return
	}

	userID, err := h.sessionRepo.UserIDBySession(ctx, cookie.Value)
	if err != nil {
		_ = conn.Close(websocket.StatusPolicyViolation, "invalid or expired session")

		return
	}

	// Wrap the raw WS connection in the service's Conn abstraction.
	adapted := &adaptedConn{userID: userID, conn: conn}

	// Track the player's current mode so we can clean up on disconnect.
	var currentMode string

	// Read loop: dispatch join/leave messages to the service.
	for {
		var msg domain.InboundMessage

		readErr := wsjson.Read(ctx, conn, &msg)
		if readErr != nil {
			// Any read error (EOF, context cancel, close frame) → leave.
			if currentMode != "" {
				// Use a fresh, independent context (not r.Context()) so the
				// Redis HDEL still succeeds even after the request context is
				// canceled by the connection drop.  See cleanupTimeout const.
				leaveWithFreshCtx(ctx, h.svc, userID, currentMode)
			}

			return
		}

		switch msg.Type {
		case "join":
			if currentMode != "" {
				// Already queued — leave the old slot before joining a new one.
				leaveWithFreshCtx(ctx, h.svc, userID, currentMode)
			}

			player := domain.Player{
				UserID:     userID,
				Mode:       msg.Mode,
				Level:      msg.Level,
				EnqueuedAt: time.Now(),
			}

			joinErr := h.svc.Join(ctx, adapted, player)
			if joinErr != nil {
				var cooldownErr *domain.CooldownError
				if errors.As(joinErr, &cooldownErr) {
					sendCooldownMsg(ctx, conn, cooldownErr.SecondsRemaining)
				} else {
					sendErrMsg(ctx, conn, safeErrMsg(joinErr))
				}

				continue
			}

			currentMode = msg.Mode

		case "leave":
			if currentMode != "" {
				leaveWithFreshCtx(ctx, h.svc, userID, currentMode)
				currentMode = ""
			}

		default:
			sendErrMsg(ctx, conn, domain.ErrUnknownType.Error())
		}
	}
}

// leaveWithFreshCtx calls svc.Leave with a short-lived background context so
// the Redis HDEL is not affected by a canceled request context (which would
// leave a ghost entry in mm:queue:<mode>).
// The _ parameter is accepted so callers satisfy contextcheck; it is
// intentionally NOT used — the whole point is to detach from it.
func leaveWithFreshCtx(_ context.Context, svc service.MatchmakingService, userID int64, mode string) {
	ctx, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
	defer cancel()

	svc.Leave(ctx, userID, mode) //nolint:contextcheck // intentional detach from caller ctx
}

// safeErrMsg returns a client-safe error string. Domain sentinel errors
// (ErrInvalidLevel, ErrInvalidMode) are forwarded as-is; everything else
// is redacted to "internal error" so Redis addresses and backend details
// never reach the WS client.
func safeErrMsg(err error) string {
	if errors.Is(err, domain.ErrInvalidLevel) || errors.Is(err, domain.ErrInvalidMode) {
		return err.Error()
	}

	return "internal error"
}

// sendCooldownMsg writes a JSON cooldown error envelope to the client. Best-effort.
func sendCooldownMsg(ctx context.Context, conn *websocket.Conn, secondsRemaining int) {
	msg := struct {
		Type             string `json:"type"`
		Reason           string `json:"reason"`
		SecondsRemaining int    `json:"seconds_remaining"`
	}{Type: "error", Reason: "cooldown", SecondsRemaining: secondsRemaining}

	data, err := json.Marshal(msg)
	if err != nil {
		return
	}

	_ = conn.Write(ctx, websocket.MessageText, data)
}

// sendErrMsg writes a JSON error envelope to the client. Best-effort.
func sendErrMsg(ctx context.Context, conn *websocket.Conn, reason string) {
	msg := struct {
		Type  string `json:"type"`
		Error string `json:"error"`
	}{Type: "error", Error: reason}

	data, err := json.Marshal(msg)
	if err != nil {
		return
	}

	_ = conn.Write(ctx, websocket.MessageText, data)
}

// adaptedConn wraps *websocket.Conn to implement service.Conn.
type adaptedConn struct {
	userID int64
	conn   *websocket.Conn
}

func (c *adaptedConn) UserID() int64 { return c.userID }

// Send writes a MatchedMessage to the peer with a bounded timeout so a wedged
// client cannot stall the single matcher goroutine indefinitely.
func (c *adaptedConn) Send(msg domain.MatchedMessage) error {
	ctx, cancel := context.WithTimeout(context.Background(), sendTimeout)
	defer cancel()

	return wsjson.Write(ctx, c.conn, msg)
}

func (c *adaptedConn) Close(reason string) {
	_ = c.conn.Close(websocket.StatusInternalError, reason)
}
