package models

// Project is a row of the `projects` table: one residential development.
type Project struct {
	ID         string  `db:"id" json:"id"` // slug, e.g. "project-1"
	Name       string  `db:"name" json:"name"`
	ReraNumber string  `db:"rera_number" json:"reraNumber"`
	Locality   string  `db:"locality" json:"locality"`
	Address    string  `db:"address" json:"address"`
	Latitude   float64 `db:"latitude" json:"latitude"` // location pin for site visits
	Longitude  float64 `db:"longitude" json:"longitude"`
}
