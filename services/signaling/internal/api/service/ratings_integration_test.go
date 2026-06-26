package service

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/mock/gomock"
	"go.uber.org/zap"

	"github.com/pizdagladki/full/services/signaling/internal/api/domain"
	"github.com/pizdagladki/full/services/signaling/internal/api/repository"
	repomocks "github.com/pizdagladki/full/services/signaling/internal/api/repository/mocks"
)

// spyRatingsClient captures ApplyResult calls for assertions in tests.
type spyRatingsClient struct {
	mu      sync.Mutex
	calls   []ApplyResultRequest
	callCnt atomic.Int32
	err     error // returned on every call if set
}

func (m *spyRatingsClient) ApplyResult(_ context.Context, req ApplyResultRequest) error {
	m.mu.Lock()
	m.calls = append(m.calls, req)
	m.mu.Unlock()
	m.callCnt.Add(1)
	return m.err
}

func (m *spyRatingsClient) count() int {
	return int(m.callCnt.Load())
}

func (m *spyRatingsClient) snapshot() []ApplyResultRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]ApplyResultRequest, len(m.calls))
	copy(out, m.calls)
	return out
}

// newRatingsFixture creates a signalingService fixture with a spy ratings client.
// The times slice controls the injectable clock (same semantics as newArbFixture).
func newRatingsFixture(t *testing.T, spy *spyRatingsClient, times []time.Time, confirmBuf time.Duration) (*signalingService, *capturedTimer) {
	t.Helper()

	ctrl := gomock.NewController(t)
	roomRepo := repomocks.NewMockRoomRepository(ctrl)
	roomRepo.EXPECT().
		Join(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(repository.JoinResultJoined, nil).
		AnyTimes()
	roomRepo.EXPECT().
		RemoveRoom(gomock.Any(), gomock.Any()).
		Return(nil).
		AnyTimes()

	idx := 0
	nowFunc := func() time.Time {
		if idx >= len(times) {
			return times[len(times)-1]
		}
		v := times[idx]
		idx++
		return v
	}

	ct := &capturedTimer{}
	af := fakeAfterFunc(ct)

	svc := NewSignalingService(
		zap.NewNop(),
		roomRepo,
		nowFunc,
		af,
		confirmBuf,
		spy,
	).(*signalingService)

	return svc, ct
}

// waitForRatingsCall spins until the spy has received at least n calls or the
// timeout elapses. Returns the number of calls seen.
func waitForRatingsCall(spy *spyRatingsClient, n int, timeout time.Duration) int {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if spy.count() >= n {
			return spy.count()
		}
		time.Sleep(2 * time.Millisecond)
	}
	return spy.count()
}

