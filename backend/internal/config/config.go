// Package config reads settings from environment variables.
//
// Go lesson: a "package" is a folder of .go files that share one namespace.
// Names that start with a Capital letter (Config, Load) are exported, meaning
// other packages can use them. Lowercase names stay private to the package.
package config

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

// Config holds every setting the server needs. A struct is Go's way of
// grouping related values, similar to an object or a class without methods.
type Config struct {
	Port        string // HTTP port the server listens on
	DatabaseURL string // Supabase session-mode connection (used for migrations, seeding and tests)
	PooledURL   string // Supabase transaction-mode pooler (used by the running app)
	AppEnv      string // "development" or "production" (the default)

	// The AI model behind the chat assistant. AI_PROVIDER alone decides which
	// company is used (see internal/ai/providers); AI_MODEL picks the model,
	// empty meaning that provider's default.
	AIProvider    string // "gemini" (the default), "openai" or "offline"
	AIModel       string
	GeminiAPIKey  string
	OpenAIAPIKey  string
	OpenAIBaseURL string // empty means OpenAI itself; set it for an OpenAI-compatible API

	// TrustedProxyHeader names the ONE header the hosting platform's proxy sets
	// to the real client IP (e.g. "Fly-Client-IP"). Empty means no proxy: the
	// connection's own address is used and every forwarding header is ignored.
	TrustedProxyHeader string

	// AllowedOrigins lists extra browser origins that may send state-changing
	// requests. Needed in development, where the React dev server runs on its
	// own port. Empty in production, where one server serves everything.
	AllowedOrigins []string

	// PublicURL is where buyers reach the site, e.g. https://visits.example.com.
	// Reminder messages use it to link to the buyer's visit page. Optional.
	PublicURL string

	// StaticDir is the folder holding the built React app (frontend/dist).
	// When set, the server serves the whole site; empty means API only.
	StaticDir string

	// InboundWebhookSecret protects the endpoint the messaging provider calls
	// with buyer replies (e.g. "STOP"). Empty switches the endpoint off.
	InboundWebhookSecret string

	// RemindersOff stops the background reminder sender (REMINDERS=off), for
	// example on a second server that should only answer requests.
	RemindersOff bool
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
		Port:        getenv("PORT", "8080"),
		DatabaseURL: os.Getenv("DATABASE_URL"),
		PooledURL:   os.Getenv("DATABASE_URL_POOLED"),

		AIProvider:    strings.ToLower(getenv("AI_PROVIDER", "gemini")),
		AIModel:       strings.TrimSpace(os.Getenv("AI_MODEL")),
		GeminiAPIKey:  os.Getenv("GEMINI_API_KEY"),
		OpenAIAPIKey:  os.Getenv("OPENAI_API_KEY"),
		OpenAIBaseURL: os.Getenv("OPENAI_BASE_URL"),

		PublicURL:            strings.TrimRight(strings.TrimSpace(os.Getenv("PUBLIC_URL")), "/"),
		StaticDir:            strings.TrimSpace(os.Getenv("STATIC_DIR")),
		InboundWebhookSecret: os.Getenv("INBOUND_WEBHOOK_SECRET"),
		RemindersOff:         strings.EqualFold(os.Getenv("REMINDERS"), "off"),

		TrustedProxyHeader: strings.TrimSpace(os.Getenv("TRUSTED_PROXY_HEADER")),
		// Unset means production: forgetting the variable must never switch
		// security off. Local development sets APP_ENV=development on purpose.
		AppEnv: getenv("APP_ENV", "production"),
	}

	if cfg.AppEnv != "development" && cfg.AppEnv != "production" {
		return Config{}, fmt.Errorf("config: APP_ENV must be development or production, not %q", cfg.AppEnv)
	}

	switch cfg.AIProvider {
	case "gemini", "openai", "offline":
	default:
		return Config{}, fmt.Errorf("config: AI_PROVIDER must be gemini, openai or offline, not %q", cfg.AIProvider)
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

// SecureCookies reports whether cookies get the Secure flag (HTTPS only). It
// is on everywhere except an explicit local development setup.
func (c Config) SecureCookies() bool {
	return c.AppEnv != "development"
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
