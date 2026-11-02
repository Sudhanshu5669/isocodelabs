package httpapi

import (
	"net/http"
	"net/mail"
	"time"

	"leadflow/internal/auth"
)

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// validate checks the input before we touch the database.
// bcrypt only uses the first 72 bytes, so longer passwords are rejected.
func (l loginRequest) validate() string {
	if len(l.Email) == 0 || len(l.Email) > 254 {
		return "Enter a valid email address."
	}
	if _, err := mail.ParseAddress(l.Email); err != nil {
		return "Enter a valid email address."
	}
	if len(l.Password) == 0 || len(l.Password) > 72 {
		return "Enter your password."
	}
	return ""
}

// login checks the email and password, then starts a session.
func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	if !s.loginLimiter.allow(r) {
		writeError(w, http.StatusTooManyRequests, "Too many attempts. Please wait a minute and try again.")
		return
	}

	var req loginRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if msg := req.validate(); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}

	const genericFailure = "Incorrect email or password."

	rec, err := s.store.UserByEmail(r.Context(), req.Email)
	if err != nil {
		auth.DummyCheck(req.Password) // keep timing the same whether or not the email exists
		writeError(w, http.StatusUnauthorized, genericFailure)
		return
	}
	if !rec.Active || !auth.CheckPassword(rec.PasswordHash, req.Password) {
		writeError(w, http.StatusUnauthorized, genericFailure)
		return
	}

	token, hash, err := auth.NewSessionToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Something went wrong. Please try again.")
		return
	}
	expires := time.Now().Add(auth.SessionTTL)
	if err := s.store.CreateSession(r.Context(), rec.ID, hash, expires); err != nil {
		writeError(w, http.StatusInternalServerError, "Something went wrong. Please try again.")
		return
	}

	s.setSessionCookie(w, token, expires)
	writeJSON(w, http.StatusOK, rec.User)
}

// logout ends the session and clears the cookie. It always succeeds, so a
// stale cookie never gets stuck in the browser.
func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(auth.CookieName); err == nil {
		_ = s.store.DeleteSession(r.Context(), auth.HashToken(c.Value))
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
// and Secure keeps it to HTTPS in production.
func (s *Server) setSessionCookie(w http.ResponseWriter, token string, expires time.Time) {
	c := &http.Cookie{
		Name:     auth.CookieName,
		Value:    token,
		Path:     "/",
		Expires:  expires,
		HttpOnly: true,
		Secure:   s.cfg.AppEnv == "production",
		SameSite: http.SameSiteLaxMode,
	}
	if token == "" {
		c.MaxAge = -1
	}
	http.SetCookie(w, c)
}
