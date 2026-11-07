package store

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"leadflow/internal/auth"
	"leadflow/internal/pii"
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

// UserByEmail finds a user by email, ignoring case, active or not. The caller
// checks Active (after the password, so both failures take the same time).
//
// The email is encrypted in the database, so the lookup uses its blind index
// (an HMAC of the lower-cased address); see internal/pii.
//
// Errors: ErrNotFound.
func (s *Store) UserByEmail(ctx context.Context, email string) (UserRecord, error) {
	var u UserRecord
	err := s.pool.QueryRow(ctx, `
		SELECT id::text, email, name, role, password_hash, active
		FROM users
		WHERE email_bidx = $1`, pii.EmailIndex(email),
	).Scan(&u.ID, pii.Open(pii.Email, &u.Email), pii.Open(pii.Name, &u.Name), &u.Role, &u.PasswordHash, &u.Active)
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
	).Scan(&u.ID, pii.Open(pii.Email, &u.Email), pii.Open(pii.Name, &u.Name), &u.Role)
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

// -----------------------------------------------------------------------------
// User management (sales head only)
// -----------------------------------------------------------------------------

// ErrEmailTaken means another user already has this email address.
var ErrEmailTaken = errors.New("store: email already in use")

// StaffUser is a team member as the head's user list shows them.
type StaffUser struct {
	ID             string    `json:"id"`
	Email          string    `json:"email"`
	Name           string    `json:"name"`
	Role           string    `json:"role"`
	Active         bool      `json:"active"`
	CreatedAt      time.Time `json:"createdAt"`
	OpenLeads      int64     `json:"openLeads"`      // assigned leads not yet visited or lost
	UpcomingVisits int64     `json:"upcomingVisits"` // booked visits still to come
}

// ListUsers lists the whole team, the head first, then by name.
//
// Names are encrypted, so Postgres cannot sort by them; the (small) team is
// sorted here after decryption instead.
func (s *Store) ListUsers(ctx context.Context, now time.Time) ([]StaffUser, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT u.id::text, u.email, u.name, u.role, u.active, u.created_at,
		       (SELECT count(*) FROM leads l WHERE l.assigned_to = u.id AND l.status IN ('new', 'engaged', 'visit_booked')),
		       (SELECT count(*) FROM visits v JOIN visit_slots sl ON sl.id = v.slot_id
		         WHERE sl.executive_id = u.id AND v.status = 'booked' AND sl.starts_at > $1::timestamptz)
		FROM users u`, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []StaffUser{}
	for rows.Next() {
		var u StaffUser
		if err := rows.Scan(&u.ID, pii.Open(pii.Email, &u.Email), pii.Open(pii.Name, &u.Name), &u.Role, &u.Active,
			&u.CreatedAt, &u.OpenLeads, &u.UpcomingVisits); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.SliceStable(out, func(i, j int) bool {
		if hi, hj := out[i].Role == auth.RoleHead, out[j].Role == auth.RoleHead; hi != hj {
			return hi
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out, nil
}

// CreateUser adds a team member. passwordHash must already be a bcrypt hash.
//
// Errors: ErrEmailTaken.
func (s *Store) CreateUser(ctx context.Context, email, name, role, passwordHash string) (StaffUser, error) {
	var u StaffUser
	err := s.pool.QueryRow(ctx, `
		INSERT INTO users (email, email_bidx, name, role, password_hash) VALUES ($1, $2, $3, $4, $5)
		RETURNING id::text, email, name, role, active, created_at`,
		pii.Seal(pii.Email, email), pii.EmailIndex(email), pii.Seal(pii.Name, name), role, passwordHash,
	).Scan(&u.ID, pii.Open(pii.Email, &u.Email), pii.Open(pii.Name, &u.Name), &u.Role, &u.Active, &u.CreatedAt)
	if constraintName(err) == "users_email_key" {
		return StaffUser{}, ErrEmailTaken
	}
	return u, err
}

// DeactivateResult tells the head what still needs attention after a user is
// switched off.
type DeactivateResult struct {
	CancelledOpenSlots int64 `json:"cancelledOpenSlots"` // their free slots, removed so buyers cannot pick them
	BookedVisits       int64 `json:"bookedVisits"`       // upcoming visits the head must move to someone else
	AssignedLeads      int64 `json:"assignedLeads"`      // open leads the head should reassign
}

// SetUserActive switches a user on or off. Switching off signs them out
// everywhere at once and withdraws their free future slots.
//
// Errors: ErrNotFound.
func (s *Store) SetUserActive(ctx context.Context, now time.Time, userID string, active bool) (DeactivateResult, error) {
	var res DeactivateResult
	err := s.withTx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE users SET active = $2 WHERE id = $1::uuid`, userID, active)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		if active {
			return nil
		}
		if _, err := tx.Exec(ctx, `DELETE FROM sessions WHERE user_id = $1::uuid`, userID); err != nil {
			return err
		}
		tag, err = tx.Exec(ctx, `
			UPDATE visit_slots SET status = 'cancelled'
			WHERE executive_id = $1::uuid AND status = 'open' AND starts_at > $2::timestamptz`, userID, now)
		if err != nil {
			return err
		}
		res.CancelledOpenSlots = tag.RowsAffected()
		if err := tx.QueryRow(ctx, `
			SELECT count(*) FROM visits v JOIN visit_slots sl ON sl.id = v.slot_id
			WHERE sl.executive_id = $1::uuid AND v.status = 'booked' AND sl.starts_at > $2::timestamptz`,
			userID, now).Scan(&res.BookedVisits); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `
			SELECT count(*) FROM leads WHERE assigned_to = $1::uuid AND status IN ('new', 'engaged', 'visit_booked')`,
			userID).Scan(&res.AssignedLeads)
	})
	return res, err
}

// SetPassword replaces a user's password and signs them out everywhere.
//
// Errors: ErrNotFound.
func (s *Store) SetPassword(ctx context.Context, userID, passwordHash string) error {
	return s.withTx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE users SET password_hash = $2 WHERE id = $1::uuid`, userID, passwordHash)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		_, err = tx.Exec(ctx, `DELETE FROM sessions WHERE user_id = $1::uuid`, userID)
		return err
	})
}
