// Package testdb gives integration tests a real, empty Postgres database to use.
//
// The guarantees we test (row locks, unique indexes, transactions) only exist in
// a real database, so tests run against Neon. To keep the sample data safe, each
// test run creates a throw-away schema, applies the real migrations into it, and
// drops it afterwards. Nothing in the real tables is ever touched.
//
// Tests need DATABASE_URL (a direct, non-pooled connection). Without it Open
// returns a nil pool and tests skip themselves.
package testdb

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/joho/godotenv"
	"github.com/pressly/goose/v3"

	"leadflow/migrations"
)

// Open prepares an isolated schema and returns a pool that can only see it.
// The returned cleanup function drops the schema; always call it.
// A nil pool with a nil error means "no database configured, skip the tests".
func Open(ctx context.Context, envFile string) (pool *pgxpool.Pool, cleanup func(), err error) {
	noop := func() {}

	_ = godotenv.Load(envFile)
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		return nil, noop, nil
	}

	admin, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, noop, err
	}

	raw := make([]byte, 6)
	_, _ = rand.Read(raw)
	schema := "t_" + hex.EncodeToString(raw)
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		admin.Close()
		return nil, noop, err
	}

	var testPool *pgxpool.Pool
	var db *sql.DB
	cleanup = func() {
		if testPool != nil {
			testPool.Close()
		}
		if db != nil {
			db.Close()
		}
		_, _ = admin.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE")
		admin.Close()
	}

	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		cleanup()
		return nil, noop, err
	}

	// Only the throw-away schema is on the path, so no statement in a test can
	// ever resolve to the real tables in "public".
	//
	// Neon silently ignores search_path as a connection startup parameter, so we
	// SET it right after every new connection instead.
	setPath := func(ctx context.Context, c *pgx.Conn) error {
		_, err := c.Exec(ctx, "SET search_path TO "+schema)
		return err
	}
	cfg.AfterConnect = setPath
	cfg.MaxConns = 25

	if testPool, err = pgxpool.NewWithConfig(ctx, cfg); err != nil {
		cleanup()
		return nil, noop, err
	}

	db = stdlib.OpenDB(*cfg.ConnConfig, stdlib.OptionAfterConnect(setPath))
	goose.SetBaseFS(migrations.FS)
	_ = goose.SetDialect("postgres")
	goose.SetLogger(goose.NopLogger())
	// Keep goose's bookkeeping table inside the throw-away schema too. Without
	// this it can see the real one in "public" and think nothing needs migrating.
	goose.SetTableName(schema + ".goose_db_version")
	if err := goose.Up(db, "."); err != nil {
		cleanup()
		return nil, noop, fmt.Errorf("migrate test schema: %w", err)
	}

	// Safety guard: refuse to continue unless the tables really live in our schema.
	var tables int
	if err := testPool.QueryRow(ctx, `SELECT count(*) FROM information_schema.tables
		WHERE table_schema = $1 AND table_name IN ('users', 'visit_slots', 'visits')`, schema).Scan(&tables); err != nil || tables != 3 {
		cleanup()
		return nil, noop, errors.New("test schema was not migrated; refusing to continue")
	}

	// Fixture every test needs: two projects.
	if _, err := testPool.Exec(ctx, `INSERT INTO projects (id, name, rera_number, locality, address, latitude, longitude)
		VALUES ('project-1', 'Project 1', 'TEST-RERA', 'Ahmedabad', 'Test address', 23.0, 72.5),
		       ('project-2', 'Project 2', 'TEST-RERA-2', 'Ahmedabad', 'Test address', 23.1, 72.6)`); err != nil {
		cleanup()
		return nil, noop, err
	}
	return testPool, cleanup, nil
}
