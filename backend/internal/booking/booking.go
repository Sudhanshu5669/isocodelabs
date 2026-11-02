// Package booking holds the pure rules of site-visit booking: how much notice a
// visit needs, when reminders are due, and the errors the booking engine returns.
//
// It has no database code, so every rule here can be tested instantly. The
// database transactions that apply these rules live in package store.
package booking

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	"leadflow/internal/models"
)

// IST is India Standard Time. Slot days ("Saturday at 11:00") are in Ahmedabad time.
var IST = time.FixedZone("IST", 5*3600+1800)

const (
	// MinNotice is how far ahead a slot must start to be bookable.
	// ASSUMPTION: 2 hours, so a visit can still be prepared and the
	// 2-hours-before reminder is meaningful. Not stated in the brief.
	MinNotice = 2 * time.Hour

	MinSlotLength = 15 * time.Minute
	MaxSlotLength = 4 * time.Hour
)

// Errors the booking engine returns. Handlers translate these into friendly
// messages and status codes.
//
// Go lesson: errors.New makes a fixed error value. Callers compare with
// errors.Is(err, booking.ErrSlotUnavailable) instead of matching on text.
var (
	ErrSlotUnavailable    = errors.New("booking: slot is not available")
	ErrLeadAlreadyBooked  = errors.New("booking: lead already has a booked visit")
	ErrLeadNotFound       = errors.New("booking: lead not found")
	ErrVisitNotFound      = errors.New("booking: visit not found")
	ErrVisitNotChangeable = errors.New("booking: visit can no longer be changed")
	ErrVisitNotMarkable   = errors.New("booking: visit cannot be marked yet")
	ErrSameSlot           = errors.New("booking: new slot is the same as the current one")
	ErrSlotNotFound       = errors.New("booking: slot not found")
	ErrSlotOverlap        = errors.New("booking: executive already has a slot at that time")
	ErrSlotHasBooking     = errors.New("booking: slot already has a booking")
	ErrInvalidSlotTimes   = errors.New("booking: invalid slot times")
	ErrExecutiveNotFound  = errors.New("booking: executive or project not found")
)

// ReminderPlan describes one reminder to store for a visit.
type ReminderPlan struct {
	Kind       string
	DueAt      time.Time
	Status     string // queued or skipped
	SkipReason string // empty unless skipped
}

// PlanReminders decides the two reminders (1 day and 2 hours before) for a
// visit starting at `start`, as of time `now`.
//
//   - A buyer who has not consented, or has opted out, gets nothing (DPDP Act).
//   - A reminder whose time has already passed is skipped, for example the
//     "1 day before" reminder when the visit was booked for tomorrow morning.
func PlanReminders(now, start time.Time, canContact bool) []ReminderPlan {
	schedule := []struct {
		kind   string
		before time.Duration
	}{
		{models.ReminderDayBefore, 24 * time.Hour},
		{models.ReminderTwoHoursBefore, 2 * time.Hour},
	}

	plans := make([]ReminderPlan, 0, len(schedule))
	for _, s := range schedule {
		p := ReminderPlan{Kind: s.kind, DueAt: start.Add(-s.before), Status: models.ReminderQueued}
		switch {
		case !canContact:
			p.Status, p.SkipReason = models.ReminderSkipped, "opted_out"
		case !p.DueAt.After(now):
			p.Status, p.SkipReason = models.ReminderSkipped, "too_late"
		}
		plans = append(plans, p)
	}
	return plans
}

// ValidateSlotTimes checks a slot an admin wants to create.
func ValidateSlotTimes(now, start, end time.Time) error {
	length := end.Sub(start)
	if !start.After(now) || length < MinSlotLength || length > MaxSlotLength {
		return ErrInvalidSlotTimes
	}
	return nil
}

// NewVisitCode returns an unguessable code for the buyer's /visit/:code link.
// 12 random bytes is 96 bits, far beyond anything that can be guessed.
func NewVisitCode() (string, error) {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
