package redis

import (
	"context"
	"testing"
	"time"
)

func TestNew_PingFails(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Port 1 is reserved and refuses connections, so the ping fails fast.
	_, err := New(ctx, "127.0.0.1:1", "")
	if err == nil {
		t.Fatal("expected a ping error against an unreachable Redis, got nil")
	}
}
