package models

import "time"

// Role values allowed in users.role.
const (
	RoleExecutive = "executive"
	RoleHead      = "head"
)

// User is a row of the `users` table: a member of the sales team.
type User struct {
	ID           string    `db:"id" json:"id"`
	Email        string    `db:"email" json:"email"`
	Name         string    `db:"name" json:"name"`
	Role         string    `db:"role" json:"role"`
	PasswordHash string    `db:"password_hash" json:"-"` // never sent to the browser
	Active       bool      `db:"active" json:"active"`
	CreatedAt    time.Time `db:"created_at" json:"createdAt"`
}
