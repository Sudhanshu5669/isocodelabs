package models

import "time"

// Session is a row of the `sessions` table: one signed-in browser.
// Only a hash of the cookie token is stored, never the token itself.
type Session struct {
	TokenHash []byte    `db:"token_hash" json:"-"`
	UserID    string    `db:"user_id" json:"-"`
	ExpiresAt time.Time `db:"expires_at" json:"-"`
	CreatedAt time.Time `db:"created_at" json:"-"`
}
