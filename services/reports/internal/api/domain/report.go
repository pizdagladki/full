package domain

// CheatReport is the domain model for a player cheat report.
type CheatReport struct {
	ReporterID int64
	ReportedID int64
	MatchID    string
}

// CooldownStatus describes the current cooldown state for a user.
type CooldownStatus struct {
	Active           bool
	SecondsRemaining int
}

// BugReport is the domain model for a bug report.
type BugReport struct {
	UserID      int64
	Device      string
	Description string
	ObjectKey   string // empty for mobile reports
}
