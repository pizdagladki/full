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
	repomocks "github.com/pizdagladki/full/services/signaling/internal/api/repository/mocks"
)

// --- fake-clock fixture helpers ---

// capturedTimer holds the callback registered with the fake afterFunc so tests
// can fire the timer manually.
type capturedTimer struct {
	mu       sync.Mutex
	callback func()
	fired    bool
}

func (ct *capturedTimer) fire() {
	ct.mu.Lock()
	cb := ct.callback
	ct.fired = true
	ct.mu.Unlock()

	if cb != nil {
		cb()
	}
}

// fakeAfterFunc captures the callback and returns a no-op *time.Timer.
// The test calls ct.fire() to trigger the timeout manually.
func fakeAfterFunc(ct *capturedTimer) func(time.Duration, func()) *time.Timer {
	return func(_ time.Duration, fn func()) *time.Timer {
		ct.mu.Lock()
		ct.callback = fn
		ct.mu.Unlock()
		// Return a stopped timer — we drive it manually.
		t := time.NewTimer(0)
		t.Stop()

		return t
	}
}

// arbFixture builds a signalingService with injectable clock for arbitration tests.
type arbFixture struct {
	ctrl     *gomock.Controller
	roomRepo *repomocks.MockRoomRepository
	svc      *signalingService
	now      func() time.Time
	ct       *capturedTimer
}

// newArbFixture creates a fixture where now() returns successive values from
// the provided slice (one per call in order). If the slice is exhausted the last
// value is repeated. afterFunc is wired to ct so the timer can be fired manually.
func newArbFixture(t *testing.T, times []time.Time, confirmBuf time.Duration) *arbFixture {
	t.Helper()

	ctrl := gomock.NewController(t)
	roomRepo := repomocks.NewMockRoomRepository(ctrl)

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

	roomCodeRepo := repomocks.NewMockRoomCodeRepository(ctrl)
	svc := NewSignalingService(
		zap.NewNop(), roomRepo, nowFunc, af, confirmBuf, &nopRatingsClient{},
		roomCodeRepo, newTestRoomIDGen(),
	).(*signalingService)

	return &arbFixture{ctrl: ctrl, roomRepo: roomRepo, svc: svc, ct: ct}
}

// seedRoom pre-populates the in-process hub with connA and connB already joined.
func (f *arbFixture) seedRoom(roomID string, connA, connB *fakeConn) {
	f.svc.mu.Lock()
	f.svc.rooms[roomID] = map[int64]Conn{connA.UserID(): connA, connB.UserID(): connB}
	f.svc.mu.Unlock()
}

// parseOutcome decodes an outcome frame and returns (winnerID, loserID).
func parseOutcome(t *testing.T, b []byte) (int64, int64) {
	t.Helper()

	var msg struct {
		Type     string `json:"type"`
		WinnerID int64  `json:"winner_id"`
		LoserID  int64  `json:"loser_id"`
	}
	if err := json.Unmarshal(b, &msg); err != nil {
		t.Fatalf("parseOutcome: invalid JSON %q: %v", b, err)
	}

	if msg.Type != domain.TypeOutcome {
		t.Fatalf("parseOutcome: unexpected type %q", msg.Type)
	}

	return msg.WinnerID, msg.LoserID
}

// --- Table-driven arbitration tests ---

func TestArbitration_EarlierTimestampLoses(t *testing.T) {
	t.Parallel()

	// criterion: 1 — server stamps each event with its own receive time;
	// criterion: 2 — member with the earlier server timestamp is the loser.

	t0 := time.Unix(0, 0)
	t1 := t0.Add(10 * time.Millisecond)

	tests := []struct {
		name         string
		times        []time.Time // nowFunc returns these in order
		wantWinnerID int64
		wantLoserID  int64
	}{
		{
			// criterion: 2 — fails if A (earlier) is not the loser
			name:         "earlier-timestamp-loses: A at t0, B at t1 => A loses",
			times:        []time.Time{t0, t1},
			wantWinnerID: 2,
			wantLoserID:  1,
		},
		{
			// criterion: 2 — fails if B (earlier) is not the loser
			name:         "earlier-timestamp-loses: B at t0, A at t1 => B loses",
			times:        []time.Time{t0, t1},
			wantWinnerID: 1,
			wantLoserID:  2,
		},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newArbFixture(t, tt.times, 150*time.Millisecond)
			connA := newFakeConn(1)
			connB := newFakeConn(2)
			f.seedRoom("room-arb", connA, connB)

			// Determine which conn reports first based on test index.
			var first, second *fakeConn
			if i == 0 {
				first, second = connA, connB
			} else {
				first, second = connB, connA
			}

			err := f.svc.ReportEvent(context.Background(), first, "room-arb", domain.TypeBlink)
			if err != nil {
				t.Fatalf("first ReportEvent error = %v", err)
			}

			err = f.svc.ReportEvent(context.Background(), second, "room-arb", domain.TypeBlink)
			if err != nil {
				t.Fatalf("second ReportEvent error = %v", err)
			}

			// Both peers must receive exactly one outcome frame.
			for _, c := range []*fakeConn{connA, connB} {
				frames := c.Sent()
				if len(frames) != 1 {
					t.Fatalf("conn %d received %d frames, want 1", c.UserID(), len(frames)) // criterion: 2
				}

				w, l := parseOutcome(t, frames[0])
				if w != tt.wantWinnerID || l != tt.wantLoserID {
					t.Errorf("conn %d: got winner=%d loser=%d, want winner=%d loser=%d", // criterion: 2
						c.UserID(), w, l, tt.wantWinnerID, tt.wantLoserID)
				}
			}
		})
	}
}

