package models

import "time"

// Reminder kinds (reminders.kind): sent 1 day and 2 hours before a visit.
const (
	ReminderDayBefore      = "day_before"
	ReminderTwoHoursBefore = "two_hours_before"
)

// Reminder states (reminders.status).
const (
	ReminderQueued  = "queued"
	ReminderSent    = "sent"
	ReminderSkipped = "skipped"
	ReminderFailed  = "failed"
)

// Reminder is a row of the `reminders` table: one scheduled message for a visit.
type Reminder struct {
	ID         string     `db:"id" json:"id"`
	VisitID    string     `db:"visit_id" json:"visitId"`
	Kind       string     `db:"kind" json:"kind"`
	DueAt      time.Time  `db:"due_at" json:"dueAt"`
	Status     string     `db:"status" json:"status"`
	SkipReason *string    `db:"skip_reason" json:"skipReason"` // e.g. "opted_out"
	Body       *string    `db:"body" json:"body"`              // rendered text, includes the RERA number
	SentAt     *time.Time `db:"sent_at" json:"sentAt"`
}
