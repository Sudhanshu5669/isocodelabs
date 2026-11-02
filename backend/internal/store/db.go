// Package store is the only place that talks to the database.
package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Connect opens a connection pool to Postgres.
//
// Go lesson: context.Context carries deadlines and cancellation through a call
// chain. Almost every function that does I/O (database, network) takes one as
// its first argument so it can be stopped if the caller gives up.
//
// Neon's free databases go to sleep when idle, so the first connection after a
// pause can fail or be slow. We retry a few times with a growing delay.
func Connect(ctx context.Context, url string) (*pgxpool.Pool, error) {
	var lastErr error

	for attempt := 1; attempt <= 4; attempt++ {
		pool, err := pgxpool.New(ctx, url)
		if err == nil {
			// pgxpool.New is lazy, so Ping makes sure we can really reach the DB.
			pingCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
			err = pool.Ping(pingCtx)
			cancel()
			if err == nil {
				return pool, nil
			}
			pool.Close()
		}
		lastErr = err

		select {
		case <-time.After(time.Duration(attempt) * time.Second):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return nil, fmt.Errorf("store: could not connect to database: %w", lastErr)
}
