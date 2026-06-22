package domain

import "testing"

// ─── ELO delta tests ─────────────────────────────────────────────────────────

func TestCalcELODeltas(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		rw         int // winner ELO
		rl         int // loser ELO
		gwPlayed   int // winner games_played (determines K_w)
		glPlayed   int // loser  games_played (determines K_l)
		wantWinner int
		wantLoser  int
	}{
		// Required spec vectors ────────────────────────────────────────────────
		{
			name:       "equal ratings calibrated (K=32) +16/-13",
			rw:         1000,
			rl:         1000,
			gwPlayed:   20, // K=32
			glPlayed:   20, // K=32
			wantWinner: 16,
			wantLoser:  -13,
		},
		{
			name:       "equal ratings new (K=64) +32/-26",
			rw:         1000,
			rl:         1000,
			gwPlayed:   0, // K=64
			glPlayed:   0, // K=64
			wantWinner: 32,
			wantLoser:  -26,
		},
		{
			// Upset: underdog 1000 beats favourite 1400 — large winner gain, small loser loss.
			// raw_wd = 32*(1-0.0909) ≈ 29.09 → 29; raw_ld = 0.8*32*(0-0.9091) ≈ -23.27 → -23
			name:       "upset: 1000 beats 1400 (K=32) winner=+29 loser=-23",
			rw:         1000,
			rl:         1400,
			gwPlayed:   20,
			glPlayed:   20,
			wantWinner: 29,
			wantLoser:  -23,
		},
		{
			// Favourite: 1400 beats expected 1000 — small winner gain, tiny loser loss.
			// raw_wd = 32*(1-0.9091) ≈ 2.91 → 3; raw_ld = 0.8*32*(0-0.0909) ≈ -2.33 → -2
			name:       "favourite: 1400 beats 1000 (K=32) winner=+3 loser=-2",
			rw:         1400,
			rl:         1000,
			gwPlayed:   20,
			glPlayed:   20,
			wantWinner: 3,
			wantLoser:  -2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gotW, gotL := CalcELODeltas(tt.rw, tt.rl, tt.gwPlayed, tt.glPlayed)

			if gotW != tt.wantWinner {
				t.Errorf("winnerDelta = %d, want %d", gotW, tt.wantWinner)
			}
			if gotL != tt.wantLoser {
				t.Errorf("loserDelta  = %d, want %d", gotL, tt.wantLoser)
			}
		})
	}
}

func TestKFactor(t *testing.T) {
	t.Parallel()

	tests := []struct {
		gamesPlayed int
		wantK       float64
	}{
		{gamesPlayed: 0, wantK: 64},
		{gamesPlayed: 19, wantK: 64},
		{gamesPlayed: 20, wantK: 32},
		{gamesPlayed: 100, wantK: 32},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			t.Parallel()

			got := kFactor(tt.gamesPlayed)
			if got != tt.wantK {
				t.Errorf("kFactor(%d) = %v, want %v", tt.gamesPlayed, got, tt.wantK)
			}
		})
	}
}

// ─── Level band tests ─────────────────────────────────────────────────────────

func TestLevelForELO(t *testing.T) {
	t.Parallel()

	tests := []struct {
		elo       int
		wantLevel int
	}{
		// Below-floor value
		{elo: 100, wantLevel: 1},
		// L1 boundary
		{elo: 500, wantLevel: 1},
		// L2 boundaries
		{elo: 501, wantLevel: 2},
		{elo: 700, wantLevel: 2},
		// L3 boundaries
		{elo: 701, wantLevel: 3},
		{elo: 900, wantLevel: 3},
		// L4 boundaries (includes default 1000)
		{elo: 901, wantLevel: 4},
		{elo: 1000, wantLevel: 4},
		{elo: 1100, wantLevel: 4},
		// L5 boundary
		{elo: 1101, wantLevel: 5},
		// L8 boundary
		{elo: 1900, wantLevel: 8},
		// L9 boundaries
		{elo: 1901, wantLevel: 9},
		{elo: 2000, wantLevel: 9},
		// L10 boundary
		{elo: 2001, wantLevel: 10},
		// Well above
		{elo: 3000, wantLevel: 10},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			t.Parallel()

			got := LevelForELO(tt.elo)
			if got != tt.wantLevel {
				t.Errorf("LevelForELO(%d) = %d, want %d", tt.elo, got, tt.wantLevel)
			}
		})
	}
}
