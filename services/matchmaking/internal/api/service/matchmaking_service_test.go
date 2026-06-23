package service

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"go.uber.org/mock/gomock"
	"go.uber.org/zap"

	"github.com/pizdagladki/full/services/matchmaking/internal/api/domain"
	"github.com/pizdagladki/full/services/matchmaking/internal/api/repository/mocks"
)

// --- fakes ---

type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

type fakeConn struct {
	mu       sync.Mutex
	userID   int64
	sent     []domain.MatchedMessage
	closeMsg string
}

func newFakeConn(userID int64) *fakeConn {
	return &fakeConn{userID: userID}
}

func (c *fakeConn) UserID() int64 { return c.userID }

func (c *fakeConn) Send(msg domain.MatchedMessage) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sent = append(c.sent, msg)
	return nil
}

func (c *fakeConn) Close(reason string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closeMsg = reason
}

func (c *fakeConn) Messages() []domain.MatchedMessage {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]domain.MatchedMessage(nil), c.sent...)
}

type errConn struct {
	fakeConn
}

func (c *errConn) Send(_ domain.MatchedMessage) error {
	return fmt.Errorf("send failed")
}

type deterministicRoomIDGen struct {
	id string
}

func (g *deterministicRoomIDGen) NewRoomID() string { return g.id }

// --- helpers ---

type serviceFixture struct {
	ctrl      *gomock.Controller
	queueRepo *mocks.MockQueueRepository
	clock     *fakeClock
	roomGen   *deterministicRoomIDGen
	svc       MatchmakingService
}

func newFixture(t *testing.T, levelDist int, fallbackAfter time.Duration) *serviceFixture {
	t.Helper()

	ctrl := gomock.NewController(t)
	queueRepo := mocks.NewMockQueueRepository(ctrl)
	clock := &fakeClock{now: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)}
	roomGen := &deterministicRoomIDGen{id: "room-1"}

	svc := NewMatchmakingService(
		zap.NewNop(),
		queueRepo,
		clock,
		roomGen,
		levelDist,
		fallbackAfter,
	)

	return &serviceFixture{
		ctrl:      ctrl,
		queueRepo: queueRepo,
		clock:     clock,
		roomGen:   roomGen,
		svc:       svc,
	}
}

// --- tests ---

func TestMatchmakingService_Join_Validation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		player  domain.Player
		wantErr error
	}{
		{
			name:    "invalid level zero",
			player:  domain.Player{UserID: 1, Mode: "ranked", Level: 0},
			wantErr: domain.ErrInvalidLevel,
		},
		{
			name:    "invalid level eleven",
			player:  domain.Player{UserID: 1, Mode: "ranked", Level: 11},
			wantErr: domain.ErrInvalidLevel,
		},
		{
			name:    "invalid empty mode",
			player:  domain.Player{UserID: 1, Mode: "", Level: 5},
			wantErr: domain.ErrInvalidMode,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t, 3, 10*time.Second)
			// No repo calls expected — validation fails first.
			conn := newFakeConn(tt.player.UserID)
			err := f.svc.Join(context.Background(), conn, tt.player)

			if err == nil {
				t.Fatal("Join() error = nil, want validation error")
			}
		})
	}
}

func TestMatchmakingService_Join_Success(t *testing.T) {
	t.Parallel()

	f := newFixture(t, 3, 10*time.Second)
	ctx := context.Background()
	player := domain.Player{UserID: 1, Mode: "ranked", Level: 5, EnqueuedAt: f.clock.Now()}
	conn := newFakeConn(1)

	f.queueRepo.EXPECT().Enqueue(gomock.Any(), player).Return(nil)

	err := f.svc.Join(ctx, conn, player)
	if err != nil {
		t.Fatalf("Join() error = %v", err)
	}
}

func TestMatchmakingService_Join_EnqueueError(t *testing.T) {
	t.Parallel()

	f := newFixture(t, 3, 10*time.Second)
	ctx := context.Background()
	player := domain.Player{UserID: 1, Mode: "ranked", Level: 5, EnqueuedAt: f.clock.Now()}
	conn := newFakeConn(1)

	f.queueRepo.EXPECT().Enqueue(gomock.Any(), player).Return(fmt.Errorf("redis down"))

	err := f.svc.Join(ctx, conn, player)
	if err == nil {
		t.Fatal("Join() error = nil, want error from repo")
	}
}

