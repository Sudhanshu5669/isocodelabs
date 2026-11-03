package httpapi

import (
	"errors"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"leadflow/internal/auth"
	"leadflow/internal/store"
)

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// validate checks the input before we touch the database.
func (l LoginRequest) Validate() string {
	if len(l.Email) == 0 || len(l.Email) > 254 {
		return "Enter a valid email address."
	}
	// ParseAddress also accepts "Bob <bob@x.com>"; we want the bare address only.
	if a, err := mail.ParseAddress(l.Email); err != nil || a.Address != l.Email {
		return "Enter a valid email address."
	}
	if len(l.Password) == 0 {
		return "Enter your password."
	}
	// bcrypt only reads the first 72 bytes, so a longer password could match
	// a different one that shares those bytes. Staff passwords are shorter.
	if len(l.Password) > 72 {
		return "Passwords are at most 72 characters."
	}
	return ""
}

// login checks the email and password, then starts a session.
//
// Three defences against password guessing:
//   - at most 10 attempts a minute from one client IP (s.loginLimiter);
//   - at most 5 wrong passwords per 15 minutes for one account, from anywhere
//     (s.loginFails), so rotating IPs does not help either;
//   - every failure takes the same time and gives the same message, so the
//     response does not reveal whether an email exists or is disabled.
func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	if !s.loginLimiter.Allow(ClientIP(r, s.cfg.TrustedProxyHeader)) {
		writeError(w, http.StatusTooManyRequests, "Too many attempts. Please wait a minute and try again.")
		return
	}

	var req LoginRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if msg := req.Validate(); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}

	account := strings.ToLower(req.Email)
	if s.loginFails.Blocked(account) {
		// Note: someone who knows an email can lock that person out for up to
		// 15 minutes. That is the accepted price of stopping guessing.
		writeError(w, http.StatusTooManyRequests, "Too many failed attempts for this account. Please wait 15 minutes and try again.")
		return
	}

	const genericFailure = "Incorrect email or password."
	rec, err := s.store.UserByEmail(r.Context(), req.Email)
	switch {
	case errors.Is(err, store.ErrNotFound):
		auth.DummyCheck(req.Password) // same time as a real check, so timing does not reveal the email is unknown
		s.loginFails.Allow(account)
		writeError(w, http.StatusUnauthorized, genericFailure)
		return
	case err != nil:
		// A database problem is not a wrong password: say so, and log it.
		s.serverError(w, "login lookup", err)
		return
	}

	// Check the password BEFORE looking at the active flag, so a disabled
	// account takes exactly as long to refuse as a wrong password.
	if !auth.CheckPassword(rec.PasswordHash, req.Password) || !rec.Active {
		s.loginFails.Allow(account)
		writeError(w, http.StatusUnauthorized, genericFailure)
		return
	}
	s.loginFails.Forget(account)

	token, hash, err := auth.NewSessionToken()
	if err != nil {
		s.serverError(w, "session token", err)
		return
	}
	expires := time.Now().Add(auth.SessionTTL)
	if err := s.store.CreateSession(r.Context(), rec.ID, hash, expires); err != nil {
		s.serverError(w, "create session", err)
		return
	}

	s.setSessionCookie(w, token, expires)
	writeJSON(w, http.StatusOK, rec.User)
}

// logout ends the session on the server, then clears the cookie.
//
// If the session cannot be deleted, logout FAILS (500) and the cookie is left
// alone: telling the user they are signed out while the token still works
// would be worse than asking them to try again.
func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(auth.CookieName); err == nil && c.Value != "" {
		if err := s.store.DeleteSession(r.Context(), auth.HashToken(c.Value)); err != nil {
			s.serverError(w, "logout", err)
			return
		}
	}
	s.setSessionCookie(w, "", time.Unix(0, 0))
	w.WriteHeader(http.StatusNoContent)
}

// me returns the signed-in user. RequireAuth has already checked the session.
func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFromContext(r.Context())
	writeJSON(w, http.StatusOK, u)
}

// setSessionCookie sets (or, with an empty token, clears) the login cookie.
// HttpOnly keeps JavaScript away from it, SameSite=Lax limits cross-site use,
// and Secure (HTTPS only) is on unless APP_ENV is exactly "development".
func (s *Server) setSessionCookie(w http.ResponseWriter, token string, expires time.Time) {
	c := &http.Cookie{
		Name:     auth.CookieName,
		Value:    token,
		Path:     "/",
		Expires:  expires,
		HttpOnly: true,
		Secure:   s.cfg.SecureCookies(),
		SameSite: http.SameSiteLaxMode,
	}
	if token == "" {
		c.MaxAge = -1
	}
	http.SetCookie(w, c)
}
