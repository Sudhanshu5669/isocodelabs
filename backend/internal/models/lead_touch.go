package models

import "time"

// LeadTouch is a row of the `lead_touches` table: one enquiry from a person.
// A lead keeps its first source; every later enquiry is recorded here.
type LeadTouch struct {
	ID        string    `db:"id" json:"id"`
	LeadID    string    `db:"lead_id" json:"leadId"`
	Source    string    `db:"source" json:"source"`
	ProjectID *string   `db:"project_id" json:"projectId"`
	Note      *string   `db:"note" json:"note"`
	CreatedAt time.Time `db:"created_at" json:"createdAt"`
}
