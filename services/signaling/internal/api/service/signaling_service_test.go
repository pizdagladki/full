package service

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"go.uber.org/mock/gomock"
	"go.uber.org/zap"

	"github.com/pizdagladki/full/services/signaling/internal/api/domain"
	"github.com/pizdagladki/full/services/signaling/internal/api/repository"
	repomocks "github.com/pizdagladki/full/services/signaling/internal/api/repository/mocks"
)

// --- fakeConn ---

type fakeConn struct {
	mu       sync.Mutex
	userID   int64
	sent     [][]byte
	closeMsg string
}

func newFakeConn(userID int64) *fakeConn {
	return &fakeConn{userID: userID}
}

func (c *fakeConn) UserID() int64 { return c.userID }

func (c *fakeConn) Send(raw []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	cp := make([]byte, len(raw))
	copy(cp, raw)
	c.sent = append(c.sent, cp)

	return nil
}

func (c *fakeConn) Close(reason string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closeMsg = reason
}

func (c *fakeConn) Sent() [][]byte {
	c.mu.Lock()
	defer c.mu.Unlock()

	return append([][]byte(nil), c.sent...)
}

func (c *fakeConn) CloseMsg() string {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.closeMsg
}

// --- fixture ---

type fixture struct {
	ctrl     *gomock.Controller
	roomRepo *repomocks.MockRoomRepository
	svc      *signalingService
}

func newFixture(t *testing.T) *fixture {
	t.Helper()

	ctrl := gomock.NewController(t)
	roomRepo := repomocks.NewMockRoomRepository(ctrl)
	svc := NewSignalingService(zap.NewNop(), roomRepo, time.Now, time.AfterFunc, 150*time.Millisecond).(*signalingService)

	return &fixture{ctrl: ctrl, roomRepo: roomRepo, svc: svc}
}

// parseMsg decodes raw JSON bytes into a string map for assertions.
func parseMsg(t *testing.T, b []byte) map[string]string {
	t.Helper()

	var msg map[string]string
	if err := json.Unmarshal(b, &msg); err != nil {
		t.Fatalf("parseMsg: invalid JSON %q: %v", b, err)
	}

	return msg
}

// --- Join tests ---

func TestSignalingService_Join_Admitted(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		repoResult repository.JoinResult
		wantErr    error
	}{
		{
			// criterion: 1 — fails if first join rejected
			name:       "first peer admitted (JoinResultJoined)",
			repoResult: repository.JoinResultJoined,
			wantErr:    nil,
		},
		{
			// criterion: 1 — fails if already-member not allowed (idempotent)
			name:       "already-member join is idempotent",
			repoResult: repository.JoinResultAlreadyMember,
			wantErr:    nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t)
			conn := newFakeConn(1)

			f.roomRepo.EXPECT().
				Join(gomock.Any(), "room-1", int64(1)).
				Return(tt.repoResult, nil)

			err := f.svc.Join(context.Background(), conn, "room-1")
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("Join() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestSignalingService_Join_ThirdPeerRejected(t *testing.T) {
	t.Parallel()

	// criterion: 1 — third joiner to a full room is rejected with ErrRoomFull
	tests := []struct {
		name string
	}{
		{name: "third joiner to full room returns ErrRoomFull"}, // criterion: 1 — fails if third peer not rejected
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t)
			conn := newFakeConn(3)

			f.roomRepo.EXPECT().
				Join(gomock.Any(), "room-full", int64(3)).
				Return(repository.JoinResultFull, nil)

			err := f.svc.Join(context.Background(), conn, "room-full")
			if !errors.Is(err, domain.ErrRoomFull) {
				t.Errorf("Join() error = %v, want ErrRoomFull", err) // criterion: 1
			}
		})
	}
}

func TestSignalingService_Join_InvalidRoomID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		roomID string
	}{
		{
			// criterion: 1 — fails if empty room_id not validated
			name:   "empty room id rejected",
			roomID: "",
		},
		{
			// criterion: 1 — fails if oversized room_id not validated
			name:   "too long room id rejected",
			roomID: string(make([]byte, 129)),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t)
			conn := newFakeConn(1)
			// No repo call expected — validation fails first.

			err := f.svc.Join(context.Background(), conn, tt.roomID)
			if !errors.Is(err, domain.ErrInvalidRoomID) {
				t.Errorf("Join() error = %v, want ErrInvalidRoomID", err)
			}
		})
	}
}

func TestSignalingService_Join_RepoError(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	conn := newFakeConn(1)

	f.roomRepo.EXPECT().
		Join(gomock.Any(), "room-err", int64(1)).
		Return(repository.JoinResult(0), errors.New("redis down"))

	err := f.svc.Join(context.Background(), conn, "room-err")
	if err == nil {
		t.Fatal("Join() error = nil, want error from repo")
	}
}

