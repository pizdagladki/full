// Command health runs the health microservice.
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/pizdagladki/full/services/health/internal/app"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(),
		syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)
	defer stop()

	if err := app.New("health").Run(ctx); err != nil {
		log.Printf("health service exited with error: %v", err)
		os.Exit(1)
	}
}
