// Package httpapi holds the HTTP layer: routes, handlers and middleware.
// It is named httpapi, not http, so it does not clash with the standard
// library's net/http package.
package httpapi

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"

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
	loginLimiter *ipLimiter
	publicLimit  *ipLimiter       // buyer-facing endpoints that need no login
	now          func() time.Time // replaceable clock, so tests can control time
}

// NewRouter builds the route table.
func NewRouter(cfg config.Config, db *pgxpool.Pool) http.Handler {
	s := &Server{
		db:           db,
		store:        store.New(db),
		cfg:          cfg,
		loginLimiter: newIPLimiter(10, time.Minute), // 10 login attempts per minute per IP
		publicLimit:  newIPLimiter(120, time.Minute),
		now:          time.Now,
	}

	r := chi.NewRouter()
	// Middleware wraps every request, top to bottom.
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer) // turns a panic into a 500 instead of crashing the server
	r.Use(middleware.Timeout(30 * time.Second))
	r.Use(securityHeaders)

	r.Get("/healthz", s.health)

	// Everything the React app calls lives under /api.
	r.Route("/api", func(r chi.Router) {
		r.Use(sameOrigin(cfg.AllowedOrigins)) // CSRF defence for state-changing requests
		r.Use(auth.Authenticate(s.store))     // loads the user from the session cookie, if any

		r.Get("/healthz", s.health)

		r.Route("/auth", func(r chi.Router) {
			r.Post("/login", s.login)
			r.Post("/logout", s.logout)
			r.With(auth.RequireAuth).Get("/me", s.me)
		})

		// Buyer-facing, no login. A visit is reached through its unguessable code.
		r.Group(func(r chi.Router) {
			r.Use(rateLimit(s.publicLimit))
			r.Get("/slots", s.listSlots)
			r.Route("/visits/{code}", func(r chi.Router) {
				r.Get("/", s.getVisit)
				r.Post("/cancel", s.cancelVisitByCode)
				r.Post("/reschedule", s.rescheduleVisitByCode)
			})
		})

		// Staff only.
		r.Route("/staff", func(r chi.Router) {
			r.Use(auth.RequireAuth)
			r.Post("/visits", s.staffBookVisit)
			r.Post("/visits/{id}/cancel", s.staffCancelVisit)
			r.Post("/visits/{id}/outcome", s.staffVisitOutcome)

			// Leads. Executives see their own; the head sees all (enforced in the store).
			r.Get("/leads", s.listLeads)
			r.Post("/leads", s.createLead)
			r.Post("/leads/import", s.importLeads)
			r.Get("/leads/{id}", s.getLead)

			// Only the sales head manages slots, hands out leads and settles duplicates.
			head := auth.RequireRole(auth.RoleHead)
			r.With(head).Post("/slots", s.createSlot)
			r.With(head).Delete("/slots/{id}", s.deleteSlot)
			r.With(head).Post("/leads/{id}/assign", s.assignLead)
			r.With(head).Post("/leads/{id}/duplicate", s.resolveDuplicate)
		})
	})
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
