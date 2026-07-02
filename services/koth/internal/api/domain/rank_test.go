package domain_test

import (
	"testing"

	"github.com/pizdagladki/full/services/koth/internal/api/domain"
)

var testThresholds = []int{5000, 15000, 30000, 60000, 120000}

// TestComputeRank verifies criterion: 1 — threshold->rank mapping (ascending
// thresholds; achieved rank is the number of thresholds <= heldMs).
func TestComputeRank(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		heldMs     int
		thresholds []int
		want       int
	}{
		{
			// criterion: 1 — below the first threshold stays at rank 0
			name:       "below first threshold is rank 0",
			heldMs:     4999,
			thresholds: testThresholds,
			want:       0,
		},
		{
			// criterion: 1 — exactly meeting the first threshold reaches rank 1
			name:       "exact first threshold reaches rank 1",
			heldMs:     5000,
			thresholds: testThresholds,
			want:       1,
		},
		{
			name:       "between first and second threshold stays rank 1",
			heldMs:     14999,
			thresholds: testThresholds,
			want:       1,
		},
		{
			// criterion: 1 — meeting the second threshold reaches rank 2
			name:       "exact second threshold reaches rank 2",
			heldMs:     15000,
			thresholds: testThresholds,
			want:       2,
		},
		{
			// criterion: 1 — exceeding the top threshold reaches the max rank
			name:       "beyond top threshold reaches max rank",
			heldMs:     999999,
			thresholds: testThresholds,
			want:       5,
		},
		{
			name:       "zero held ms is rank 0",
			heldMs:     0,
			thresholds: testThresholds,
			want:       0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := domain.ComputeRank(tt.heldMs, tt.thresholds)
			if got != tt.want {
				t.Errorf("ComputeRank(%d) = %d, want %d", tt.heldMs, got, tt.want)
			}
		})
	}
}

// TestNextTargetMs verifies criterion: 2 — the /me next_target_ms computation
// (rank 0 -> thresholds[0]; max rank -> 0, no higher target).
func TestNextTargetMs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		currentRank int
		thresholds  []int
		want        int
	}{
		{
			// criterion: 2 — rank 0 targets the first threshold
			name:        "rank 0 targets first threshold",
			currentRank: 0,
			thresholds:  testThresholds,
			want:        5000,
		},
		{
			name:        "rank 1 targets second threshold",
			currentRank: 1,
			thresholds:  testThresholds,
			want:        15000,
		},
		{
			// criterion: 2 — already at the max rank has no higher target (0)
			name:        "max rank has no higher target",
			currentRank: 5,
			thresholds:  testThresholds,
			want:        0,
		},
		{
			name:        "rank beyond max defensively returns 0",
			currentRank: 9,
			thresholds:  testThresholds,
			want:        0,
		},
		{
			name:        "negative rank defensively returns 0",
			currentRank: -1,
			thresholds:  testThresholds,
			want:        0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := domain.NextTargetMs(tt.currentRank, tt.thresholds)
			if got != tt.want {
				t.Errorf("NextTargetMs(%d) = %d, want %d", tt.currentRank, got, tt.want)
			}
		})
	}
}
