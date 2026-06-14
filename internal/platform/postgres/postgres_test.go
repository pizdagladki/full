package postgres

import (
	"context"
	"testing"
)

func TestNew_InvalidDSN(t *testing.T) {
	t.Parallel()

	_, err := New(context.Background(), "postgres://localhost:not-a-port/db")
	if err == nil {
		t.Fatal("expected an error for a malformed DSN, got nil")
	}
}
