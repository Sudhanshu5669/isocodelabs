package auth

import (
	"context"
	"encoding/json"
	"net/http"
)

// ctxKey is a private type so no other package can collide with our context key.
type ctxKey struct{}

// UserFromContext returns the signed-in user a handler can rely on after
// Authenticate has run.
func UserFromContext(ctx context.Context) (User, bool) {
	u, ok := ctx.Value(ctxKey{}).(User)
	return u, ok
}

// Authenticate reads the session cookie and, if it is valid, stores the user in
// the request context. It never blocks a request by itself; RequireAuth and
// RequireRole decide that.
//
// Go lesson: middleware is a function that takes a handler and returns a new
// handler that does something before or after calling the original.
func Authenticate(store SessionStore) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if c, err := r.Cookie(CookieName); err == nil && c.Value != "" {
				if u, err := store.UserBySession(r.Context(), HashToken(c.Value)); err == nil {
					r = r.WithContext(context.WithValue(r.Context(), ctxKey{}, u))
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireAuth rejects requests that are not signed in (401).
func RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := UserFromContext(r.Context()); !ok {
			writeError(w, http.StatusUnauthorized, "Please sign in.")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RequireRole allows only the listed roles (403 otherwise). Use it after
// RequireAuth.
func RequireRole(roles ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			u, ok := UserFromContext(r.Context())
			if !ok {
				writeError(w, http.StatusUnauthorized, "Please sign in.")
				return
			}
			for _, role := range roles {
				if u.Role == role {
					next.ServeHTTP(w, r)
					return
				}
			}
			writeError(w, http.StatusForbidden, "You do not have access to this.")
		})
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
