package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// withTx runs fn inside a database transaction: everything fn does is saved
// together, or none of it is.
//
// If Postgres aborts the transaction because of a deadlock or a serialization
// clash (two transactions touching the same rows at once), the whole thing is
// safely retried a few times.
func (s *Store) withTx(ctx context.Context, fn func(tx pgx.Tx) error) error {
	var err error
	for attempt := 0; attempt < 4; attempt++ {
		err = s.runTx(ctx, fn)
		if !isRetryable(err) {
			return err
		}
		select {
		case <-time.After(time.Duration(attempt+1) * 25 * time.Millisecond):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return err
}

func (s *Store) runTx(ctx context.Context, fn func(tx pgx.Tx) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	// Rollback after a successful Commit does nothing, so deferring it is the
	// standard way to guarantee cleanup on every early return.
	defer tx.Rollback(ctx)

	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// rowQuerier is satisfied by both a pool and a transaction.
type rowQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func pgError(err error) *pgconn.PgError {
	var pe *pgconn.PgError
	if errors.As(err, &pe) {
		return pe
	}
	return nil
}

// isRetryable reports deadlock (40P01) and serialization failure (40001).
func isRetryable(err error) bool {
	if pe := pgError(err); pe != nil {
		return pe.Code == "40P01" || pe.Code == "40001"
	}
	return false
}

// constraintName returns the violated constraint or index, if err is one.
func constraintName(err error) string {
	if pe := pgError(err); pe != nil {
		return pe.ConstraintName
	}
	return ""
}

// nullable turns an empty string into SQL NULL.
func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}
