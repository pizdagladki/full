package domain_test

import (
	"errors"
	"testing"

	"github.com/pizdagladki/full/services/koth/internal/api/domain"
)

// TestParseHillType verifies criterion: 5 — an invalid hill_type is rejected
// with domain.ErrInvalidHillType, while "daily"/"monthly" parse cleanly.
func TestParseHillType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		raw     string
		want    domain.HillType
		wantErr bool
	}{
		{
			name: "daily parses",
			raw:  "daily",
			want: domain.HillTypeDaily,
		},
		{
			name: "monthly parses",
			raw:  "monthly",
			want: domain.HillTypeMonthly,
		},
		{
			// criterion: 5 — an invalid hill_type value is rejected
			name:    "invalid value returns ErrInvalidHillType",
			raw:     "weekly",
			wantErr: true,
		},
		{
			// criterion: 5 — an empty hill_type value is rejected
			name:    "empty value returns ErrInvalidHillType",
			raw:     "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := domain.ParseHillType(tt.raw)

			if tt.wantErr {
				if !errors.Is(err, domain.ErrInvalidHillType) {
					t.Errorf("ParseHillType(%q) error = %v, want ErrInvalidHillType", tt.raw, err)
				}

				return
			}

			if err != nil {
				t.Fatalf("ParseHillType(%q) unexpected error = %v", tt.raw, err)
			}

			if got != tt.want {
				t.Errorf("ParseHillType(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}
