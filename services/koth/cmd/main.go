// Command koth runs the King-of-the-Hill microservice.
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/pizdagladki/full/services/koth/internal/app"
)

// shutdownSignals are the OS signals that trigger a graceful shutdown of the
// service (cancels the ctx passed to app.Run).
var shutdownSignals = []os.Signal{syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP}

func main() {
	err := run()
	if err != nil {
		log.Printf("koth service exited with error: %v", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), shutdownSignals...)
	defer stop()

	return app.New("koth").Run(ctx)
}
