// Command migrate applies database migrations.
//
//	go run ./cmd/migrate          # apply every pending migration
//	go run ./cmd/migrate status   # show which have been applied
//	go run ./cmd/migrate down     # undo the most recent migration
package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"

	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" driver for database/sql
	"github.com/pressly/goose/v3"

	"leadflow/internal/config"
	"leadflow/migrations"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "migrate:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	// Step 2 of the DPDP migrations (00006_pii_encrypt.go) encrypts existing
	// rows, so the personal-data keys must be installed first.
	if err := cfg.InstallPIIKeys(); err != nil {
		return err
	}

	// Migrations use the session-mode connection: schema changes can misbehave
	// through the transaction pooler.
	url := cfg.DatabaseURL
	if url == "" {
		url = cfg.PooledURL
	}

	db, err := sql.Open("pgx", url)
	if err != nil {
		return err
	}
	defer db.Close()

	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}

	command := "up"
	if len(os.Args) > 1 {
		command = os.Args[1]
	}
	// "." is the folder inside the embedded filesystem.
	return goose.RunContext(context.Background(), command, db, ".")
}
