package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// writeJSON sends a value as a JSON response.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError sends {"error": "..."} with a status code. Messages here are
// meant for people, so they never include internal details.
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// decodeJSON reads a request body into dst. It rejects oversized bodies,
// unknown fields and trailing junk, which keeps handlers strict by default.
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MiB
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return errors.New("The request could not be read.")
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("The request could not be read.")
	}
	return nil
}

// securityHeaders adds basic protective headers to every response.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		next.ServeHTTP(w, r)
	})
}

// sameOrigin blocks cross-site requests that change data (a CSRF defence).
// Browsers send an Origin header on such requests; if it is present it must
// match the host we are serving, or be on the explicit allow-list.
func sameOrigin(allowed []string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet, http.MethodHead, http.MethodOptions:
				next.ServeHTTP(w, r)
				return
			}
			if origin := r.Header.Get("Origin"); origin != "" && !originAllowed(origin, r.Host, allowed) {
				writeError(w, http.StatusForbidden, "Cross-site request blocked.")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func originAllowed(origin, host string, allowed []string) bool {
	if u, err := url.Parse(origin); err == nil && u.Host == host {
		return true
	}
	for _, a := range allowed {
		if origin == a {
			return true
		}
	}
	return false
}

// rateLimit rejects a client IP that calls a route too often (429).
func rateLimit(l *ipLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !l.allow(r) {
				writeError(w, http.StatusTooManyRequests, "Too many requests. Please slow down and try again shortly.")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// ipLimiter limits how often each client IP may call a route.
type ipLimiter struct {
	mu      sync.Mutex
	clients map[string]*rate.Limiter
	limit   rate.Limit
	burst   int
}

func newIPLimiter(perWindow int, window time.Duration) *ipLimiter {
	return &ipLimiter{
		clients: make(map[string]*rate.Limiter),
		limit:   rate.Every(window / time.Duration(perWindow)),
		burst:   perWindow,
	}
}

// allow reports whether this request's IP is within its limit.
func (l *ipLimiter) allow(r *http.Request) bool {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		ip = r.RemoteAddr
	}

	l.mu.Lock() // only one goroutine may touch the map at a time
	defer l.mu.Unlock()

	if len(l.clients) > 10000 { // crude safety valve against unbounded memory
		l.clients = make(map[string]*rate.Limiter)
	}
	lim, ok := l.clients[ip]
	if !ok {
		lim = rate.NewLimiter(l.limit, l.burst)
		l.clients[ip] = lim
	}
	return lim.Allow()
}
