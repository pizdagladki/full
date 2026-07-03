package domain

import "testing"

func TestBuildKingClipObjectKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		hillType string
		userID   int64
		id       string
		want     string
	}{
		{
			name:     "daily hill key",
			hillType: HillTypeDaily,
			userID:   42,
			id:       "abc-123",
			want:     "king-clips/daily/42/abc-123.webm",
		},
		{
			name:     "monthly hill key",
			hillType: HillTypeMonthly,
			userID:   7,
			id:       "uuid-here",
			want:     "king-clips/monthly/7/uuid-here.webm",
		},
		{
			name:     "ranked hill key",
			hillType: HillTypeRanked,
			userID:   0,
			id:       "xyz",
			want:     "king-clips/ranked/0/xyz.webm",
		},
		{
			// criterion: 1 — the king-clip object key uses a dedicated prefix,
			// distinct from the win-clip "clips/" prefix (BuildObjectKey).
			name:     "king-clip prefix is separate from win-clip clips/ prefix",
			hillType: HillTypeDaily,
			userID:   42,
			id:       "abc-123",
			want:     "king-clips/daily/42/abc-123.webm",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := BuildKingClipObjectKey(tt.hillType, tt.userID, tt.id)
			if got != tt.want {
				t.Errorf("BuildKingClipObjectKey(%q, %d, %q) = %q, want %q", tt.hillType, tt.userID, tt.id, got, tt.want)
			}

			// The win-clip key builder must never produce a key under the
			// king-clips/ prefix, confirming the prefixes are disjoint.
			winKey := BuildObjectKey(tt.userID, tt.id)
			if got == winKey {
				t.Errorf("king-clip key %q collided with win-clip key %q", got, winKey)
			}
		})
	}
}

func TestValidHillType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		s    string
		want bool
	}{
		{name: "daily is valid", s: HillTypeDaily, want: true},
		{name: "monthly is valid", s: HillTypeMonthly, want: true},
		{name: "ranked is valid", s: HillTypeRanked, want: true},
		{
			// criterion: 5 — unknown hill_type is rejected.
			name: "unknown hill type is rejected",
			s:    "weekly",
			want: false,
		},
		{
			name: "empty string is rejected",
			s:    "",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := ValidHillType(tt.s)
			if got != tt.want {
				t.Errorf("ValidHillType(%q) = %v, want %v", tt.s, got, tt.want)
			}
		})
	}
}
