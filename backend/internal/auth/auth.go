// Package auth handles passwords, session tokens and who-may-do-what.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const (
	// CookieName is the name of the session cookie.
	CookieName = "session"
	// SessionTTL is how long a login lasts.
	SessionTTL = 12 * time.Hour

	RoleExecutive = "executive"
	RoleHead      = "head"
)

// ErrNoSession means the token is unknown or expired.
var ErrNoSession = errors.New("auth: no valid session")

// User is the signed-in staff member. It never contains the password hash.
type User struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
	Role  string `json:"role"`
}

// SessionStore is what the middleware needs from the database.
//
// Go lesson: an interface lists methods. Any type that has those methods
// satisfies it automatically, with no "implements" keyword. The real database
// store satisfies this, and so does the tiny fake we use in tests.
type SessionStore interface {
	UserBySession(ctx context.Context, tokenHash []byte) (User, error)
}

// NewSessionToken makes a random token for the browser cookie plus the hash we
// keep in the database. Only the hash is stored.
func NewSessionToken() (token string, hash []byte, err error) {
	raw := make([]byte, 32) // 256 bits of randomness
	if _, err := rand.Read(raw); err != nil {
		return "", nil, err
	}
	token = base64.RawURLEncoding.EncodeToString(raw)
	return token, HashToken(token), nil
}

// HashToken is the one-way hash used to look a token up.
func HashToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

// HashPassword turns a password into a bcrypt hash (salted, deliberately slow).
func HashPassword(password string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(b), err
}

// CheckPassword reports whether the password matches the hash.
func CheckPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// dummyHash is compared against when the email does not exist, so a failed
// login takes about the same time either way. That stops attackers from
// discovering which emails are registered by timing the response.
var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("not-a-real-password"), bcrypt.DefaultCost)

// DummyCheck burns the same time as a real password check.
func DummyCheck(password string) {
	_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(password))
}
