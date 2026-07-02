package domain

import "errors"

// ReasonMatchWin identifies points earned for winning a match. Reasons are an
// extensible set — they only key the config amount lookup and are NOT
// validated against a fixed list.
const ReasonMatchWin = "match_win"

// ReasonLevelUp identifies points earned for leveling up.
const ReasonLevelUp = "level_up"

// ErrInvalidCredit is returned when a points credit request has an empty
// reason or a resolved delta that is not positive.
var ErrInvalidCredit = errors.New("invalid points credit")

// PointsCredit is the domain input for crediting points to a user.
// Delta is an explicit override; 0 means "resolve the amount from config by
// Reason".
type PointsCredit struct {
	UserID int64
	Reason string
	RefID  string
	Delta  int64
}