// TestRatingsClient_RankedRoomCallsApplyResultOnce verifies that for a ranked
// room, ApplyResult is called exactly once after both peers report a blink, with
// correct winner_id, loser_id, mode="ranked", and positive duration_ms.
//
// criterion: 1 — ranked room fires ApplyResult exactly once with correct payload.
func TestRatingsClient_RankedRoomCallsApplyResultOnce(t *testing.T) {
	t.Parallel()

	t0 := time.Unix(1000, 0)
	t1 := t0.Add(5 * time.Millisecond)  // A blinks at t0 (earlier => A loses)
	t2 := t0.Add(10 * time.Millisecond) // B blinks at t1 (later => B wins)
	// battleStart is captured when the room becomes full on the second join call.
	// nowFunc call order: join1, join2 (battleStart), ReportEvent(A), ReportEvent(B), fireRatingsCall now()
	// We need times for: battleStart (t0), A-report (t1), B-report (t2), ratings-now (t2+1ms)
	times := []time.Time{
		t0,                           // battleStart (2nd join triggers battleStart)
		t1,                           // A reports blink
		t2,                           // B reports blink (decides)
		t2.Add(1 * time.Millisecond), // fireRatingsCall s.now() for durationMs
	}

	spy := &spyRatingsClient{}
	svc, _ := newRatingsFixture(t, spy, times, 150*time.Millisecond)

	connA := newFakeConn(1)
	connB := newFakeConn(2)
	ctx := context.Background()

	// A joins first — no battleStart yet (only 1 member).
	if err := svc.Join(ctx, connA, "room-ranked", domain.ModeRanked); err != nil {
		t.Fatalf("A Join error = %v", err)
	}
	// B joins second — room is now full, battleStart is recorded.
	if err := svc.Join(ctx, connB, "room-ranked", domain.ModeRanked); err != nil {
		t.Fatalf("B Join error = %v", err)
	}

	// A reports blink (earlier timestamp => A is loser).
	if err := svc.ReportEvent(ctx, connA, "room-ranked", domain.TypeBlink); err != nil {
		t.Fatalf("A ReportEvent error = %v", err)
	}

	// B reports blink (later timestamp => B is winner, decides the match).
	if err := svc.ReportEvent(ctx, connB, "room-ranked", domain.TypeBlink); err != nil {
		t.Fatalf("B ReportEvent error = %v", err)
	}

	// Wait for the async goroutine.
	got := waitForRatingsCall(spy, 1, 200*time.Millisecond)
	if got != 1 {
		// criterion: 1 — fails if ApplyResult not called exactly once for ranked room
		t.Fatalf("ApplyResult called %d times, want 1 (criterion: 1 — ranked room must call ratings once)", got)
	}

	calls := spy.snapshot()
	req := calls[0]

	if req.WinnerID != 2 {
		t.Errorf("winner_id = %d, want 2 (criterion: 1 — correct winner_id)", req.WinnerID)
	}
	if req.LoserID != 1 {
		t.Errorf("loser_id = %d, want 1 (criterion: 1 — correct loser_id)", req.LoserID)
	}
	if req.Mode != domain.ModeRanked {
		t.Errorf("mode = %q, want %q (criterion: 1 — mode must be ranked)", req.Mode, domain.ModeRanked)
	}
	if req.DurationMs <= 0 {
		t.Errorf("duration_ms = %d, want positive (criterion: 1 — positive duration)", req.DurationMs)
	}
}

// TestRatingsClient_UnrankedRoomNoCall verifies that for an unranked room,
// ApplyResult is never called even after the match is decided.
//
// criterion: 2 — unranked room never fires ApplyResult.
func TestRatingsClient_UnrankedRoomNoCall(t *testing.T) {
	t.Parallel()

	t0 := time.Unix(2000, 0)
	t1 := t0.Add(5 * time.Millisecond)
	t2 := t0.Add(10 * time.Millisecond)
	times := []time.Time{t0, t1, t2, t2}

	spy := &spyRatingsClient{}
	svc, _ := newRatingsFixture(t, spy, times, 150*time.Millisecond)

	connA := newFakeConn(1)
	connB := newFakeConn(2)
	ctx := context.Background()

	if err := svc.Join(ctx, connA, "room-unranked", domain.ModeUnranked); err != nil {
		t.Fatalf("A Join error = %v", err)
	}
	if err := svc.Join(ctx, connB, "room-unranked", domain.ModeUnranked); err != nil {
		t.Fatalf("B Join error = %v", err)
	}

	if err := svc.ReportEvent(ctx, connA, "room-unranked", domain.TypeBlink); err != nil {
		t.Fatalf("A ReportEvent error = %v", err)
	}
	if err := svc.ReportEvent(ctx, connB, "room-unranked", domain.TypeBlink); err != nil {
		t.Fatalf("B ReportEvent error = %v", err)
	}

	// Give the goroutine time to run (it won't, but we need to be sure).
	time.Sleep(30 * time.Millisecond)

	// criterion: 2 — fails if ApplyResult is called for an unranked room
	if n := spy.count(); n != 0 {
		t.Errorf("ApplyResult called %d times for unranked room, want 0 (criterion: 2 — unranked must not call ratings)", n)
	}

	// Also verify the outcome WAS sent to peers (match still decided).
	if len(connA.Sent()) == 0 || len(connB.Sent()) == 0 {
		t.Error("outcome frame not sent to peers for unranked room")
	}
}

