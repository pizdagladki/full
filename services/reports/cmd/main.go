// Command reports runs the reports microservice.
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/pizdagladki/full/services/reports/internal/app"
)

func main() {
	err := run()
	if err != nil {
		log.Printf("reports service exited with error: %v", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(),
		syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)
	defer stop()

	return app.New("reports").Run(ctx)
}
