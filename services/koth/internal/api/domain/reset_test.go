package domain_test

import (
	"testing"
	"time"

	"github.com/pizdagladki/full/services/koth/internal/api/domain"
)

// TestPeriodStart verifies criterion: 1 and 2 — PeriodStart computes the
// day/month boundary (in UTC) that a given instant belongs to, which is what
// the daily/monthly reset jobs compare a reign's StartedAt against to decide
// whether the boundary has rolled over.
func TestPeriodStart(t *testing.T) {
	t.Parallel()

	est := time.FixedZone("EST", -5*60*60)

	tests := []struct {
		name     string
		hillType domain.HillType
		now      time.Time
		want     time.Time
	}{
		{
			// criterion: 1 — the daily boundary is midnight UTC of now's calendar day
			name:     "daily boundary is midnight UTC of the current day",
			hillType: domain.HillTypeDaily,
			now:      time.Date(2026, 7, 4, 15, 30, 0, 0, time.UTC),
			want:     time.Date(2026, 7, 4, 0, 0, 0, 0, time.UTC),
		},
		{
			// criterion: 1 — daily boundary normalizes a non-UTC input to UTC first
			name:     "daily boundary normalizes non-UTC input to UTC",
			hillType: domain.HillTypeDaily,
			// 2026-07-04 23:30 EST == 2026-07-05 04:30 UTC — must land on 07-05.
			now:  time.Date(2026, 7, 4, 23, 30, 0, 0, est),
			want: time.Date(2026, 7, 5, 0, 0, 0, 0, time.UTC),
		},
		{
			// criterion: 2 — the monthly boundary is midnight UTC of the 1st of now's month
			name:     "monthly boundary is the 1st of the month at midnight UTC",
			hillType: domain.HillTypeMonthly,
			now:      time.Date(2026, 7, 31, 23, 59, 59, 0, time.UTC),
			want:     time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			// criterion: 2 — a monthly boundary computed at the very start of the month is itself
			name:     "monthly boundary at start of month is unchanged",
			hillType: domain.HillTypeMonthly,
			now:      time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
			want:     time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := domain.PeriodStart(tt.hillType, tt.now)

			if !got.Equal(tt.want) || got.Location() != time.UTC {
				t.Errorf("PeriodStart(%v, %v) = %v, want %v (UTC)", tt.hillType, tt.now, got, tt.want)
			}
		})
	}
}