// TestRatingsClient_DuplicateOutcomeNotRecalled verifies that ApplyResult is
// called exactly once even if ReportEvent is called a third time after the match
// is already decided.
//
// criterion: 3 — idempotency: ApplyResult called at most once per match.
func TestRatingsClient_DuplicateOutcomeNotRecalled(t *testing.T) {
	t.Parallel()

	t0 := time.Unix(3000, 0)
	t1 := t0.Add(5 * time.Millisecond)
	t2 := t0.Add(10 * time.Millisecond)
	t3 := t0.Add(15 * time.Millisecond)
	times := []time.Time{t0, t1, t2, t3, t3}

	spy := &spyRatingsClient{}
	svc, _ := newRatingsFixture(t, spy, times, 150*time.Millisecond)

	connA := newFakeConn(1)
	connB := newFakeConn(2)
	ctx := context.Background()

	if err := svc.Join(ctx, connA, "room-idem2", domain.ModeRanked); err != nil {
		t.Fatalf("A Join error = %v", err)
	}
	if err := svc.Join(ctx, connB, "room-idem2", domain.ModeRanked); err != nil {
		t.Fatalf("B Join error = %v", err)
	}

	// Both report — outcome decided.
	if err := svc.ReportEvent(ctx, connA, "room-idem2", domain.TypeBlink); err != nil {
		t.Fatalf("A ReportEvent error = %v", err)
	}
	if err := svc.ReportEvent(ctx, connB, "room-idem2", domain.TypeBlink); err != nil {
		t.Fatalf("B ReportEvent error = %v", err)
	}

	// Wait for first ratings call.
	waitForRatingsCall(spy, 1, 200*time.Millisecond)

	// Third ReportEvent — must return ErrMatchFinished, not re-call ratings.
	err := svc.ReportEvent(ctx, connA, "room-idem2", domain.TypeBlink)
	if !errors.Is(err, domain.ErrMatchFinished) {
		t.Errorf("third ReportEvent error = %v, want ErrMatchFinished", err)
	}

	// Give extra time and confirm count stays at 1.
	time.Sleep(30 * time.Millisecond)

	// criterion: 3 — fails if ApplyResult called more than once
	if n := spy.count(); n != 1 {
		t.Errorf("ApplyResult called %d times, want exactly 1 (criterion: 3 — idempotent: must not re-call ratings)", n)
	}
}

// TestRatingsClient_ClientErrorDoesNotBlockOutcome verifies that when the
// ratings client returns an error, the outcome frame is still delivered to both
// peers (fire-and-forget: errors do not block the match outcome).
//
// criterion: 4 — ratings client error does not block outcome delivery.
func TestRatingsClient_ClientErrorDoesNotBlockOutcome(t *testing.T) {
	t.Parallel()

	t0 := time.Unix(4000, 0)
	t1 := t0.Add(5 * time.Millisecond)
	t2 := t0.Add(10 * time.Millisecond)
	times := []time.Time{t0, t1, t2, t2}

	spy := &spyRatingsClient{
		err: errors.New("ratings timeout"),
	}
	svc, _ := newRatingsFixture(t, spy, times, 150*time.Millisecond)

	connA := newFakeConn(1)
	connB := newFakeConn(2)
	ctx := context.Background()

	if err := svc.Join(ctx, connA, "room-err2", domain.ModeRanked); err != nil {
		t.Fatalf("A Join error = %v", err)
	}
	if err := svc.Join(ctx, connB, "room-err2", domain.ModeRanked); err != nil {
		t.Fatalf("B Join error = %v", err)
	}

	if err := svc.ReportEvent(ctx, connA, "room-err2", domain.TypeBlink); err != nil {
		t.Fatalf("A ReportEvent error = %v", err)
	}
	if err := svc.ReportEvent(ctx, connB, "room-err2", domain.TypeBlink); err != nil {
		t.Fatalf("B ReportEvent error = %v", err)
	}

	// Outcome must be sent to both peers despite the ratings error.
	// criterion: 4 — fails if outcome not sent when ratings client errors
	if len(connA.Sent()) == 0 {
		t.Error("connA did not receive outcome frame — ratings error must not block outcome (criterion: 4)")
	}
	if len(connB.Sent()) == 0 {
		t.Error("connB did not receive outcome frame — ratings error must not block outcome (criterion: 4)")
	}

	// The ratings call was still attempted (fire-and-forget with error).
	waitForRatingsCall(spy, 1, 200*time.Millisecond)
	if n := spy.count(); n != 1 {
		t.Errorf("ApplyResult called %d times, want 1 (client error does not prevent the call)", n)
	}
}
