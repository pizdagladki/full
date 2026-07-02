package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"go.uber.org/mock/gomock"
	"go.uber.org/zap"

	"github.com/pizdagladki/full/services/matchmaking/internal/api/domain"
	rmocks "github.com/pizdagladki/full/services/matchmaking/internal/api/repository/mocks"
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

// fakeRatingsClient is a test double for RatingsClient that returns a
// preconfigured level or error. It records how many times GetLevel was called
// so tests can assert it was invoked.
type fakeRatingsClient struct {
	mu    sync.Mutex
	level int
	err   error
	calls int
}

func newFakeRatingsClient(level int, err error) *fakeRatingsClient {
	return &fakeRatingsClient{level: level, err: err}
}

func (f *fakeRatingsClient) GetLevel(_ context.Context, _ int64) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return f.level, f.err
}

func (f *fakeRatingsClient) CallCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

// fakeReportsClient is a test double for ReportsClient that returns a
// preconfigured CooldownStatus or error.
type fakeReportsClient struct {
	mu     sync.Mutex
	status CooldownStatus
	err    error
	calls  int
}

func newFakeReportsClient(status CooldownStatus, err error) *fakeReportsClient {
	return &fakeReportsClient{status: status, err: err}
}

// noCooldownReportsClient returns a no-op ReportsClient (no cooldown, no error).
func noCooldownReportsClient() *fakeReportsClient {
	return newFakeReportsClient(CooldownStatus{Active: false}, nil)
}

func (f *fakeReportsClient) GetCooldown(_ context.Context, _ int64) (CooldownStatus, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return f.status, f.err
}

func (f *fakeReportsClient) CallCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

// --- helpers ---

type serviceFixture struct {
	ctrl          *gomock.Controller
	queueRepo     *rmocks.MockQueueRepository
	ratingsClient *fakeRatingsClient
	reports       *fakeReportsClient
	clock         *fakeClock
	roomGen       *deterministicRoomIDGen
	svc           MatchmakingService
}

// newFixture builds a test fixture with a fakeRatingsClient that returns the
// given level (and no error) by default. Callers that need different rating
// behaviours should use newFixtureWithRatings.
func newFixture(t *testing.T, levelDist int, fallbackAfter time.Duration) *serviceFixture {
	t.Helper()
	return newFixtureWithRatings(t, levelDist, fallbackAfter, 5, nil)
}

// newFixtureWithRatings creates a fixture where the fake ratings client returns
// ratingsLevel (and ratingsErr) for every GetLevel call. The reports client
// defaults to no-cooldown (fail-open).
func newFixtureWithRatings(t *testing.T, levelDist int, fallbackAfter time.Duration, ratingsLevel int, ratingsErr error) *serviceFixture {
	t.Helper()

	ctrl := gomock.NewController(t)
	queueRepo := rmocks.NewMockQueueRepository(ctrl)
	rc := newFakeRatingsClient(ratingsLevel, ratingsErr)
	rpc := noCooldownReportsClient()
	clock := &fakeClock{now: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)}
	roomGen := &deterministicRoomIDGen{id: "room-1"}

	svc := NewMatchmakingService(
		zap.NewNop(),
		queueRepo,
		clock,
		roomGen,
		levelDist,
		fallbackAfter,
		rc,
		rpc,
	)

	return &serviceFixture{
		ctrl:          ctrl,
		queueRepo:     queueRepo,
		ratingsClient: rc,
		reports:       rpc,
		clock:         clock,
		roomGen:       roomGen,
		svc:           svc,
	}
}

// --- tests ---

func TestMatchmakingService_Join_Validation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		player       domain.Player
		ratingsLevel int
		ratingsErr   error
		wantErr      bool
	}{
		{
			// The ratings service returns error, so defaultLevel (4) is used;
			// but mode is empty so validation still fails.
			name:         "invalid empty mode",
			player:       domain.Player{UserID: 1, Mode: "", Level: 5},
			ratingsLevel: 5,
			ratingsErr:   nil,
			wantErr:      true,
		},
		{
			// Ratings returns level 11 — that is out of range.
			name:         "invalid level eleven from ratings",
			player:       domain.Player{UserID: 1, Mode: "ranked", Level: 5},
			ratingsLevel: 11,
			ratingsErr:   nil,
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixtureWithRatings(t, 3, 10*time.Second, tt.ratingsLevel, tt.ratingsErr)
			conn := newFakeConn(tt.player.UserID)
			err := f.svc.Join(context.Background(), conn, tt.player)

			if !tt.wantErr && err != nil {
				t.Fatalf("Join() unexpected error = %v", err)
			}
			if tt.wantErr && err == nil {
				t.Fatal("Join() error = nil, want validation error")
			}
		})
	}
}

