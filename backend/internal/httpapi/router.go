// Package httpapi holds the HTTP layer: routes, handlers and middleware.
// It is named httpapi, not http, so it does not clash with the standard
// library's net/http package.
package httpapi

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"

	"leadflow/internal/ai"
	"leadflow/internal/ai/assistant"
	"leadflow/internal/ai/providers"
	"leadflow/internal/auth"
	"leadflow/internal/config"
	"leadflow/internal/store"
)

// Server bundles the things handlers need. Handlers are methods on it, so they
// can reach the database without using global variables.
type Server struct {
	db           *pgxpool.Pool
	store        *store.Store
	cfg          config.Config
	loginLimiter *IPLimiter       // login attempts per client IP
	loginFails   *IPLimiter       // failed logins per email address, wherever they come from
	publicLimit  *IPLimiter       // buyer-facing endpoints that need no login
	chatStart    *IPLimiter       // new chats create leads, so they are limited harder
	chatSend     *IPLimiter       // buyer messages cost a model call each
	now          func() time.Time // replaceable clock, so tests can control time

	llm       ai.ChatCompletion // the language model (picked by AI_PROVIDER; a fake in tests)
	agent     *assistant.Agent  // the chat assistant built on llm
	chatLocks *chatLocks        // one message at a time per chat

	// background counts work still running after a response was sent (the
	// handoff summary). Wait on it before shutting down.
	background *sync.WaitGroup
}

