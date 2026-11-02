package store

import "github.com/jackc/pgx/v5/pgxpool"

// Store groups every database operation. Other packages receive a *Store and
// never write SQL themselves.
type Store struct {
	pool *pgxpool.Pool
}

// New wraps a connection pool.
func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}