func TestMatchmakingService_Leave(t *testing.T) {
	t.Parallel()

	f := newFixture(t, 3, 10*time.Second)
	ctx := context.Background()
	player := domain.Player{UserID: 1, Mode: "ranked", Level: 5, EnqueuedAt: f.clock.Now()}
	conn := newFakeConn(1)

	f.queueRepo.EXPECT().Enqueue(gomock.Any(), player).Return(nil)

	if err := f.svc.Join(ctx, conn, player); err != nil {
		t.Fatalf("Join: %v", err)
	}

	f.queueRepo.EXPECT().Remove(gomock.Any(), "ranked", int64(1)).Return(true, nil)
	f.svc.Leave(ctx, 1, "ranked")

	// After Leave the player's mode should not appear in activeModes.
	ms := f.svc.(*matchmakingService)
	ms.mu.Lock()
	_, stillConn := ms.conns[1]
	_, stillMode := ms.modes[1]
	ms.mu.Unlock()

	if stillConn {
		t.Error("conn still registered after Leave")
	}
	if stillMode {
		t.Error("mode still registered after Leave")
	}
}

func TestMatchmakingService_Tick_InDistanceMatch(t *testing.T) {
	t.Parallel()

	f := newFixture(t, 3, 10*time.Second)
	ctx := context.Background()
	now := f.clock.Now()

	pA := domain.Player{UserID: 1, Mode: "ranked", Level: 5, EnqueuedAt: now}
	pB := domain.Player{UserID: 2, Mode: "ranked", Level: 7, EnqueuedAt: now} // diff=2 ≤ 3

	connA := newFakeConn(1)
	connB := newFakeConn(2)

	// Register both conns via Join.
	f.queueRepo.EXPECT().Enqueue(gomock.Any(), pA).Return(nil)
	f.queueRepo.EXPECT().Enqueue(gomock.Any(), pB).Return(nil)

	if err := f.svc.Join(ctx, connA, pA); err != nil {
		t.Fatalf("Join A: %v", err)
	}
	if err := f.svc.Join(ctx, connB, pB); err != nil {
		t.Fatalf("Join B: %v", err)
	}

	// Tick: lists waiting, attempts pair.
	f.queueRepo.EXPECT().ListWaiting(gomock.Any(), "ranked").Return([]domain.Player{pA, pB}, nil)
	f.queueRepo.EXPECT().Pair(gomock.Any(), pA, pB).Return(true, nil)

	f.svc.Tick(ctx)

	// Both connections receive the same room_id, with the correct opponent.
	msgsA := connA.Messages()
	msgsB := connB.Messages()

	if len(msgsA) != 1 {
		t.Fatalf("connA got %d messages, want 1", len(msgsA))
	}
	if len(msgsB) != 1 {
		t.Fatalf("connB got %d messages, want 1", len(msgsB))
	}

	if msgsA[0].RoomID != msgsB[0].RoomID {
		t.Errorf("room_id mismatch: A=%s B=%s", msgsA[0].RoomID, msgsB[0].RoomID)
	}
	if msgsA[0].Opponent != pB.UserID {
		t.Errorf("A.Opponent = %d, want %d", msgsA[0].Opponent, pB.UserID)
	}
	if msgsB[0].Opponent != pA.UserID {
		t.Errorf("B.Opponent = %d, want %d", msgsB[0].Opponent, pA.UserID)
	}
	if msgsA[0].Type != "matched" {
		t.Errorf("A.Type = %q, want matched", msgsA[0].Type)
	}
}