func TestMatchmakingService_Join_Success(t *testing.T) {
	t.Parallel()

	f := newFixtureWithRatings(t, 3, 10*time.Second, 5, nil)
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

	f := newFixtureWithRatings(t, 3, 10*time.Second, 5, nil)
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

	f := newFixtureWithRatings(t, 3, 10*time.Second, 5, nil)
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

	// Both players at level 5; ratings returns 5 for both. diff=0 ≤ 3.
	f := newFixtureWithRatings(t, 3, 10*time.Second, 5, nil)
	ctx := context.Background()
	now := f.clock.Now()

	pA := domain.Player{UserID: 1, Mode: "ranked", Level: 5, EnqueuedAt: now}
	pB := domain.Player{UserID: 2, Mode: "ranked", Level: 5, EnqueuedAt: now}

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

	// We need two different players at different levels (1 and 6) so diff=5 > dist=1.
	// Use a per-call fake that returns level based on userID.
	ctrl := gomock.NewController(t)
	queueRepo := rmocks.NewMockQueueRepository(ctrl)
	clock := &fakeClock{now: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)}

	// We build a custom multi-level fake: returns level 1 for userID=1, level 6 for userID=2.
	rcMulti := &multiLevelRatingsClient{levels: map[int64]int{1: 1, 2: 6}}

	svc := NewMatchmakingService(
		zap.NewNop(),
		queueRepo,
		clock,
		&deterministicRoomIDGen{id: "room-1"},
		1, // dist=1 → pA(level=1) and pB(level=6) diff=5 won't match in-distance
		10*time.Second,
		rcMulti,
		noCooldownReportsClient(),
	)

	ctx := context.Background()
	now := clock.Now()

	pA := domain.Player{UserID: 1, Mode: "ranked", Level: 1, EnqueuedAt: now}
	pB := domain.Player{UserID: 2, Mode: "ranked", Level: 6, EnqueuedAt: now}

	connA := newFakeConn(1)
	connB := newFakeConn(2)

	queueRepo.EXPECT().Enqueue(gomock.Any(), pA).Return(nil)
	queueRepo.EXPECT().Enqueue(gomock.Any(), pB).Return(nil)

	if err := svc.Join(ctx, connA, pA); err != nil {
		t.Fatalf("Join A: %v", err)
	}
	if err := svc.Join(ctx, connB, pB); err != nil {
		t.Fatalf("Join B: %v", err)
	}

	// Tick before deadline — no match expected; both waiters remain so Refresh fires.
	queueRepo.EXPECT().ListWaiting(gomock.Any(), "ranked").Return([]domain.Player{pA, pB}, nil)
	queueRepo.EXPECT().Refresh(gomock.Any(), "ranked").Return(nil)
	svc.Tick(ctx)

	if len(connA.Messages()) != 0 {
		t.Errorf("before deadline: connA got %d messages, want 0", len(connA.Messages()))
	}

	// Advance clock past the fallback deadline.
	clock.Advance(11 * time.Second)

	// Tick after deadline — fallback match should fire.
	queueRepo.EXPECT().ListWaiting(gomock.Any(), "ranked").Return([]domain.Player{pA, pB}, nil)
	queueRepo.EXPECT().Pair(gomock.Any(), pA, pB).Return(true, nil)

	svc.Tick(ctx)

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

// multiLevelRatingsClient returns a per-userID level from a map.
type multiLevelRatingsClient struct {
	levels map[int64]int
}

func (m *multiLevelRatingsClient) GetLevel(_ context.Context, userID int64) (int, error) {
	if l, ok := m.levels[userID]; ok {
		return l, nil
	}
	return defaultLevel, nil
}

