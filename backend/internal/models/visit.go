package models

import "time"

// Visit states (visits.status).
const (
	VisitBooked    = "booked"
	VisitVisited   = "visited"
	VisitNoShow    = "no_show"
	VisitCancelled = "cancelled"
)

// Visit is a row of the `visits` table: a lead's booking of one slot.
// The database guarantees a slot holds at most one live visit and a lead has at
// most one upcoming booked visit.
type Visit struct {
	ID        string     `db:"id" json:"id"`
	Code      string     `db:"code" json:"code"` // unguessable token for the buyer's /visit/:code link
	LeadID    string     `db:"lead_id" json:"leadId"`
	SlotID    string     `db:"slot_id" json:"slotId"`
	Status    string     `db:"status" json:"status"`
	Notes     *string    `db:"notes" json:"notes"`
	OutcomeAt *time.Time `db:"outcome_at" json:"outcomeAt"`
	CreatedAt time.Time  `db:"created_at" json:"createdAt"`
	UpdatedAt time.Time  `db:"updated_at" json:"updatedAt"`
}
