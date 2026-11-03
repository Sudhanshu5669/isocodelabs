// Package store is the only place that talks to the database.
package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// supabaseTxPoolerPort is the port of Supabase's transaction-mode pooler.
const supabaseTxPoolerPort = 6543

// Connect opens a connection pool to Postgres (Supabase).
//
// Go lesson: context.Context carries deadlines and cancellation through a call
// chain. Almost every function that does I/O (database, network) takes one as
// its first argument so it can be stopped if the caller gives up.
//
// The Supabase pooler can drop or refuse a connection for a moment (for example
// while a paused free project wakes up), so we retry a few times with a growing delay.
func Connect(ctx context.Context, url string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("store: bad database URL: %w", err)
	}

	// The transaction pooler hands each transaction to whichever server
	// connection is free, so a prepared statement made on one may not exist on
	// the next. The simple protocol sends every query as plain text and never
	// prepares statements, which is what the pooler needs.
	if cfg.ConnConfig.Port == supabaseTxPoolerPort {
		cfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	}

	var lastErr error
	for attempt := 1; attempt <= 4; attempt++ {
		pool, err := pgxpool.NewWithConfig(ctx, cfg)
		if err == nil {
			// The pool is lazy, so Ping makes sure we can really reach the DB.
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