// --- Relay tests ---

func TestSignalingService_Relay_SDPForwardedToOtherPeer(t *testing.T) {
	t.Parallel()

	// criterion: 2 — SDP forwarded verbatim to B only, never echoed to A, never to other rooms
	tests := []struct {
		name    string
		payload []byte
	}{
		{
			name:    "SDP offer forwarded verbatim to peer only", // criterion: 2 — fails if SDP not forwarded or echoed
			payload: []byte(`{"type":"sdp","sdp":"v=0 offer..."}`),
		},
		{
			name:    "SDP answer forwarded verbatim to peer only", // criterion: 2 — fails if answer not forwarded
			payload: []byte(`{"type":"sdp","sdp":"v=0 answer..."}`),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t)
			ctx := context.Background()
			connA := newFakeConn(1)
			connB := newFakeConn(2)

			f.svc.mu.Lock()
			f.svc.rooms["room-sdp"] = map[int64]Conn{1: connA, 2: connB}
			f.svc.mu.Unlock()

			err := f.svc.Relay(ctx, connA, "room-sdp", tt.payload)
			if err != nil {
				t.Fatalf("Relay() error = %v", err)
			}

			// Peer B received the frame verbatim.
			sentB := connB.Sent()
			if len(sentB) != 1 {
				t.Fatalf("connB received %d frames, want 1", len(sentB)) // criterion: 2
			}

			if string(sentB[0]) != string(tt.payload) {
				t.Errorf("connB received %q, want %q (verbatim)", sentB[0], tt.payload) // criterion: 2 — not verbatim
			}

			// Sender A received nothing (no echo).
			if n := len(connA.Sent()); n != 0 {
				t.Errorf("sender received %d frames, want 0 (echo not allowed)", n) // criterion: 2 — echoed
			}
		})
	}
}

func TestSignalingService_Relay_ICEBothDirectionsTrickle(t *testing.T) {
	t.Parallel()

	// criterion: 3 — ICE from either member forwarded; trickle (multiple) supported
	tests := []struct {
		name     string
		senderID int64
		peerID   int64
		messages [][]byte
	}{
		{
			// criterion: 3 — fails if ICE from A not relayed to B
			name:     "ICE trickle from A to B",
			senderID: 1,
			peerID:   2,
			messages: [][]byte{
				[]byte(`{"type":"ice","candidate":"a1"}`),
				[]byte(`{"type":"ice","candidate":"a2"}`),
				[]byte(`{"type":"ice","candidate":"a3"}`),
			},
		},
		{
			// criterion: 3 — fails if ICE from B not relayed to A
			name:     "ICE trickle from B to A",
			senderID: 2,
			peerID:   1,
			messages: [][]byte{
				[]byte(`{"type":"ice","candidate":"b1"}`),
				[]byte(`{"type":"ice","candidate":"b2"}`),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t)
			ctx := context.Background()
			connA := newFakeConn(1)
			connB := newFakeConn(2)

			f.svc.mu.Lock()
			f.svc.rooms["room-ice"] = map[int64]Conn{1: connA, 2: connB}
			f.svc.mu.Unlock()

			var sender, peer *fakeConn
			if tt.senderID == 1 {
				sender, peer = connA, connB
			} else {
				sender, peer = connB, connA
			}

			for _, msg := range tt.messages {
				if err := f.svc.Relay(ctx, sender, "room-ice", msg); err != nil {
					t.Fatalf("Relay() error = %v", err)
				}
			}

			sent := peer.Sent()
			if len(sent) != len(tt.messages) {
				t.Fatalf("peer received %d ICE frames, want %d", len(sent), len(tt.messages)) // criterion: 3
			}

			for i, want := range tt.messages {
				if string(sent[i]) != string(want) {
					t.Errorf("ICE[%d] got %q, want %q", i, sent[i], want) // criterion: 3 — not verbatim
				}
			}

			// Sender received nothing.
			if n := len(sender.Sent()); n != 0 {
				t.Errorf("sender received %d frames, want 0", n)
			}
		})
	}
}

