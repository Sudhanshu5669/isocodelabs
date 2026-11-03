package booking_test

import (
	"errors"
	"testing"
	"time"

	"leadflow/internal/booking"
	"leadflow/internal/models"
)

func TestPlanReminders(t *testing.T) {
	now := time.Date(2026, 11, 2, 12, 0, 0, 0, booking.IST)

	tests := []struct {
		name       string
		start      time.Time
		canContact bool
		wantDay    string // status of the 1-day reminder
		wantTwoHr  string // status of the 2-hour reminder
		wantReason string // skip reason of the 1-day reminder
	}{
		{"visit in 3 days: both queued", now.Add(72 * time.Hour), true, models.ReminderQueued, models.ReminderQueued, ""},
		{"visit tomorrow morning: day-before is already past", now.Add(20 * time.Hour), true, models.ReminderSkipped, models.ReminderQueued, "too_late"},
		{"visit in 3 hours: day-before skipped, 2-hour still due", now.Add(3 * time.Hour), true, models.ReminderSkipped, models.ReminderQueued, "too_late"},
		{"visit in 1 hour: both skipped", now.Add(1 * time.Hour), true, models.ReminderSkipped, models.ReminderSkipped, "too_late"},
		{"opted-out buyer gets nothing", now.Add(72 * time.Hour), false, models.ReminderSkipped, models.ReminderSkipped, "opted_out"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			plans := booking.PlanReminders(now, tc.start, tc.canContact)
			if len(plans) != 2 {
				t.Fatalf("want 2 reminders, got %d", len(plans))
			}
			if plans[0].Kind != models.ReminderDayBefore || plans[1].Kind != models.ReminderTwoHoursBefore {
				t.Fatalf("unexpected kinds: %s, %s", plans[0].Kind, plans[1].Kind)
			}
			if plans[0].Status != tc.wantDay || plans[1].Status != tc.wantTwoHr {
				t.Errorf("statuses = %s/%s, want %s/%s", plans[0].Status, plans[1].Status, tc.wantDay, tc.wantTwoHr)
			}
			if plans[0].SkipReason != tc.wantReason {
				t.Errorf("skip reason = %q, want %q", plans[0].SkipReason, tc.wantReason)
			}
			if got, want := plans[0].DueAt, tc.start.Add(-24*time.Hour); !got.Equal(want) {
				t.Errorf("day-before due at %v, want %v", got, want)
			}
			if got, want := plans[1].DueAt, tc.start.Add(-2*time.Hour); !got.Equal(want) {
				t.Errorf("two-hours due at %v, want %v", got, want)
			}
		})
	}
}

func TestValidateSlotTimes(t *testing.T) {
	now := time.Date(2026, 11, 2, 12, 0, 0, 0, booking.IST)
	start := now.Add(48 * time.Hour)

	tests := []struct {
		name       string
		start, end time.Time
		wantErr    bool
	}{
		{"one hour in the future", start, start.Add(time.Hour), false},
		{"in the past", now.Add(-time.Hour), now, true},
		{"ends before it starts", start, start.Add(-time.Hour), true},
		{"too short", start, start.Add(5 * time.Minute), true},
		{"too long", start, start.Add(5 * time.Hour), true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := booking.ValidateSlotTimes(now, tc.start, tc.end)
			if (err != nil) != tc.wantErr {
				t.Errorf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if err != nil && !errors.Is(err, booking.ErrInvalidSlotTimes) {
				t.Errorf("unexpected error type: %v", err)
			}
		})
	}
}

func TestVisitCodesAreUniqueAndLongEnough(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 1000; i++ {
		c, err := booking.NewVisitCode()
		if err != nil {
			t.Fatal(err)
		}
		if len(c) != 24 {
			t.Fatalf("code length %d, want 24", len(c))
		}
		if seen[c] {
			t.Fatal("duplicate code generated")
		}
		seen[c] = true
	}
}