func TestArbitration_SimultaneousWithinBufferTiebreak(t *testing.T) {
	t.Parallel()

	// criterion: 2 — when both reports arrive at identical timestamps the
	// tiebreak is deterministic (first reporter is the loser — same time is
	// NOT Before, so the else branch picks firstReport as loser).

	sameTime := time.Unix(0, 0)

	tests := []struct {
		name         string
		wantLoserID  int64
		wantWinnerID int64
	}{
		{
			// criterion: 2 — fails if simultaneous tiebreak is inconsistent
			name:         "simultaneous-within-buffer tiebreak: first reporter is loser",
			wantLoserID:  1, // A reports first; same timestamp => A is loser
			wantWinnerID: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Both calls to now() return sameTime.
			f := newArbFixture(t, []time.Time{sameTime, sameTime}, 150*time.Millisecond)
			connA := newFakeConn(1)
			connB := newFakeConn(2)
			f.seedRoom("room-tie", connA, connB)

			if err := f.svc.ReportEvent(context.Background(), connA, "room-tie", domain.TypeBlink); err != nil {
				t.Fatalf("first ReportEvent error = %v", err)
			}

			if err := f.svc.ReportEvent(context.Background(), connB, "room-tie", domain.TypeBlink); err != nil {
				t.Fatalf("second ReportEvent error = %v", err)
			}

			for _, c := range []*fakeConn{connA, connB} {
				frames := c.Sent()
				if len(frames) != 1 {
					t.Fatalf("conn %d received %d frames, want 1", c.UserID(), len(frames)) // criterion: 2
				}

				w, l := parseOutcome(t, frames[0])
				if w != tt.wantWinnerID || l != tt.wantLoserID {
					t.Errorf("conn %d: got winner=%d loser=%d, want winner=%d loser=%d", // criterion: 2
						c.UserID(), w, l, tt.wantWinnerID, tt.wantLoserID)
				}
			}
		})
	}
}

func TestArbitration_IdempotentOutcome(t *testing.T) {
	t.Parallel()

	// criterion: 2 — re-sends after a decided outcome are ignored (ErrMatchFinished).

	t0 := time.Unix(0, 0)
	t1 := t0.Add(10 * time.Millisecond)
	t2 := t0.Add(20 * time.Millisecond)

	tests := []struct {
		name string
	}{
		{
			// criterion: 2 — fails if third report does not return ErrMatchFinished
			name: "idempotent: third report after decided outcome returns ErrMatchFinished",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newArbFixture(t, []time.Time{t0, t1, t2}, 150*time.Millisecond)
			connA := newFakeConn(1)
			connB := newFakeConn(2)
			f.seedRoom("room-idem", connA, connB)

			if err := f.svc.ReportEvent(context.Background(), connA, "room-idem", domain.TypeBlink); err != nil {
				t.Fatalf("first ReportEvent error = %v", err)
			}

			if err := f.svc.ReportEvent(context.Background(), connB, "room-idem", domain.TypeBlink); err != nil {
				t.Fatalf("second ReportEvent error = %v", err)
			}

			// Third report (from A again) after outcome decided.
			err := f.svc.ReportEvent(context.Background(), connA, "room-idem", domain.TypeBlink)
			if !errors.Is(err, domain.ErrMatchFinished) {
				t.Errorf("third ReportEvent error = %v, want ErrMatchFinished", err) // criterion: 2
			}

			// connA received exactly 1 outcome frame (not a second one).
			frames := connA.Sent()
			if len(frames) != 1 {
				t.Errorf("connA received %d frames after idempotent report, want 1", len(frames)) // criterion: 2
			}
		})
	}
}

