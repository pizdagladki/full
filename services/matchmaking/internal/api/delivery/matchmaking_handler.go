package delivery

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"go.uber.org/zap"

	"github.com/pizdagladki/full/services/matchmaking/internal/api/domain"
	"github.com/pizdagladki/full/services/matchmaking/internal/api/repository"
	"github.com/pizdagladki/full/services/matchmaking/internal/api/service"
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
				h.svc.Leave(ctx, userID, currentMode)
			}

			return
		}

		switch msg.Type {
		case "join":
			if currentMode != "" {
				// Already in a queue — remove from old queue first.
				h.svc.Leave(ctx, userID, currentMode)
			}

			player := domain.Player{
				UserID:     userID,
				Mode:       msg.Mode,
				Level:      msg.Level,
				EnqueuedAt: time.Now(),
			}

			joinErr := h.svc.Join(ctx, adapted, player)
			if joinErr != nil {
				sendErrMsg(ctx, conn, joinErr.Error())

				continue
			}

			currentMode = msg.Mode

		case "leave":
			if currentMode != "" {
				h.svc.Leave(ctx, userID, currentMode)
				currentMode = ""
			}

		default:
			sendErrMsg(ctx, conn, domain.ErrUnknownType.Error())
		}
	}
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

func (c *adaptedConn) Send(msg domain.MatchedMessage) error {
	return wsjson.Write(context.Background(), c.conn, msg)
}

func (c *adaptedConn) Close(reason string) {
	_ = c.conn.Close(websocket.StatusPolicyViolation, reason)
}