func TestMatchmakingService_Tick_NoPairingWhenPairReturnsFalse(t *testing.T) {
	t.Parallel()

	f := newFixtureWithRatings(t, 3, 10*time.Second, 5, nil)
	ctx := context.Background()
	now := f.clock.Now()

	pA := domain.Player{UserID: 1, Mode: "ranked", Level: 5, EnqueuedAt: now}
	pB := domain.Player{UserID: 2, Mode: "ranked", Level: 5, EnqueuedAt: now}

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
	f.queueRepo.EXPECT().ListWaiting(gomock.Any(), "ranked").Return([]domain.Player{pA, pB}, nil)
	f.queueRepo.EXPECT().Pair(gomock.Any(), gomock.Any(), gomock.Any()).Return(false, nil).AnyTimes()
	f.queueRepo.EXPECT().Refresh(gomock.Any(), "ranked").Return(nil)

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

	// Three players all at level 5 (ratings returns 5).
	f := newFixtureWithRatings(t, 3, 10*time.Second, 5, nil)
	ctx := context.Background()
	now := f.clock.Now()

	pA := domain.Player{UserID: 1, Mode: "ranked", Level: 5, EnqueuedAt: now}
	pB := domain.Player{UserID: 2, Mode: "ranked", Level: 5, EnqueuedAt: now}
	pC := domain.Player{UserID: 3, Mode: "ranked", Level: 5, EnqueuedAt: now}

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

	// A pairs with B (first match); C remains unmatched, so Refresh fires for C.
	f.queueRepo.EXPECT().ListWaiting(gomock.Any(), "ranked").Return([]domain.Player{pA, pB, pC}, nil)
	f.queueRepo.EXPECT().Pair(gomock.Any(), pA, pB).Return(true, nil)
	// No second Pair call for C — it has no partner.
	f.queueRepo.EXPECT().Refresh(gomock.Any(), "ranked").Return(nil)

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

	f := newFixtureWithRatings(t, 3, 10*time.Second, 5, nil)
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

	f := newFixtureWithRatings(t, 3, 10*time.Second, 5, nil)
	ctx := context.Background()
	now := f.clock.Now()

	pA := domain.Player{UserID: 1, Mode: "ranked", Level: 5, EnqueuedAt: now}
	pB := domain.Player{UserID: 2, Mode: "ranked", Level: 5, EnqueuedAt: now}

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

	f := newFixtureWithRatings(t, 3, 10*time.Second, 5, nil)
	ctx := context.Background()
	now := f.clock.Now()

	pA := domain.Player{UserID: 1, Mode: "ranked", Level: 5, EnqueuedAt: now}
	pB := domain.Player{UserID: 2, Mode: "ranked", Level: 5, EnqueuedAt: now}

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

	f := newFixtureWithRatings(t, 3, 10*time.Second, 5, nil)
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

	f := newFixtureWithRatings(t, 3, 10*time.Second, 5, nil)
	ctx := context.Background()
	now := f.clock.Now()

	pRanked1 := domain.Player{UserID: 1, Mode: "ranked", Level: 5, EnqueuedAt: now}
	pRanked2 := domain.Player{UserID: 2, Mode: "ranked", Level: 5, EnqueuedAt: now}
	pCasual1 := domain.Player{UserID: 3, Mode: "casual", Level: 5, EnqueuedAt: now}
	pCasual2 := domain.Player{UserID: 4, Mode: "casual", Level: 5, EnqueuedAt: now}

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
	// Both modes are fully drained → no Refresh expected.

	f.svc.Tick(ctx)

	for _, c := range []*fakeConn{connR1, connR2, connC1, connC2} {
		if len(c.Messages()) != 1 {
			t.Errorf("conn %d: want 1 message, got %d", c.UserID(), len(c.Messages()))
		}
	}
}

// TestMatchmakingService_Tick_Refresh_SoloWaiter asserts that when a mode has
// one lone player (no opponent found), Refresh is called to extend the hash TTL
// so the live waiter is never evicted by the crash-orphan backstop.
func TestMatchmakingService_Tick_Refresh_SoloWaiter(t *testing.T) {
	t.Parallel()

	f := newFixtureWithRatings(t, 3, 10*time.Second, 5, nil)
	ctx := context.Background()
	now := f.clock.Now()

	pA := domain.Player{UserID: 1, Mode: "ranked", Level: 5, EnqueuedAt: now}
	connA := newFakeConn(1)

	f.queueRepo.EXPECT().Enqueue(gomock.Any(), pA).Return(nil)
	if err := f.svc.Join(ctx, connA, pA); err != nil {
		t.Fatalf("Join: %v", err)
	}

	// One player, no opponents → no Pair call. Refresh must be called.
	f.queueRepo.EXPECT().ListWaiting(gomock.Any(), "ranked").Return([]domain.Player{pA}, nil)
	f.queueRepo.EXPECT().Refresh(gomock.Any(), "ranked").Return(nil)

	f.svc.Tick(ctx)

	// No match message was sent.
	if len(connA.Messages()) != 0 {
		t.Errorf("solo waiter: got %d messages, want 0", len(connA.Messages()))
	}
}