func TestArbitration_NonMemberRejected(t *testing.T) {
	t.Parallel()

	// criterion: 4 — only the two room members can report; non-member is rejected.

	tests := []struct {
		name       string
		strangerID int64
	}{
		{
			// criterion: 4 — fails if non-member can submit a blink report
			name:       "non-member rejection: stranger cannot report blink",
			strangerID: 99,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newArbFixture(t, []time.Time{time.Now()}, 150*time.Millisecond)
			connA := newFakeConn(1)
			connB := newFakeConn(2)
			stranger := newFakeConn(tt.strangerID)
			f.seedRoom("room-nm", connA, connB)

			err := f.svc.ReportEvent(context.Background(), stranger, "room-nm", domain.TypeBlink)
			if !errors.Is(err, domain.ErrNotMember) {
				t.Errorf("ReportEvent from stranger: error = %v, want ErrNotMember", err) // criterion: 4
			}

			// No outcome sent to members.
			if n := len(connA.Sent()); n != 0 {
				t.Errorf("connA received %d frames from non-member report, want 0", n) // criterion: 4
			}

			if n := len(connB.Sent()); n != 0 {
				t.Errorf("connB received %d frames from non-member report, want 0", n) // criterion: 4
			}
		})
	}
}

func TestArbitration_ForfeitOnDisconnect(t *testing.T) {
	t.Parallel()

	// criterion: 5 — on disconnect/forfeit, the remaining peer is declared winner.

	tests := []struct {
		name string
	}{
		{
			// criterion: 5 — fails if leaving peer is not made loser on disconnect
			name: "forfeit-on-disconnect: leaving peer A is loser, B is winner",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newArbFixture(t, []time.Time{time.Now()}, 150*time.Millisecond)
			connA := newFakeConn(1)
			connB := newFakeConn(2)
			f.seedRoom("room-forfeit", connA, connB)

			f.roomRepo.EXPECT().RemoveRoom(gomock.Any(), "room-forfeit").Return(nil)

			// A disconnects.
			f.svc.Leave(context.Background(), connA, "room-forfeit")

			// B must have received an outcome before peer_left.
			sentB := connB.Sent()
			if len(sentB) < 2 {
				t.Fatalf("connB received %d frames, want at least 2 (outcome + peer_left)", len(sentB)) // criterion: 5
			}

			w, l := parseOutcome(t, sentB[0])
			if w != 2 || l != 1 {
				t.Errorf("forfeit outcome: got winner=%d loser=%d, want winner=2 loser=1", w, l) // criterion: 5
			}

			// Second frame must be peer_left.
			var peerLeftMsg map[string]string
			if err := json.Unmarshal(sentB[1], &peerLeftMsg); err != nil {
				t.Fatalf("second frame not valid JSON: %v", err)
			}

			if peerLeftMsg["type"] != "peer_left" {
				t.Errorf("second frame type = %q, want peer_left", peerLeftMsg["type"]) // criterion: 5
			}
		})
	}
}

func TestArbitration_TimeoutSingleReporterLoses(t *testing.T) {
	t.Parallel()

	// criterion: 3 — confirmation buffer timer fires when only one peer reports;
	// that peer (the first reporter) is the loser, the other member wins.

	t0 := time.Unix(0, 0)

	tests := []struct {
		name         string
		wantLoserID  int64
		wantWinnerID int64
	}{
		{
			// criterion: 3 — fails if the lone reporter (A) is not loser after timer fires
			name:         "timeout-single-reporter-loses: only A reports, timer fires, A=loser B=winner",
			wantLoserID:  1,
			wantWinnerID: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newArbFixture(t, []time.Time{t0}, 150*time.Millisecond)
			connA := newFakeConn(1)
			connB := newFakeConn(2)
			f.seedRoom("room-timeout", connA, connB)

			if err := f.svc.ReportEvent(context.Background(), connA, "room-timeout", domain.TypeBlink); err != nil {
				t.Fatalf("ReportEvent error = %v", err)
			}

			// Manually fire the confirmation-buffer timer.
			f.ct.fire()

			// Both peers receive outcome.
			for _, c := range []*fakeConn{connA, connB} {
				frames := c.Sent()
				if len(frames) != 1 {
					t.Fatalf("conn %d received %d frames, want 1", c.UserID(), len(frames)) // criterion: 3
				}

				w, l := parseOutcome(t, frames[0])
				if w != tt.wantWinnerID || l != tt.wantLoserID {
					t.Errorf("conn %d: got winner=%d loser=%d, want winner=%d loser=%d", // criterion: 3
						c.UserID(), w, l, tt.wantWinnerID, tt.wantLoserID)
				}
			}
		})
	}
}

