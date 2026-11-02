package models

import "time"

// Slot states (visit_slots.status).
const (
	SlotOpen      = "open"
	SlotBooked    = "booked"
	SlotCancelled = "cancelled"
)

// VisitSlot is a row of the `visit_slots` table: a one-hour window an executive
// can show a project. The database prevents one executive from owning two
// overlapping slots.
type VisitSlot struct {
	ID          string    `db:"id" json:"id"`
	ProjectID   string    `db:"project_id" json:"projectId"`
	ExecutiveID string    `db:"executive_id" json:"executiveId"`
	StartsAt    time.Time `db:"starts_at" json:"startsAt"`
	EndsAt      time.Time `db:"ends_at" json:"endsAt"`
	Status      string    `db:"status" json:"status"`
}
