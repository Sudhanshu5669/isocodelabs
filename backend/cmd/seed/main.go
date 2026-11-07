// Command seed fills the database with sample data.
//
//	go run ./cmd/seed          # only works on an empty database
//	go run ./cmd/seed -reset   # wipe everything first, then reseed
//	go run ./cmd/seed -units   # only add the sample flats (safe on a seeded database)
//
// The staff password comes from SEED_DEMO_PASSWORD. Only with
// APP_ENV=development may it be left out, in which case the documented demo
// password is used. In production:
//   - SEED_DEMO_PASSWORD must be set on purpose (for the assessment demo it is
//     deliberately the published one, so reviewers can sign in);
//   - -reset is refused, so pointing this at the wrong database cannot wipe it;
//   - the password is never printed.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"

	"leadflow/internal/config"
	"leadflow/internal/seed"
	"leadflow/internal/store"
)

// defaultDemoPassword is published in deployment.md for local use.
const defaultDemoPassword = "Demo@2026!"

func main() {
	reset := flag.Bool("reset", false, "wipe existing data before seeding")
	unitsOnly := flag.Bool("units", false, "only add the sample flats for the catalogue")
	flag.Parse()

	if *unitsOnly {
		if err := addUnits(); err != nil {
			fmt.Fprintln(os.Stderr, "seed:", err)
			os.Exit(1)
		}
		return
	}
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
	if err := cfg.InstallPIIKeys(); err != nil {
		return err
	}

	dev := cfg.AppEnv == "development"
	if reset && !dev {
		return errors.New("refusing -reset with APP_ENV=production: it would wipe every lead; reset only a local database")
	}
	password := os.Getenv("SEED_DEMO_PASSWORD")
	if password == "" {
		if !dev {
			return errors.New("set SEED_DEMO_PASSWORD: the built-in demo password is only used with APP_ENV=development")
		}
		password = defaultDemoPassword
	}

	ctx := context.Background()
	url := cfg.DatabaseURL // seeding uses the session-mode connection
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

	if dev && password == defaultDemoPassword {
		fmt.Println("Seeded. Demo logins (password:", password+")")
	} else {
		fmt.Println("Seeded. Staff logins use the password from SEED_DEMO_PASSWORD:")
	}
	fmt.Println("  head@companya.test    (sales head)")
	fmt.Println("  rohan@companya.test   (executive)")
	fmt.Println("  aisha@companya.test   (executive)")
	fmt.Println("  vikram@companya.test  (executive)")
	fmt.Println("  kunal@companya.test   (executive, switched off: cannot sign in)")
	return nil
}

// addUnits adds the catalogue's sample flats without touching anything else.
func addUnits() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	url := cfg.DatabaseURL
	if url == "" {
		url = cfg.PooledURL
	}
	ctx := context.Background()
	pool, err := store.Connect(ctx, url)
	if err != nil {
		return err
	}
	defer pool.Close()
	n, err := seed.AddUnits(ctx, pool)
	if err != nil {
		return err
	}
	fmt.Println("Added", n, "sample flats.")
	return nil
}
