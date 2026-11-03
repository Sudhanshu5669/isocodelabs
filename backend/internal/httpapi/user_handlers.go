package httpapi

import (
	"errors"
	"net/http"
	"net/mail"
	"strings"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"

	"leadflow/internal/auth"
	"leadflow/internal/models"
	"leadflow/internal/store"
)

// User management, sales head only. There is no self sign-up: the head adds
// each executive and gives them a first password to change.

const (
	minPasswordLen = 10
	maxPasswordLen = 72 // bcrypt reads only the first 72 bytes
)

// passwordProblem returns why a new password is not acceptable, or "".
func passwordProblem(pw string) string {
	switch {
	case len(pw) > maxPasswordLen:
		return "Passwords are at most 72 characters."
	case utf8.RuneCountInString(pw) < minPasswordLen:
		return "Passwords need at least 10 characters."
	case strings.TrimSpace(pw) == "":
		return "Passwords cannot be only spaces."
	}
	return ""
}

func (s *Server) listUsers(w http.ResponseWriter, r *http.Request) {
	users, err := s.store.ListUsers(r.Context(), s.now())
	if err != nil {
		s.serverError(w, "list users", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": users})
}

type createUserRequest struct {
	Email    string `json:"email"`
	Name     string `json:"name"`
	Role     string `json:"role"` // "executive" or "head"
	Password string `json:"password"`
}

// createUser adds a team member. POST /api/staff/users
func (s *Server) createUser(w http.ResponseWriter, r *http.Request) {
	var req createUserRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	req.Email, req.Name = strings.TrimSpace(req.Email), strings.TrimSpace(req.Name)
	if a, err := mail.ParseAddress(req.Email); err != nil || a.Address != req.Email || len(req.Email) > 254 {
		writeError(w, http.StatusBadRequest, "Enter a valid email address.")
		return
	}
	if req.Name == "" || utf8.RuneCountInString(req.Name) > 80 {
		writeError(w, http.StatusBadRequest, "Enter a name of up to 80 characters.")
		return
	}
	if req.Role != models.RoleExecutive && req.Role != models.RoleHead {
		writeError(w, http.StatusBadRequest, "Role must be executive or head.")
		return
	}
	if msg := passwordProblem(req.Password); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		s.serverError(w, "hash password", err)
		return
	}
	u, err := s.store.CreateUser(r.Context(), strings.ToLower(req.Email), req.Name, req.Role, hash)
	if errors.Is(err, store.ErrEmailTaken) {
		writeError(w, http.StatusConflict, "Someone already uses that email address.")
		return
	}
	if err != nil {
		s.serverError(w, "create user", err)
		return
	}
	writeJSON(w, http.StatusCreated, u)
}

type activeRequest struct {
	Active bool `json:"active"`
}

// setUserActive switches a user on or off. POST /api/staff/users/{id}/active
// Switching someone off signs them out at once and withdraws their free slots;
// the reply says what still needs reassigning.
func (s *Server) setUserActive(w http.ResponseWriter, r *http.Request) {
	me, _ := auth.UserFromContext(r.Context())
	id := chi.URLParam(r, "id")
	if !reUUID.MatchString(id) {
		writeError(w, http.StatusNotFound, "We could not find that user.")
		return
	}
	var req activeRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if id == me.ID && !req.Active {
		writeError(w, http.StatusBadRequest, "You cannot switch off your own account.")
		return
	}
	res, err := s.store.SetUserActive(r.Context(), s.now(), id, req.Active)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "We could not find that user.")
		return
	}
	if err != nil {
		s.serverError(w, "set user active", err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

type passwordRequest struct {
	Password string `json:"password"`
}

// resetPassword sets a new password for a user and signs them out everywhere.
// POST /api/staff/users/{id}/password
func (s *Server) resetPassword(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !reUUID.MatchString(id) {
		writeError(w, http.StatusNotFound, "We could not find that user.")
		return
	}
	var req passwordRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if msg := passwordProblem(req.Password); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		s.serverError(w, "hash password", err)
		return
	}
	err = s.store.SetPassword(r.Context(), id, hash)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "We could not find that user.")
		return
	}
	if err != nil {
		s.serverError(w, "reset password", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
