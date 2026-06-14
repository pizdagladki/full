// Package postgres provides the shared pgxpool connector used by services that
// own Postgres-backed data. It arrives with the first service that needs it and
// must not be duplicated inside a service.
package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// New opens a pgxpool against dsn and verifies the connection with a ping. The
// pool is closed automatically when the ping fails, so the caller never receives
// a live pool together with an error.
func New(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		// The parse error from pgx can embed the raw DSN (incl. the password),
		// so we return a static message rather than wrapping it, keeping
		// credentials out of logs.
		return nil, errors.New("create pgx pool: invalid connection string")
	}

	err = pool.Ping(ctx)
	if err != nil {
		pool.Close()

		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	return pool, nil
}