func TestMatchmakingService_Tick_FallbackAfterDeadline(t *testing.T) {
	t.Parallel()

	f := newFixture(t, 1, 10*time.Second) // dist=1 → pA and pB (diff=5) won't match in-distance
	ctx := context.Background()
	now := f.clock.Now()

	pA := domain.Player{UserID: 1, Mode: "ranked", Level: 1, EnqueuedAt: now}
	pB := domain.Player{UserID: 2, Mode: "ranked", Level: 6, EnqueuedAt: now} // diff=5 > 1

	connA := newFakeConn(1)
	connB := newFakeConn(2)

	f.queueRepo.EXPECT().Enqueue(gomock.Any(), pA).Return(nil)
	f.queueRepo.EXPECT().Enqueue(gomock.Any(), pB).Return(nil)

	if err := f.svc.Join(ctx, connA, pA); err != nil {
		t.Fatalf("Join A: %v", err)
	}
	if err := f.svc.Join(ctx, connB, pB); err != nil {
		t.Fatalf("Join B: %v", err)
	}

	// Tick before deadline — no match expected.
	f.queueRepo.EXPECT().ListWaiting(gomock.Any(), "ranked").Return([]domain.Player{pA, pB}, nil)
	f.svc.Tick(ctx)

	if len(connA.Messages()) != 0 {
		t.Errorf("before deadline: connA got %d messages, want 0", len(connA.Messages()))
	}

	// Advance clock past the fallback deadline.
	f.clock.Advance(11 * time.Second)

	// Tick after deadline — fallback match should fire.
	f.queueRepo.EXPECT().ListWaiting(gomock.Any(), "ranked").Return([]domain.Player{pA, pB}, nil)
	f.queueRepo.EXPECT().Pair(gomock.Any(), pA, pB).Return(true, nil)

	f.svc.Tick(ctx)

	msgsA := connA.Messages()
	msgsB := connB.Messages()

	if len(msgsA) != 1 {
		t.Fatalf("after deadline: connA got %d messages, want 1", len(msgsA))
	}
	if len(msgsB) != 1 {
		t.Fatalf("after deadline: connB got %d messages, want 1", len(msgsB))
	}
	if msgsA[0].RoomID != msgsB[0].RoomID {
		t.Errorf("room_id mismatch: A=%s B=%s", msgsA[0].RoomID, msgsB[0].RoomID)
	}
}

func TestMatchmakingService_Tick_NoPairingWhenPairReturnsFalse(t *testing.T) {
	t.Parallel()

	f := newFixture(t, 3, 10*time.Second)
	ctx := context.Background()
	now := f.clock.Now()

	pA := domain.Player{UserID: 1, Mode: "ranked", Level: 5, EnqueuedAt: now}
	pB := domain.Player{UserID: 2, Mode: "ranked", Level: 6, EnqueuedAt: now}

	connA := newFakeConn(1)
	connB := newFakeConn(2)

	f.queueRepo.EXPECT().Enqueue(gomock.Any(), pA).Return(nil)
	f.queueRepo.EXPECT().Enqueue(gomock.Any(), pB).Return(nil)

	if err := f.svc.Join(ctx, connA, pA); err != nil {
		t.Fatalf("Join A: %v", err)
	}
	if err := f.svc.Join(ctx, connB, pB); err != nil {
		t.Fatalf("Join B: %v", err)
	}

	// Pair returns false → lost the race, no messages sent.
	// Both orderings are acceptable: the service iterates the slice and may try
	// (pA,pB) first or (pB,pA) first depending on which is the outer candidate.
	// All Pair calls return false so no match is emitted.
	f.queueRepo.EXPECT().ListWaiting(gomock.Any(), "ranked").Return([]domain.Player{pA, pB}, nil)
	f.queueRepo.EXPECT().Pair(gomock.Any(), gomock.Any(), gomock.Any()).Return(false, nil).AnyTimes()

	f.svc.Tick(ctx)

	if len(connA.Messages()) != 0 {
		t.Errorf("connA got messages despite race loss: %v", connA.Messages())
	}
	if len(connB.Messages()) != 0 {
		t.Errorf("connB got messages despite race loss: %v", connB.Messages())
	}
}

