package domain_test

import (
	"errors"
	"testing"

	"github.com/pizdagladki/full/services/store/internal/api/domain"
)

func TestValidKind(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		kind string
		want bool
	}{
		{name: "distraction is valid", kind: "distraction", want: true},
		{name: "edit is valid", kind: "edit", want: true},
		{name: "empty string is invalid", kind: "", want: false},
		{name: "unknown kind is invalid", kind: "unknown", want: false},
		{name: "uppercase is invalid", kind: "Distraction", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := domain.ValidKind(tt.kind)
			if got != tt.want {
				t.Errorf("ValidKind(%q) = %v, want %v", tt.kind, got, tt.want)
			}
		})
	}
}

func TestErrInvalidKind(t *testing.T) {
	t.Parallel()

	// Verify the sentinel is distinct and wrappable.
	err := domain.ErrInvalidKind
	if err == nil {
		t.Fatal("ErrInvalidKind is nil")
	}

	if !errors.Is(err, domain.ErrInvalidKind) {
		t.Error("errors.Is(ErrInvalidKind, ErrInvalidKind) = false, want true")
	}
}

func TestKindConstants(t *testing.T) {
	t.Parallel()

	if domain.KindDistraction != "distraction" {
		t.Errorf("KindDistraction = %q, want %q", domain.KindDistraction, "distraction")
	}

	if domain.KindEdit != "edit" {
		t.Errorf("KindEdit = %q, want %q", domain.KindEdit, "edit")
	}
}
