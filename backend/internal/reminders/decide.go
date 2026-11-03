package reminders

import (
	"time"

	"leadflow/internal/models"
)

// Why a due reminder was not sent (stored in reminders.skip_reason).
const (
	SkipVisitNotBooked = "visit_not_booked" // cancelled, or already marked visited / no-show
	SkipOptedOut       = "opted_out"        // no consent, or the buyer opted out
	SkipTooLate        = "too_late"         // e.g. the server was down when it was due
	FailedSend         = "send_failed"
)

// Decide returns why a due reminder must NOT be sent, or "" if it may be.
//
// Everything is checked again at send time, because a lot can change between
// booking and the reminder: the buyer may have cancelled, or opted out.
func Decide(now time.Time, kind, visitStatus string, consent, optedOut bool, startsAt time.Time) string {
	switch {
	case visitStatus != models.VisitBooked:
		return SkipVisitNotBooked
	case !consent || optedOut:
		return SkipOptedOut
	case !startsAt.After(now):
		return SkipTooLate
	case kind == models.ReminderDayBefore && !startsAt.Add(-2*time.Hour).After(now):
		// The "day before" one is so late that the 2-hour one is due too:
		// send only that one, not two messages back to back.
		return SkipTooLate
	}
	return ""
}