func TestMatchmakingService_Tick_NoDoubleMatch(t *testing.T) {
	t.Parallel()

	// Three players: A, B, C. A-B pair in-distance. C should not be double-matched.
	f := newFixture(t, 3, 10*time.Second)
	ctx := context.Background()
	now := f.clock.Now()

	pA := domain.Player{UserID: 1, Mode: "ranked", Level: 5, EnqueuedAt: now}
	pB := domain.Player{UserID: 2, Mode: "ranked", Level: 6, EnqueuedAt: now}
	pC := domain.Player{UserID: 3, Mode: "ranked", Level: 7, EnqueuedAt: now}

	connA := newFakeConn(1)
	connB := newFakeConn(2)
	connC := newFakeConn(3)

	f.queueRepo.EXPECT().Enqueue(gomock.Any(), pA).Return(nil)
	f.queueRepo.EXPECT().Enqueue(gomock.Any(), pB).Return(nil)
	f.queueRepo.EXPECT().Enqueue(gomock.Any(), pC).Return(nil)

	if err := f.svc.Join(ctx, connA, pA); err != nil {
		t.Fatalf("Join A: %v", err)
	}
	if err := f.svc.Join(ctx, connB, pB); err != nil {
		t.Fatalf("Join B: %v", err)
	}
	if err := f.svc.Join(ctx, connC, pC); err != nil {
		t.Fatalf("Join C: %v", err)
	}

	// A pairs with B (first match); C remains unmatched.
	f.queueRepo.EXPECT().ListWaiting(gomock.Any(), "ranked").Return([]domain.Player{pA, pB, pC}, nil)
	f.queueRepo.EXPECT().Pair(gomock.Any(), pA, pB).Return(true, nil)
	// No second Pair call for C — it has no partner.

	f.svc.Tick(ctx)

	if len(connA.Messages()) != 1 {
		t.Fatalf("connA: want 1 message, got %d", len(connA.Messages()))
	}
	if len(connB.Messages()) != 1 {
		t.Fatalf("connB: want 1 message, got %d", len(connB.Messages()))
	}
	if len(connC.Messages()) != 0 {
		t.Fatalf("connC: want 0 messages, got %d", len(connC.Messages()))
	}
}

func TestMatchmakingService_Leave_Disconnect_Cleanup(t *testing.T) {
	t.Parallel()

	f := newFixture(t, 3, 10*time.Second)
	ctx := context.Background()
	now := f.clock.Now()

	pA := domain.Player{UserID: 5, Mode: "ranked", Level: 5, EnqueuedAt: now}
	connA := newFakeConn(5)

	f.queueRepo.EXPECT().Enqueue(gomock.Any(), pA).Return(nil)
	if err := f.svc.Join(ctx, connA, pA); err != nil {
		t.Fatalf("Join: %v", err)
	}

	// Disconnect — Leave should clean up.
	f.queueRepo.EXPECT().Remove(gomock.Any(), "ranked", int64(5)).Return(true, nil)
	f.svc.Leave(ctx, 5, "ranked")

	// Next Tick — player 5 is no longer in the hub's mode map so no ListWaiting
	// call is expected (no active modes).
	f.svc.Tick(ctx) // no mock expectations → gomock catches unexpected calls
}

func TestMatchmakingService_PairedAway_NeverRematched(t *testing.T) {
	t.Parallel()

	f := newFixture(t, 3, 10*time.Second)
	ctx := context.Background()
	now := f.clock.Now()

	pA := domain.Player{UserID: 1, Mode: "ranked", Level: 5, EnqueuedAt: now}
	pB := domain.Player{UserID: 2, Mode: "ranked", Level: 6, EnqueuedAt: now}

	connA := newFakeConn(1)
	connB := newFakeConn(2)

	f.queueRepo.EXPECT().Enqueue(gomock.Any(), pA).Return(nil)
	f.queueRepo.EXPECT().Enqueue(gomock.Any(), pB).Return(nil)

	if err := f.svc.Join(ctx, connA, pA); err != nil {
		t.Fatalf("Join A: %v", err)
	}
	if err := f.svc.Join(ctx, connB, pB); err != nil {
		t.Fatalf("Join B: %v", err)
	}

	// First Tick — A and B are matched.
	f.queueRepo.EXPECT().ListWaiting(gomock.Any(), "ranked").Return([]domain.Player{pA, pB}, nil)
	f.queueRepo.EXPECT().Pair(gomock.Any(), pA, pB).Return(true, nil)
	f.svc.Tick(ctx)

	// Both deregistered — second Tick should have no active modes, no calls.
	f.svc.Tick(ctx) // no mock expectations

	if len(connA.Messages()) != 1 {
		t.Errorf("connA: want 1 message, got %d", len(connA.Messages()))
	}
	if len(connB.Messages()) != 1 {
		t.Errorf("connB: want 1 message, got %d", len(connB.Messages()))
	}
}

