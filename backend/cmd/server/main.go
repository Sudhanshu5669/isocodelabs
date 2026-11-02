// Command server is the entry point. `go run ./cmd/server` starts the API.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"leadflow/internal/config"
	"leadflow/internal/httpapi"
	"leadflow/internal/store"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	if err := run(logger); err != nil {
		logger.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

// run holds the real startup logic so main can stay tiny and every failure
// is handled in one place.
func run(logger *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	// ctx is cancelled when the process gets Ctrl+C or a termination signal.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	db, err := store.Connect(ctx, cfg.RuntimeDBURL())
	if err != nil {
		return err
	}
	defer db.Close()
	logger.Info("database connected")

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           httpapi.NewRouter(cfg, db),
		ReadHeaderTimeout: 5 * time.Second,
	}

	// Run the server in a goroutine (a lightweight thread) so this function can
	// wait for a shutdown signal at the same time.
	errCh := make(chan error, 1)
	go func() {
		logger.Info("listening", "addr", srv.Addr, "env", cfg.AppEnv)
		errCh <- srv.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
		logger.Info("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx) // lets in-flight requests finish
	}
	return nil
}
