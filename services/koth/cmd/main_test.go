package main

// This file intentionally does NOT use t.Parallel(): the tests below send real
// OS signals to the current process, and running them concurrently with other
// signal-sensitive tests would be racy.

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"testing"
	"time"
)

// TestShutdownSignals_Set verifies criterion 2b: the shutdown signal set is
// exactly {SIGTERM, SIGINT, SIGHUP} — no signal dropped, none extra.
func TestShutdownSignals_Set(t *testing.T) {
	want := map[os.Signal]bool{
		syscall.SIGTERM: true,
		syscall.SIGINT:  true,
		syscall.SIGHUP:  true,
	}

	if len(shutdownSignals) != len(want) {
		t.Fatalf("len(shutdownSignals) = %d, want %d (got %v)", len(shutdownSignals), len(want), shutdownSignals)
	}

	seen := make(map[os.Signal]bool, len(shutdownSignals))
	for _, s := range shutdownSignals {
		seen[s] = true
	}

	for s := range want {
		if !seen[s] {
			t.Errorf("shutdownSignals is missing %v", s)
		}
	}
	for s := range seen {
		if !want[s] {
			t.Errorf("shutdownSignals has unexpected extra signal %v", s)
		}
	}
}

// TestShutdownSignals_CancelsContext verifies criterion 2b: sending any of the
// signals in shutdownSignals to the process cancels a context built with
// signal.NotifyContext(shutdownSignals...) — i.e. that set actually drives
// graceful shutdown, not just the literal name of the variable.
func TestShutdownSignals_CancelsContext(t *testing.T) {
	tests := []struct {
		name string
		sig  os.Signal
	}{
		{name: "SIGTERM cancels ctx", sig: syscall.SIGTERM},
		{name: "SIGINT cancels ctx", sig: syscall.SIGINT},
		{name: "SIGHUP cancels ctx", sig: syscall.SIGHUP},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Register the handler before sending the signal.
			ctx, stop := signal.NotifyContext(context.Background(), shutdownSignals...)
			defer stop()

			p, err := os.FindProcess(os.Getpid())
			if err != nil {
				t.Fatalf("FindProcess: %v", err)
			}

			if err := p.Signal(tt.sig); err != nil {
				t.Fatalf("Signal(%v): %v", tt.sig, err)
			}

			select {
			case <-ctx.Done():
				// ok: the signal cancelled the context.
			case <-time.After(3 * time.Second):
				t.Fatalf("ctx not cancelled on %v", tt.sig)
			}

			stop()
		})
	}
}