// TestMatchmakingService_Tick_NoRefresh_FullyDrained asserts that when all
// players in a mode are paired away, Refresh is NOT called (the key is already
// gone or the mode is no longer live).
func TestMatchmakingService_Tick_NoRefresh_FullyDrained(t *testing.T) {
	t.Parallel()

	f := newFixtureWithRatings(t, 3, 10*time.Second, 5, nil)
	ctx := context.Background()
	now := f.clock.Now()

	pA := domain.Player{UserID: 1, Mode: "ranked", Level: 5, EnqueuedAt: now}
	pB := domain.Player{UserID: 2, Mode: "ranked", Level: 5, EnqueuedAt: now}

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

	// Both paired away → no Refresh expected.
	f.queueRepo.EXPECT().ListWaiting(gomock.Any(), "ranked").Return([]domain.Player{pA, pB}, nil)
	f.queueRepo.EXPECT().Pair(gomock.Any(), pA, pB).Return(true, nil)
	// gomock will fail the test if Refresh is called unexpectedly.

	f.svc.Tick(ctx)

	if len(connA.Messages()) != 1 {
		t.Fatalf("connA: want 1 message, got %d", len(connA.Messages()))
	}
	if len(connB.Messages()) != 1 {
		t.Fatalf("connB: want 1 message, got %d", len(connB.Messages()))
	}
}

// TestMatchmakingService_Tick_Refresh_Error asserts that a Refresh error is
// logged but does not abort the tick or panic.
func TestMatchmakingService_Tick_Refresh_Error(t *testing.T) {
	t.Parallel()

	f := newFixtureWithRatings(t, 3, 10*time.Second, 5, nil)
	ctx := context.Background()
	now := f.clock.Now()

	pA := domain.Player{UserID: 1, Mode: "ranked", Level: 5, EnqueuedAt: now}
	connA := newFakeConn(1)

	f.queueRepo.EXPECT().Enqueue(gomock.Any(), pA).Return(nil)
	if err := f.svc.Join(ctx, connA, pA); err != nil {
		t.Fatalf("Join: %v", err)
	}

	f.queueRepo.EXPECT().ListWaiting(gomock.Any(), "ranked").Return([]domain.Player{pA}, nil)
	f.queueRepo.EXPECT().Refresh(gomock.Any(), "ranked").Return(fmt.Errorf("redis timeout"))

	// Must not panic.
	f.svc.Tick(ctx)
}

// TestMatchmakingService_Join_AuthoritativeLevel covers criteria 1, 2, and 5:
// authoritative level from ratings is used, and failures fall back to defaultLevel.
func TestMatchmakingService_Join_AuthoritativeLevel(t *testing.T) {
	t.Parallel()

	const playerUserID = int64(42)

	tests := []struct {
		name         string
		clientLevel  int   // level the client sent in the join message
		ratingsLevel int   // level returned by ratings service
		ratingsErr   error // error returned by ratings service (nil = success)
		wantEnqLevel int   // expected level the player is enqueued at
		wantJoinErr  bool  // whether Join itself should return an error
	}{
		{
			// criterion: 1 — authoritative level is used over client-supplied value
			name:         "authoritative level overrides client",
			clientLevel:  3,
			ratingsLevel: 8,
			ratingsErr:   nil,
			wantEnqLevel: 8,
		},
		{
			// criterion: 2 — transport/non-2xx error → default level L4, no error from Join
			name:         "fallback to default on error",
			clientLevel:  7,
			ratingsLevel: 0,
			ratingsErr:   fmt.Errorf("connection refused"),
			wantEnqLevel: defaultLevel,
			wantJoinErr:  false,
		},
		{
			// criterion: 2 — 404 (unseen user) → default level L4, no error from Join
			name:         "unseen user gets default",
			clientLevel:  5,
			ratingsLevel: 0,
			ratingsErr:   fmt.Errorf("ratings service returned 404"),
			wantEnqLevel: defaultLevel,
			wantJoinErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			queueRepo := rmocks.NewMockQueueRepository(ctrl)
			rc := newFakeRatingsClient(tt.ratingsLevel, tt.ratingsErr)
			clock := &fakeClock{now: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)}

			svc := NewMatchmakingService(
				zap.NewNop(),
				queueRepo,
				clock,
				&deterministicRoomIDGen{id: "room-1"},
				3,
				10*time.Second,
				rc,
				noCooldownReportsClient(),
			)

			player := domain.Player{
				UserID:     playerUserID,
				Mode:       "ranked",
				Level:      tt.clientLevel,
				EnqueuedAt: clock.Now(),
			}
			conn := newFakeConn(playerUserID)

			// Enqueue is called with the authoritative level, not the client level.
			queueRepo.EXPECT().
				Enqueue(gomock.Any(), gomock.AssignableToTypeOf(domain.Player{})).
				DoAndReturn(func(_ context.Context, p domain.Player) error {
					if p.Level != tt.wantEnqLevel {
						t.Errorf("Enqueue called with Level=%d, want %d", p.Level, tt.wantEnqLevel)
					}
					return nil
				})

			err := svc.Join(context.Background(), conn, player)
			if tt.wantJoinErr && err == nil {
				t.Fatal("Join() error = nil, want error")
			}
			if !tt.wantJoinErr && err != nil {
				t.Fatalf("Join() unexpected error = %v", err)
			}

			// Verify GetLevel was actually called (ratings client was consulted).
			if rc.CallCount() != 1 {
				t.Errorf("GetLevel called %d times, want 1", rc.CallCount())
			}
		})
	}
}

