// Command server is the entry point. `go run ./cmd/server` starts the API.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"leadflow/internal/ai"
	"leadflow/internal/ai/providers"
	"leadflow/internal/config"
	"leadflow/internal/httpapi"
	"leadflow/internal/reminders"
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
	// Personal data is encrypted in the database driver (internal/pii); the
	// server will not start without the keys.
	if err := cfg.InstallPIIKeys(); err != nil {
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

	// The chat assistant's model, picked by AI_PROVIDER and AI_MODEL. Without
	// a key the chat still runs, on fixed replies that hand every buyer to a
	// person.
	var llm ai.ChatCompletion
	if m, err := providers.New(cfg); err != nil {
		logger.Warn("chat assistant running without a model", "error", err)
	} else {
		llm = m
		logger.Info("chat assistant model", "provider", m.Provider(), "model", m.Model())
	}

	// background tracks work that outlives a request (handoff summaries).
	var background sync.WaitGroup
	defer background.Wait() // let it finish before the database pool closes

	// The reminder sender runs in the background until shutdown. Deferred calls
	// run last-in first-out, so stopWorkers (below) runs before background.Wait.
	workerCtx, stopWorkers := context.WithCancel(ctx)
	defer stopWorkers()
	if cfg.RemindersOff {
		logger.Info("reminder sender is off (REMINDERS=off)")
	} else {
		worker := &reminders.Worker{Store: store.New(db), Messenger: reminders.LogMessenger{}, PublicURL: cfg.PublicURL}
		background.Add(1)
		go func() {
			defer background.Done()
			worker.Run(workerCtx)
		}()
		logger.Info("reminder sender started", "messenger", "mock (log only)")
	}
	if cfg.StaticDir != "" {
		logger.Info("serving the web app", "dir", cfg.StaticDir)
	}

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           httpapi.NewRouter(cfg, db, llm, &background),
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
