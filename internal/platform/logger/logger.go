// Package logger builds the shared zap logger used by all services.
package logger

import "go.uber.org/zap"

// New returns a production zap logger. Callers own its lifecycle and should
// defer logger.Sync() before exit.
func New() (*zap.Logger, error) {
	return zap.NewProduction()
}
