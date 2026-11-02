// Command seed fills the database with sample data.
//
//	go run ./cmd/seed          # only works on an empty database
//	go run ./cmd/seed -reset   # wipe everything first, then reseed
//
// The demo password for staff accounts comes from SEED_DEMO_PASSWORD
// (a default is used for local development).
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"leadflow/internal/config"
	"leadflow/internal/seed"
	"leadflow/internal/store"
)

func main() {
	reset := flag.Bool("reset", false, "wipe existing data before seeding")
	flag.Parse()

	if err := run(*reset); err != nil {
		fmt.Fprintln(os.Stderr, "seed:", err)
		os.Exit(1)
	}
}

func run(reset bool) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	password := os.Getenv("SEED_DEMO_PASSWORD")
	if password == "" {
		password = "Demo@2026!"
	}

	ctx := context.Background()
	url := cfg.DatabaseURL // seeding uses the direct connection
	if url == "" {
		url = cfg.PooledURL
	}
	pool, err := store.Connect(ctx, url)
	if err != nil {
		return err
	}
	defer pool.Close()

	if err := seed.Run(ctx, pool, seed.Options{Password: password, Reset: reset}); err != nil {
		return err
	}

	fmt.Println("Seeded. Demo logins (password:", password+")")
	fmt.Println("  head@companya.test    (sales head)")
	fmt.Println("  rohan@companya.test   (executive)")
	fmt.Println("  aisha@companya.test   (executive)")
	fmt.Println("  vikram@companya.test  (executive)")
	return nil
}