func TestArbitration_SameMemberDoubleReport(t *testing.T) {
	t.Parallel()

	// criterion: 2 — same member reporting twice before the opponent must be a no-op;
	// the match must only be decided when the true opponent submits its report.

	t0 := time.Unix(0, 0)
	t1 := t0.Add(5 * time.Millisecond)
	t2 := t0.Add(10 * time.Millisecond)
	t3 := t0.Add(15 * time.Millisecond)

	tests := []struct {
		name         string
		wantWinnerID int64
		wantLoserID  int64
	}{
		{
			// criterion: 2 — fails if same-member second report decides the match
			// (winner_id == loser_id) or locks out the true opponent with ErrMatchFinished.
			name:         "same-member double-report is no-op; opponent report decides correctly",
			wantWinnerID: 2, // B (later timestamp) wins
			wantLoserID:  1, // A (earlier timestamp) loses
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// now() sequence: t0 (A first report), t1 (A second report — ignored),
			// t2 (B report — decides), t3 (unused).
			f := newArbFixture(t, []time.Time{t0, t1, t2, t3}, 150*time.Millisecond)
			connA := newFakeConn(1)
			connB := newFakeConn(2)
			f.seedRoom("room-double", connA, connB)

			// A reports first.
			if err := f.svc.ReportEvent(context.Background(), connA, "room-double", domain.TypeBlink); err != nil {
				t.Fatalf("first ReportEvent (A) error = %v", err)
			}

			// A reports again (duplicate) — must return nil, not decide the match.
			err := f.svc.ReportEvent(context.Background(), connA, "room-double", domain.TypeBlink)
			if err != nil {
				t.Errorf("same-member second ReportEvent error = %v, want nil", err) // criterion: 2
			}

			// Match must not be decided yet.
			f.svc.mu.Lock()
			arb := f.svc.arbitrations["room-double"]
			decided := arb != nil && arb.decided
			f.svc.mu.Unlock()

			if decided {
				t.Error("arb.decided = true after same-member double report, want false") // criterion: 2
			}

			// No outcome must have been sent to either member yet.
			if n := len(connA.Sent()); n != 0 {
				t.Errorf("connA received %d frames after double report, want 0", n) // criterion: 2
			}

			if n := len(connB.Sent()); n != 0 {
				t.Errorf("connB received %d frames after double report, want 0", n) // criterion: 2
			}

			// Now the true opponent (B) reports — match must be decided with distinct IDs.
			if err := f.svc.ReportEvent(context.Background(), connB, "room-double", domain.TypeBlink); err != nil {
				t.Fatalf("opponent ReportEvent (B) error = %v", err)
			}

			for _, c := range []*fakeConn{connA, connB} {
				frames := c.Sent()
				if len(frames) != 1 {
					t.Fatalf("conn %d received %d frames after opponent report, want 1", c.UserID(), len(frames)) // criterion: 2
				}

				w, l := parseOutcome(t, frames[0])
				if w == l {
					t.Errorf("conn %d: winner_id == loser_id == %d, want distinct IDs", c.UserID(), w) // criterion: 2
				}

				if w != tt.wantWinnerID || l != tt.wantLoserID {
					t.Errorf("conn %d: got winner=%d loser=%d, want winner=%d loser=%d", // criterion: 2
						c.UserID(), w, l, tt.wantWinnerID, tt.wantLoserID)
				}
			}
		})
	}
}

func TestArbitration_AlreadyFinishedRoomRejected(t *testing.T) {
	t.Parallel()

	// criterion: 4 — report for an already-finished match is rejected.

	t0 := time.Unix(0, 0)
	t1 := t0.Add(5 * time.Millisecond)
	t2 := t0.Add(15 * time.Millisecond)

	tests := []struct {
		name string
	}{
		{
			// criterion: 4 — fails if report on decided room is not rejected
			name: "already-finished match report returns ErrMatchFinished",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newArbFixture(t, []time.Time{t0, t1, t2}, 150*time.Millisecond)
			connA := newFakeConn(1)
			connB := newFakeConn(2)
			f.seedRoom("room-finished", connA, connB)

			if err := f.svc.ReportEvent(context.Background(), connA, "room-finished", domain.TypeBlink); err != nil {
				t.Fatalf("first ReportEvent = %v", err)
			}

			if err := f.svc.ReportEvent(context.Background(), connB, "room-finished", domain.TypeBlink); err != nil {
				t.Fatalf("second ReportEvent = %v", err)
			}

			err := f.svc.ReportEvent(context.Background(), connB, "room-finished", domain.TypeFaceLost)
			if !errors.Is(err, domain.ErrMatchFinished) {
				t.Errorf("third ReportEvent = %v, want ErrMatchFinished", err) // criterion: 4
			}
		})
	}
}
