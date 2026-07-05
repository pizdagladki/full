package domain

import "time"

// Reason keys for the final-placement reward credited to the outgoing king
// when a daily/monthly reign is closed at rollover. These are free-form
// lookup keys into the store's PointsConfig.Amounts map (see the store
// service's config) — koth does not need to modify the store to introduce
// new reason keys.
const (
	// ReasonKothDailyFinal is the reward reason for the DAILY hill's
	// final-placement credit at day rollover.
	ReasonKothDailyFinal = "koth_daily_final"
	// ReasonKothMonthlyFinal is the reward reason for the MONTHLY hill's
	// final-placement credit at month rollover.
	ReasonKothMonthlyFinal = "koth_monthly_final"
)

// PeriodStart returns the start (00:00:00 UTC) of the calendar period that
// now belongs to for hillType: the start of the day for HillTypeDaily, the
// start of the month for HillTypeMonthly. now is converted to UTC first, so
// the boundary is always computed in UTC regardless of now's location.
// Any other hillType falls back to the daily boundary.
func PeriodStart(hillType HillType, now time.Time) time.Time {
	u := now.UTC()

	if hillType == HillTypeMonthly {
		return time.Date(u.Year(), u.Month(), 1, 0, 0, 0, 0, time.UTC)
	}

	return time.Date(u.Year(), u.Month(), u.Day(), 0, 0, 0, 0, time.UTC)
}
