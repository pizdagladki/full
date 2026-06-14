package app

import (
	"context"
	"testing"
	"time"
)

func TestApp_Run_GracefulShutdown(t *testing.T) {
	t.Setenv("IS_DOCKER", "1")
	t.Setenv("HTTP_ADDR", "127.0.0.1:0")

	a := New("health-test")
	if a == nil {
		t.Fatal("New returned nil")
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Cancel shortly after start so workerHTTP exercises the graceful-shutdown path.
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	err := a.Run(ctx)
	if err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}
}
