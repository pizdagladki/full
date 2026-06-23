package domain

import (
	"testing"
	"time"
)

func TestValidateLevel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		level   int
		wantErr bool
	}{
		{"valid min", 1, false},
		{"valid max", 10, false},
		{"valid mid", 5, false},
		{"zero invalid", 0, true},
		{"eleven invalid", 11, true},
		{"negative invalid", -1, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateLevel(tt.level)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateLevel(%d) error = %v, wantErr = %v", tt.level, err, tt.wantErr)
			}
			if tt.wantErr && err != ErrInvalidLevel {
				t.Errorf("ValidateLevel(%d) error = %v, want ErrInvalidLevel", tt.level, err)
			}
		})
	}
}

func TestValidateMode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		mode    string
		wantErr bool
	}{
		{"valid short", "ranked", false},
		{"valid 64 chars", string(make([]byte, 64)), false},
		{"empty invalid", "", true},
		{"65 chars invalid", string(make([]byte, 65)), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateMode(tt.mode)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateMode(%q) error = %v, wantErr = %v", tt.mode, err, tt.wantErr)
			}
			if tt.wantErr && err != ErrInvalidMode {
				t.Errorf("ValidateMode(%q) error = %v, want ErrInvalidMode", tt.mode, err)
			}
		})
	}
}

func TestValidateJoin(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		mode    string
		level   int
		wantErr bool
	}{
		{"valid", "ranked", 5, false},
		{"bad mode", "", 5, true},
		{"bad level", "ranked", 0, true},
		{"both bad returns mode error first", "", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateJoin(tt.mode, tt.level)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateJoin(%q, %d) error = %v, wantErr = %v", tt.mode, tt.level, err, tt.wantErr)
			}
		})
	}
}

var (
	t0 = time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	t1 = t0.Add(time.Second)
	t2 = t0.Add(2 * time.Second)
)

func TestNearestWithinDistance(t *testing.T) {
	t.Parallel()

	candidate := Player{UserID: 1, Mode: "ranked", Level: 5, EnqueuedAt: t0}

	tests := []struct {
		name       string
		waiting    []Player
		maxDist    int
		wantUserID int64
		wantNil    bool
	}{
		{
			name:    "empty list returns nil",
			waiting: nil,
			maxDist: 3,
			wantNil: true,
		},
		{
			name: "only candidate in list returns nil",
			waiting: []Player{
				{UserID: 1, Level: 5, EnqueuedAt: t0},
			},
			maxDist: 3,
			wantNil: true,
		},
		{
			name: "nobody within distance returns nil",
			waiting: []Player{
				{UserID: 2, Level: 9, EnqueuedAt: t0},
			},
			maxDist: 3,
			wantNil: true,
		},
		{
			name: "single within distance",
			waiting: []Player{
				{UserID: 2, Level: 7, EnqueuedAt: t0},
			},
			maxDist:    3,
			wantUserID: 2,
		},
		{
			name: "picks closer over farther",
			waiting: []Player{
				{UserID: 2, Level: 7, EnqueuedAt: t0},
				{UserID: 3, Level: 6, EnqueuedAt: t0},
			},
			maxDist:    3,
			wantUserID: 3, // diff 1 beats diff 2
		},
		{
			name: "tie-break by earlier enqueued",
			waiting: []Player{
				{UserID: 2, Level: 6, EnqueuedAt: t2},
				{UserID: 3, Level: 6, EnqueuedAt: t1},
			},
			maxDist:    3,
			wantUserID: 3, // same diff, earlier enqueue
		},
		{
			name: "tie-break by lower user id when enqueue equal",
			waiting: []Player{
				{UserID: 4, Level: 6, EnqueuedAt: t1},
				{UserID: 2, Level: 6, EnqueuedAt: t1},
			},
			maxDist:    3,
			wantUserID: 2, // lower ID wins
		},
		{
			name: "exact distance boundary included",
			waiting: []Player{
				{UserID: 2, Level: 8, EnqueuedAt: t0}, // diff=3 == maxDist
			},
			maxDist:    3,
			wantUserID: 2,
		},
		{
			name: "one over boundary excluded",
			waiting: []Player{
				{UserID: 2, Level: 9, EnqueuedAt: t0}, // diff=4 > maxDist=3
			},
			maxDist: 3,
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := NearestWithinDistance(candidate, tt.waiting, tt.maxDist)
			if tt.wantNil {
				if got != nil {
					t.Errorf("NearestWithinDistance() = %v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Fatal("NearestWithinDistance() = nil, want non-nil")
			}
			if got.UserID != tt.wantUserID {
				t.Errorf("NearestWithinDistance().UserID = %d, want %d", got.UserID, tt.wantUserID)
			}
		})
	}
}

func TestNearestRegardless(t *testing.T) {
	t.Parallel()

	candidate := Player{UserID: 1, Mode: "ranked", Level: 5, EnqueuedAt: t0}

	tests := []struct {
		name       string
		waiting    []Player
		wantUserID int64
		wantNil    bool
	}{
		{
			name:    "empty returns nil",
			waiting: nil,
			wantNil: true,
		},
		{
			name: "only candidate returns nil",
			waiting: []Player{
				{UserID: 1, Level: 5, EnqueuedAt: t0},
			},
			wantNil: true,
		},
		{
			name: "picks nearest by diff ignoring max distance",
			waiting: []Player{
				{UserID: 2, Level: 9, EnqueuedAt: t0}, // diff=4
				{UserID: 3, Level: 7, EnqueuedAt: t0}, // diff=2
			},
			wantUserID: 3,
		},
		{
			name: "single valid opponent",
			waiting: []Player{
				{UserID: 42, Level: 1, EnqueuedAt: t0}, // diff=4
			},
			wantUserID: 42,
		},
		{
			name: "tie-break earliest enqueue",
			waiting: []Player{
				{UserID: 2, Level: 5, EnqueuedAt: t2},
				{UserID: 3, Level: 5, EnqueuedAt: t1},
			},
			wantUserID: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := NearestRegardless(candidate, tt.waiting)
			if tt.wantNil {
				if got != nil {
					t.Errorf("NearestRegardless() = %v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Fatal("NearestRegardless() = nil, want non-nil")
			}
			if got.UserID != tt.wantUserID {
				t.Errorf("NearestRegardless().UserID = %d, want %d", got.UserID, tt.wantUserID)
			}
		})
	}
}

func TestPastFallbackDeadline(t *testing.T) {
	t.Parallel()

	base := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	dur := 10 * time.Second

	tests := []struct {
		name    string
		now     time.Time
		enqueue time.Time
		want    bool
	}{
		{"not yet past deadline", base.Add(9 * time.Second), base, false},
		{"exactly at deadline", base.Add(10 * time.Second), base, true},
		{"past deadline", base.Add(11 * time.Second), base, true},
		{"just enrolled", base, base, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := PastFallbackDeadline(tt.now, tt.enqueue, dur)
			if got != tt.want {
				t.Errorf("PastFallbackDeadline() = %v, want %v", got, tt.want)
			}
		})
	}
}
