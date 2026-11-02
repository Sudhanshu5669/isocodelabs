// Package config reads settings from environment variables.
//
// Go lesson: a "package" is a folder of .go files that share one namespace.
// Names that start with a Capital letter (Config, Load) are exported, meaning
// other packages can use them. Lowercase names stay private to the package.
package config

import (
	"errors"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

// Config holds every setting the server needs. A struct is Go's way of
// grouping related values, similar to an object or a class without methods.
type Config struct {
	Port         string // HTTP port the server listens on
	DatabaseURL  string // direct Neon connection (used for migrations and seeding)
	PooledURL    string // pooled Neon connection (used by the running app)
	GeminiAPIKey string // Google Gemini API key for the chat assistant
	AppEnv       string // "development" or "production"

	// AllowedOrigins lists extra browser origins that may send state-changing
	// requests. Needed in development, where the React dev server runs on its
	// own port. Empty in production, where one server serves everything.
	AllowedOrigins []string
}

// Load reads the .env file (if there is one) and then the real environment.
//
// Go lesson: functions can return more than one value. By convention the last
// value is an `error`, and the caller checks `if err != nil` right away.
// There are no exceptions in Go; errors are ordinary values.
func Load() (Config, error) {
	// In production there is no .env file, the platform injects real
	// environment variables, so a missing file is not an error.
	_ = godotenv.Load()

	cfg := Config{
		Port:         getenv("PORT", "8080"),
		DatabaseURL:  os.Getenv("DATABASE_URL"),
		PooledURL:    os.Getenv("DATABASE_URL_POOLED"),
		GeminiAPIKey: os.Getenv("GEMINI_API_KEY"),
		AppEnv:       getenv("APP_ENV", "development"),
	}

	defaultOrigins := ""
	if cfg.AppEnv != "production" {
		defaultOrigins = "http://localhost:5173"
	}
	for _, o := range strings.Split(getenv("ALLOWED_ORIGINS", defaultOrigins), ",") {
		if o = strings.TrimSpace(o); o != "" {
			cfg.AllowedOrigins = append(cfg.AllowedOrigins, o)
		}
	}

	if cfg.DatabaseURL == "" && cfg.PooledURL == "" {
		return Config{}, errors.New("config: set DATABASE_URL or DATABASE_URL_POOLED")
	}
	return cfg, nil
}

// RuntimeDBURL is the connection string the running app should use.
// The pooled URL is preferred because it copes better with many short requests.
func (c Config) RuntimeDBURL() string {
	if c.PooledURL != "" {
		return c.PooledURL
	}
	return c.DatabaseURL
}

// getenv returns the variable's value, or fallback when it is empty.
// It starts with a lowercase letter, so it is private to this package.
func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