// NewRouter builds the route table. llm may be nil, in which case the chat
// still works on its fixed fallback replies and hands every buyer to a person.
// background is waited on at shutdown (it may be nil when nobody waits).
func NewRouter(cfg config.Config, db *pgxpool.Pool, llm ai.ChatCompletion, background *sync.WaitGroup) http.Handler {
	if llm == nil {
		llm = providers.Offline{}
	}
	if background == nil {
		background = &sync.WaitGroup{}
	}
	s := &Server{
		db:           db,
		store:        store.New(db),
		cfg:          cfg,
		loginLimiter: NewIPLimiter(10, time.Minute),   // 10 login attempts per minute per IP
		loginFails:   NewIPLimiter(5, 15*time.Minute), // 5 wrong passwords per 15 minutes per account
		publicLimit:  NewIPLimiter(120, time.Minute),
		chatStart:    NewIPLimiter(10, time.Hour),
		chatSend:     NewIPLimiter(20, time.Minute),
		now:          time.Now,
		llm:          llm,
		agent:        &assistant.Agent{Model: llm},
		chatLocks:    &chatLocks{busy: map[string]bool{}},
		background:   background,
	}

	r := chi.NewRouter()
	// Middleware wraps every request, top to bottom.
	r.Use(middleware.RequestID)
	// No middleware.RealIP: it believes X-Forwarded-For from anyone, which
	// would let a client dodge rate limits. See ClientIP in helpers.go.
	r.Use(middleware.Recoverer) // turns a panic into a 500 instead of crashing the server
	r.Use(middleware.Timeout(30 * time.Second))
	r.Use(securityHeaders(cfg.SecureCookies()))

	r.Get("/healthz", s.health)

	// Everything the React app calls lives under /api.
	r.Route("/api", func(r chi.Router) {
		r.Use(apiHeaders)                     // no caching of personal data
		r.Use(SameOrigin(cfg.AllowedOrigins)) // CSRF defence for state-changing requests
		r.Use(auth.Authenticate(s.store))     // loads the user from the session cookie, if any

		r.Get("/healthz", s.health)

		r.Route("/auth", func(r chi.Router) {
			r.Post("/login", s.login)
			r.Post("/logout", s.logout)
			r.With(auth.RequireAuth).Get("/me", s.me)
		})

		// Buyer-facing, no login. A visit is reached through its unguessable code.
		r.Group(func(r chi.Router) {
			r.Use(s.rateLimit(s.publicLimit))
			r.Get("/slots", s.listSlots)
			r.Get("/projects", s.listProjects)
			r.Get("/projects/{id}", s.getProject)
			r.Get("/units", s.listUnits)
			r.Get("/units/{code}", s.getUnit)
			r.Route("/visits/{code}", func(r chi.Router) {
				r.Get("/", s.getVisit)
				r.Post("/cancel", s.cancelVisitByCode)
				r.Post("/reschedule", s.rescheduleVisitByCode)
				r.Post("/opt-out", s.optOutByCode)
			})

			// The AI chat. The buyer is identified by the chat cookie only.
			r.Route("/chat", func(r chi.Router) {
				r.With(s.rateLimit(s.chatStart)).Post("/start", s.startChat)
				r.Get("/", s.getChat)
				r.With(s.rateLimit(s.chatSend)).Post("/messages", s.chatMessage)
				r.With(s.rateLimit(s.chatSend)).Post("/book", s.chatBook)
				r.With(s.rateLimit(s.chatSend)).Post("/handoff", s.chatHandoff)
			})
		})

		// Called by the messaging provider (mocked), not by browsers. A server
		// sends no Origin header, so the CSRF check lets it through; what
		// protects it is the shared secret checked in the handler.
		r.With(s.rateLimit(s.publicLimit)).Post("/webhooks/inbound", s.inboundMessage)

		// Staff only.
		r.Route("/staff", func(r chi.Router) {
			r.Use(auth.RequireAuth)
			r.Get("/visits", s.staffVisits)
			r.Get("/slots", s.staffSlots)
			r.Get("/reminders", s.reminderLog)
			r.Get("/projects/{id}/facts", s.staffFacts)
			r.Post("/visits", s.staffBookVisit)
			r.Post("/visits/{id}/cancel", s.staffCancelVisit)
			r.Post("/visits/{id}/outcome", s.staffVisitOutcome)

			// Leads. Executives see their own; the head sees all (enforced in the store).
			r.Get("/leads", s.listLeads)
			r.Post("/leads", s.createLead)
			r.Post("/leads/import", s.importLeads)
			r.Get("/leads/{id}", s.getLead)
			r.Post("/leads/{id}/status", s.setLeadStatus)
			r.Post("/leads/{id}/opt-out", s.optOutLead)

			// Only the sales head manages slots, hands out leads and settles duplicates.
			head := auth.RequireRole(auth.RoleHead)
			r.With(head).Post("/slots", s.createSlot)
			r.With(head).Delete("/slots/{id}", s.deleteSlot)
			r.With(head).Post("/leads/{id}/assign", s.assignLead)
			r.With(head).Post("/leads/{id}/duplicate", s.resolveDuplicate)
			r.With(head).Get("/chats/flagged", s.listFlaggedChats)
			r.With(head).Post("/chats/{id}/reviewed", s.reviewFlaggedChat)

			// The dashboard and the admin editor (fact sheet, spend, team).
			r.With(head).Get("/dashboard", s.dashboard)
			r.With(head).Put("/projects/{id}/facts/{topic}", s.saveFact)
			r.With(head).Delete("/projects/{id}/facts/{topic}", s.deleteFact)
			r.With(head).Get("/spend", s.listSpend)
			r.With(head).Put("/spend", s.saveSpend)
			r.With(head).Get("/users", s.listUsers)
			r.With(head).Post("/users", s.createUser)
			r.With(head).Post("/users/{id}/active", s.setUserActive)
			r.With(head).Post("/users/{id}/password", s.resetPassword)
		})
	})

	// Everything else is the React app, when the server is set up to serve it.
	if cfg.StaticDir != "" {
		r.Handle("/*", spaHandler(cfg.StaticDir))
	}
	return r
}

// health reports whether the server and the database are reachable.
// Hosting platforms and uptime checks call this.
func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel() // defer runs this line when the function returns

	dbStatus := "ok"
	if err := s.db.Ping(ctx); err != nil {
		dbStatus = "down"
	}

	status := http.StatusOK
	if dbStatus != "ok" {
		status = http.StatusServiceUnavailable
	}
	writeJSON(w, status, map[string]string{
		"status": "up",
		"db":     dbStatus,
		"time":   time.Now().UTC().Format(time.RFC3339),
	})
}