func TestMatchmakingService_Tick_SendError_Continues(t *testing.T) {
	t.Parallel()

	f := newFixture(t, 3, 10*time.Second)
	ctx := context.Background()
	now := f.clock.Now()

	pA := domain.Player{UserID: 1, Mode: "ranked", Level: 5, EnqueuedAt: now}
	pB := domain.Player{UserID: 2, Mode: "ranked", Level: 6, EnqueuedAt: now}

	connA := &errConn{fakeConn: fakeConn{userID: 1}}
	connB := newFakeConn(2)

	f.queueRepo.EXPECT().Enqueue(gomock.Any(), pA).Return(nil)
	f.queueRepo.EXPECT().Enqueue(gomock.Any(), pB).Return(nil)

	if err := f.svc.Join(ctx, connA, pA); err != nil {
		t.Fatalf("Join A: %v", err)
	}
	if err := f.svc.Join(ctx, connB, pB); err != nil {
		t.Fatalf("Join B: %v", err)
	}

	f.queueRepo.EXPECT().ListWaiting(gomock.Any(), "ranked").Return([]domain.Player{pA, pB}, nil)
	f.queueRepo.EXPECT().Pair(gomock.Any(), pA, pB).Return(true, nil)

	// Should not panic even if connA.Send returns an error.
	f.svc.Tick(ctx)
}

func TestMatchmakingService_Tick_ListError(t *testing.T) {
	t.Parallel()

	f := newFixture(t, 3, 10*time.Second)
	ctx := context.Background()
	now := f.clock.Now()

	pA := domain.Player{UserID: 1, Mode: "ranked", Level: 5, EnqueuedAt: now}
	connA := newFakeConn(1)

	f.queueRepo.EXPECT().Enqueue(gomock.Any(), pA).Return(nil)
	if err := f.svc.Join(ctx, connA, pA); err != nil {
		t.Fatalf("Join: %v", err)
	}

	f.queueRepo.EXPECT().ListWaiting(gomock.Any(), "ranked").Return(nil, fmt.Errorf("redis error"))

	// Should not panic.
	f.svc.Tick(ctx)
}

func TestMatchmakingService_Tick_MultipleModesIsolated(t *testing.T) {
	t.Parallel()

	f := newFixture(t, 3, 10*time.Second)
	ctx := context.Background()
	now := f.clock.Now()

	pRanked1 := domain.Player{UserID: 1, Mode: "ranked", Level: 5, EnqueuedAt: now}
	pRanked2 := domain.Player{UserID: 2, Mode: "ranked", Level: 6, EnqueuedAt: now}
	pCasual1 := domain.Player{UserID: 3, Mode: "casual", Level: 3, EnqueuedAt: now}
	pCasual2 := domain.Player{UserID: 4, Mode: "casual", Level: 4, EnqueuedAt: now}

	connR1 := newFakeConn(1)
	connR2 := newFakeConn(2)
	connC1 := newFakeConn(3)
	connC2 := newFakeConn(4)

	f.queueRepo.EXPECT().Enqueue(gomock.Any(), pRanked1).Return(nil)
	f.queueRepo.EXPECT().Enqueue(gomock.Any(), pRanked2).Return(nil)
	f.queueRepo.EXPECT().Enqueue(gomock.Any(), pCasual1).Return(nil)
	f.queueRepo.EXPECT().Enqueue(gomock.Any(), pCasual2).Return(nil)

	if err := f.svc.Join(ctx, connR1, pRanked1); err != nil {
		t.Fatalf("Join R1: %v", err)
	}
	if err := f.svc.Join(ctx, connR2, pRanked2); err != nil {
		t.Fatalf("Join R2: %v", err)
	}
	if err := f.svc.Join(ctx, connC1, pCasual1); err != nil {
		t.Fatalf("Join C1: %v", err)
	}
	if err := f.svc.Join(ctx, connC2, pCasual2); err != nil {
		t.Fatalf("Join C2: %v", err)
	}

	f.queueRepo.EXPECT().ListWaiting(gomock.Any(), "ranked").Return([]domain.Player{pRanked1, pRanked2}, nil)
	f.queueRepo.EXPECT().ListWaiting(gomock.Any(), "casual").Return([]domain.Player{pCasual1, pCasual2}, nil)
	f.queueRepo.EXPECT().Pair(gomock.Any(), pRanked1, pRanked2).Return(true, nil)
	f.queueRepo.EXPECT().Pair(gomock.Any(), pCasual1, pCasual2).Return(true, nil)

	f.svc.Tick(ctx)

	for _, c := range []*fakeConn{connR1, connR2, connC1, connC2} {
		if len(c.Messages()) != 1 {
			t.Errorf("conn %d: want 1 message, got %d", c.UserID(), len(c.Messages()))
		}
	}
}