func TestSignalingService_Relay_NonMemberRejected(t *testing.T) {
	t.Parallel()

	// criterion: 4 — message from non-member rejected, not relayed
	tests := []struct {
		name     string
		roomID   string
		hasRoom  bool
		stranger int64
	}{
		{
			// criterion: 4 — fails if non-member can relay to members
			name:     "stranger in an existing room rejected",
			roomID:   "room-nm",
			hasRoom:  true,
			stranger: 99,
		},
		{
			// criterion: 4 — fails if relay to non-joined room allowed
			name:     "relay to non-existent room rejected",
			roomID:   "room-ghost",
			hasRoom:  false,
			stranger: 99,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t)
			ctx := context.Background()
			connA := newFakeConn(1)
			connB := newFakeConn(2)
			connStranger := newFakeConn(tt.stranger)

			if tt.hasRoom {
				f.svc.mu.Lock()
				f.svc.rooms[tt.roomID] = map[int64]Conn{1: connA, 2: connB}
				f.svc.mu.Unlock()
			}

			err := f.svc.Relay(ctx, connStranger, tt.roomID, []byte(`{"type":"sdp"}`))
			if !errors.Is(err, domain.ErrNotMember) {
				t.Errorf("Relay() error = %v, want ErrNotMember", err) // criterion: 4
			}

			if tt.hasRoom {
				if n := len(connA.Sent()); n != 0 {
					t.Errorf("connA received %d frames from non-member, want 0", n)
				}

				if n := len(connB.Sent()); n != 0 {
					t.Errorf("connB received %d frames from non-member, want 0", n)
				}
			}
		})
	}
}

// --- Leave tests ---

func TestSignalingService_Leave_SendsPeerLeftAndCleansUp(t *testing.T) {
	t.Parallel()

	// criterion: 5 — peer_left sent to remaining peer; room cleaned up from Redis
	tests := []struct {
		name string
	}{
		{name: "disconnecting peer triggers peer_left and Redis cleanup"}, // criterion: 5
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t)
			ctx := context.Background()
			connA := newFakeConn(1)
			connB := newFakeConn(2)

			f.svc.mu.Lock()
			f.svc.rooms["room-leave"] = map[int64]Conn{1: connA, 2: connB}
			f.svc.mu.Unlock()

			f.roomRepo.EXPECT().RemoveRoom(gomock.Any(), "room-leave").Return(nil)

			f.svc.Leave(ctx, connA, "room-leave")

			// Peer B should have received outcome (forfeit) then peer_left.
			sentB := connB.Sent()
			if len(sentB) < 2 {
				t.Fatalf("peer B received %d frames, want at least 2 (outcome + peer_left)", len(sentB)) // criterion: 5
			}

			// First frame must be outcome.
			var outcomeMsg map[string]interface{}
			if err := json.Unmarshal(sentB[0], &outcomeMsg); err != nil {
				t.Fatalf("first frame not valid JSON: %v", err)
			}

			if outcomeMsg["type"] != "outcome" {
				t.Errorf("peer B first msg type = %q, want outcome", outcomeMsg["type"]) // criterion: 5
			}

			// Second frame must be peer_left.
			msg := parseMsg(t, sentB[1])
			if msg["type"] != "peer_left" {
				t.Errorf("peer B second msg type = %q, want peer_left", msg["type"]) // criterion: 5
			}

			// Peer B's connection should be closed.
			if connB.CloseMsg() == "" {
				t.Error("peer B close not called after disconnect") // criterion: 5
			}

			// Room removed from in-process hub.
			f.svc.mu.Lock()
			_, exists := f.svc.rooms["room-leave"]
			f.svc.mu.Unlock()

			if exists {
				t.Error("room still in hub after Leave, want removed") // criterion: 5
			}
		})
	}
}

func TestSignalingService_Leave_LoneMemberNoError(t *testing.T) {
	t.Parallel()

	// criterion: 5 — lone member leaving must not panic; room cleaned up
	f := newFixture(t)
	ctx := context.Background()
	connA := newFakeConn(1)

	f.svc.mu.Lock()
	f.svc.rooms["room-lone"] = map[int64]Conn{1: connA}
	f.svc.mu.Unlock()

	f.roomRepo.EXPECT().RemoveRoom(gomock.Any(), "room-lone").Return(nil)

	// Must not panic.
	f.svc.Leave(ctx, connA, "room-lone")
}

func TestSignalingService_Leave_EmptyRoomID(t *testing.T) {
	t.Parallel()

	// Leave with empty roomID (peer never joined) must be a no-op.
	f := newFixture(t)
	conn := newFakeConn(1)
	// No repo call expected.
	f.svc.Leave(context.Background(), conn, "")
}

func TestSignalingService_Leave_RedisError_Logged(t *testing.T) {
	t.Parallel()

	// Redis RemoveRoom error must not panic; it is logged and execution continues.
	f := newFixture(t)
	ctx := context.Background()
	connA := newFakeConn(1)

	f.svc.mu.Lock()
	f.svc.rooms["room-redis-err"] = map[int64]Conn{1: connA}
	f.svc.mu.Unlock()

	f.roomRepo.EXPECT().RemoveRoom(gomock.Any(), "room-redis-err").Return(errors.New("redis timeout"))

	// Must not panic.
	f.svc.Leave(ctx, connA, "room-redis-err")
}
