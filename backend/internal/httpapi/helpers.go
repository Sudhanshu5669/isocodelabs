package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
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

// securityHeaders adds protective headers to every response. With https on,
// HSTS tells browsers to use HTTPS for this site from now on.
func securityHeaders(https bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("X-Frame-Options", "DENY")
			h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
			if https {
				h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
			}
			next.ServeHTTP(w, r)
		})
	}
}

// apiHeaders applies to every /api response. They carry personal data (lead
// names and phone numbers) and session state, so no browser or proxy may cache
// them, and as plain JSON they may load nothing and run in no frame. (The page
// that serves the React app gets its own policy when Go serves it, Phase 8.)
func apiHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Cache-Control", "no-store")
		h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		next.ServeHTTP(w, r)
	})
}

// SameOrigin blocks cross-site requests that change data (a CSRF defence).
// Browsers send an Origin header on such requests; if it is present it must
// match the host we are serving, or be on the explicit allow-list.
func SameOrigin(allowed []string) func(http.Handler) http.Handler {
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

// ClientIP is the address rate limits are counted against.
//
// By default it is the address of the TCP connection, which a client cannot
// fake. Behind a hosting platform's proxy every connection comes from the
// proxy, so the platform's own header must be named in TRUSTED_PROXY_HEADER
// (for example Fly-Client-IP). Headers are NEVER trusted otherwise: anyone can
// send "X-Forwarded-For: 1.2.3.4" to look like a new visitor.
func ClientIP(r *http.Request, trustedHeader string) string {
	if trustedHeader != "" {
		if v := strings.TrimSpace(r.Header.Get(trustedHeader)); v != "" {
			if strings.EqualFold(trustedHeader, "X-Forwarded-For") {
				// The proxy appends the address it saw; earlier entries came
				// from the client and may be invented. Take the last one.
				parts := strings.Split(v, ",")
				v = strings.TrimSpace(parts[len(parts)-1])
			}
			if ip := net.ParseIP(v); ip != nil {
				return ip.String()
			}
		}
	}
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}

// rateLimit rejects a client IP that calls a route too often (429).
func (s *Server) rateLimit(l *IPLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !l.Allow(ClientIP(r, s.cfg.TrustedProxyHeader)) {
				writeError(w, http.StatusTooManyRequests, "Too many requests. Please slow down and try again shortly.")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// IPLimiter limits how often each key (a client IP, or an email address for
// failed logins) may do something. Each key gets a token bucket: `burst`
// tokens, refilled evenly over the window.
type IPLimiter struct {
	mu      sync.Mutex
	clients map[string]*rate.Limiter
	limit   rate.Limit
	burst   int
}

// MaxTrackedKeys bounds memory. When it is reached, keys whose bucket has
// refilled (idle for a full window) are forgotten. Keys still being limited
// are never dropped, so flooding the map cannot reset anyone's limit.
const MaxTrackedKeys = 10000

func NewIPLimiter(perWindow int, window time.Duration) *IPLimiter {
	return &IPLimiter{
		clients: make(map[string]*rate.Limiter),
		limit:   rate.Every(window / time.Duration(perWindow)),
		burst:   perWindow,
	}
}

// allow takes one token for key and reports whether there was one.
func (l *IPLimiter) Allow(key string) bool {
	l.mu.Lock() // only one goroutine may touch the map at a time
	defer l.mu.Unlock()
	lim, ok := l.bucket(key)
	return ok && lim.Allow()
}

// blocked reports whether key has no tokens left, without taking one.
func (l *IPLimiter) Blocked(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	lim, ok := l.clients[key]
	return ok && lim.Tokens() < 1
}

// forget drops a key (after a successful login, for example).
func (l *IPLimiter) Forget(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.clients, key)
}

// bucket returns key's bucket, creating it if needed. ok is false when the
// map is full of keys that are all still being limited; the caller then
// refuses, failing safe. The caller must hold l.mu.
func (l *IPLimiter) bucket(key string) (*rate.Limiter, bool) {
	if lim, ok := l.clients[key]; ok {
		return lim, true
	}
	if len(l.clients) >= MaxTrackedKeys {
		for k, lim := range l.clients {
			if lim.Tokens() >= float64(l.burst) {
				delete(l.clients, k)
			}
		}
		if len(l.clients) >= MaxTrackedKeys {
			return nil, false
		}
	}
	lim := rate.NewLimiter(l.limit, l.burst)
	l.clients[key] = lim
	return lim, true
}
