package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"leadflow/internal/auth"
)

// ErrNotFound is returned when a row does not exist.
var ErrNotFound = errors.New("store: not found")

// UserRecord is a user plus the secrets needed to log in. It stays inside the
// server and is never sent to the browser.
type UserRecord struct {
	auth.User
	PasswordHash string
	Active       bool
}

// UserByEmail finds an active user by email, ignoring case.
func (s *Store) UserByEmail(ctx context.Context, email string) (UserRecord, error) {
	var u UserRecord
	err := s.pool.QueryRow(ctx, `
		SELECT id::text, email, name, role, password_hash, active
		FROM users
		WHERE lower(email) = lower($1)`, email,
	).Scan(&u.ID, &u.Email, &u.Name, &u.Role, &u.PasswordHash, &u.Active)
	if errors.Is(err, pgx.ErrNoRows) {
		return UserRecord{}, ErrNotFound
	}
	return u, err
}

// CreateSession saves a new session for the user.
func (s *Store) CreateSession(ctx context.Context, userID string, tokenHash []byte, expires time.Time) error {
	// Housekeeping: drop expired sessions whenever someone logs in.
	if _, err := s.pool.Exec(ctx, `DELETE FROM sessions WHERE expires_at < now()`); err != nil {
		return err
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO sessions (token_hash, user_id, expires_at)
		VALUES ($1, $2::uuid, $3)`, tokenHash, userID, expires)
	return err
}

// UserBySession returns the user for a live session. This is what makes
// *Store satisfy the auth.SessionStore interface.
func (s *Store) UserBySession(ctx context.Context, tokenHash []byte) (auth.User, error) {
	var u auth.User
	err := s.pool.QueryRow(ctx, `
		SELECT u.id::text, u.email, u.name, u.role
		FROM sessions s
		JOIN users u ON u.id = s.user_id
		WHERE s.token_hash = $1 AND s.expires_at > now() AND u.active`, tokenHash,
	).Scan(&u.ID, &u.Email, &u.Name, &u.Role)
	if errors.Is(err, pgx.ErrNoRows) {
		return auth.User{}, auth.ErrNoSession
	}
	return u, err
}

// DeleteSession logs a session out.
func (s *Store) DeleteSession(ctx context.Context, tokenHash []byte) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM sessions WHERE token_hash = $1`, tokenHash)
	return err
}