// TestMatchmakingService_Join_Cooldown covers the three reports-cooldown
// acceptance criteria for the Join path.
func TestMatchmakingService_Join_Cooldown(t *testing.T) {
	t.Parallel()

	const playerUserID = int64(99)

	tests := []struct {
		name             string
		cooldownStatus   CooldownStatus
		cooldownErr      error
		wantEnqueueCalls int  // 0 = Enqueue must NOT be called; 1 = must be called once
		wantCooldownErr  bool // true → Join must return *domain.CooldownError
		wantSecondsRem   int  // only checked when wantCooldownErr == true
		// criterion tag for the auditor
		criterion string
	}{
		{
			// criterion: 1 — active cooldown → player NOT enqueued, CooldownError returned
			name:             "active-cooldown-rejected",
			cooldownStatus:   CooldownStatus{Active: true, SecondsRemaining: 1800},
			cooldownErr:      nil,
			wantEnqueueCalls: 0,
			wantCooldownErr:  true,
			wantSecondsRem:   1800,
			criterion:        "1",
		},
		{
			// criterion: 2 — no cooldown → player enqueued normally
			name:             "no-cooldown-enqueued",
			cooldownStatus:   CooldownStatus{Active: false},
			cooldownErr:      nil,
			wantEnqueueCalls: 1,
			wantCooldownErr:  false,
			criterion:        "2",
		},
		{
			// criterion: 3 — lookup error → fail-open (join allowed, Enqueue called)
			name:             "lookup-error-fail-open",
			cooldownStatus:   CooldownStatus{},
			cooldownErr:      fmt.Errorf("transport error"),
			wantEnqueueCalls: 1,
			wantCooldownErr:  false,
			criterion:        "3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			queueRepo := rmocks.NewMockQueueRepository(ctrl)
			rc := newFakeRatingsClient(5, nil)
			rpc := newFakeReportsClient(tt.cooldownStatus, tt.cooldownErr)
			clock := &fakeClock{now: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)}

			svc := NewMatchmakingService(
				zap.NewNop(),
				queueRepo,
				clock,
				&deterministicRoomIDGen{id: "room-1"},
				3,
				10*time.Second,
				rc,
				rpc,
			)

			player := domain.Player{
				UserID:     playerUserID,
				Mode:       "ranked",
				Level:      5,
				EnqueuedAt: clock.Now(),
			}
			conn := newFakeConn(playerUserID)

			if tt.wantEnqueueCalls > 0 {
				queueRepo.EXPECT().
					Enqueue(gomock.Any(), gomock.AssignableToTypeOf(domain.Player{})).
					Return(nil).
					Times(tt.wantEnqueueCalls)
			}
			// When wantEnqueueCalls == 0, gomock will fail the test if Enqueue
			// is called unexpectedly — that's the load-bearing check for criterion 1.

			err := svc.Join(context.Background(), conn, player)

			if tt.wantCooldownErr {
				// criterion: 1 — must return *domain.CooldownError
				var cooldownErr *domain.CooldownError
				if !errors.As(err, &cooldownErr) {
					t.Fatalf("Join() error = %v, want *domain.CooldownError", err)
				}
				if cooldownErr.SecondsRemaining != tt.wantSecondsRem {
					t.Errorf("SecondsRemaining = %d, want %d", cooldownErr.SecondsRemaining, tt.wantSecondsRem)
				}
			} else {
				if err != nil {
					t.Fatalf("Join() unexpected error = %v", err)
				}
			}

			// Verify GetCooldown was consulted in all cases.
			if rpc.CallCount() != 1 {
				t.Errorf("GetCooldown called %d times, want 1", rpc.CallCount())
			}
		})
	}
}
