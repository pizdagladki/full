package domain_test

import (
	"errors"
	"testing"

	"github.com/pizdagladki/full/services/store/internal/api/domain"
)

func TestErrInvalidCredit(t *testing.T) {
	t.Parallel()

	err := domain.ErrInvalidCredit
	if err == nil {
		t.Fatal("ErrInvalidCredit is nil")
	}

	if !errors.Is(err, domain.ErrInvalidCredit) {
		t.Error("errors.Is(ErrInvalidCredit, ErrInvalidCredit) = false, want true")
	}
}

func TestReasonConstants(t *testing.T) {
	t.Parallel()

	if domain.ReasonMatchWin != "match_win" {
		t.Errorf("ReasonMatchWin = %q, want %q", domain.ReasonMatchWin, "match_win")
	}

	if domain.ReasonLevelUp != "level_up" {
		t.Errorf("ReasonLevelUp = %q, want %q", domain.ReasonLevelUp, "level_up")
	}
}

func TestPointsCredit_Fields(t *testing.T) {
	t.Parallel()

	pc := domain.PointsCredit{
		UserID: 7,
		Reason: domain.ReasonMatchWin,
		RefID:  "match-123",
		Delta:  0,
	}

	if pc.UserID != 7 {
		t.Errorf("UserID = %d, want 7", pc.UserID)
	}

	if pc.Reason != domain.ReasonMatchWin {
		t.Errorf("Reason = %q, want %q", pc.Reason, domain.ReasonMatchWin)
	}

	if pc.RefID != "match-123" {
		t.Errorf("RefID = %q, want %q", pc.RefID, "match-123")
	}

	if pc.Delta != 0 {
		t.Errorf("Delta = %d, want 0", pc.Delta)
	}
}
