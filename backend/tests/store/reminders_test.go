package store_test

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"leadflow/internal/models"
	"leadflow/internal/pii"
	"leadflow/internal/reminders"
	"leadflow/internal/store"
)

// bookedVisit books a visit `days` ahead for a new lead and returns the visit,
// the lead id and the slot start.
func bookedVisit(t *testing.T, days int, consent bool) (models.Visit, string, time.Time) {
	t.Helper()
	exec := newExecutive(t)
	lead := newLead(t, consent)
	start := futureStart(days)
	visit, err := testStore.BookSlot(context.Background(), time.Now(), lead, newSlot(t, exec, start))
	if err != nil {
		t.Fatal(err)
	}
	return visit, lead, start
}

func reminderStatus(t *testing.T, visitID, kind string) (status string, reason *string) {
	t.Helper()
	err := testPool.QueryRow(context.Background(), `
		SELECT status, skip_reason FROM reminders WHERE visit_id = $1::uuid AND kind = $2`, visitID, kind).Scan(&status, &reason)
	if err != nil {
		t.Fatal(err)
	}
	return status, reason
}

// recorder is a Messenger that remembers what it sent.
type recorder struct {
	mu   sync.Mutex
	sent map[string]string // phone -> last body
}

func (r *recorder) Send(_ context.Context, phone, body string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sent[phone] = body
	return nil
}

func TestWorkerSendsDueReminderWithReraNumber(t *testing.T) {
	requireDB(t)
	visit, leadID, start := bookedVisit(t, 300, true)
	phone := decrypted(t, pii.Phone, `SELECT phone FROM leads WHERE id = $1::uuid`, leadID)

	msgr := &recorder{sent: map[string]string{}}
	w := &reminders.Worker{Store: testStore, Messenger: msgr, PublicURL: "https://visits.example.com",
		Now: func() time.Time { return start.Add(-24*time.Hour + time.Minute) }} // just after the day-before one is due
	for { // drain everything due at that moment (other tests' reminders too)
		n, err := w.RunOnce(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if n == 0 {
			break
		}
	}

	if status, _ := reminderStatus(t, visit.ID, models.ReminderDayBefore); status != models.ReminderSent {
		t.Fatalf("day-before reminder: %s", status)
	}
	if status, _ := reminderStatus(t, visit.ID, models.ReminderTwoHoursBefore); status != models.ReminderQueued {
		t.Fatalf("two-hour reminder is not due yet, but is %s", status)
	}
	body := msgr.sent[phone]
	for _, want := range []string{"TEST-RERA", "STOP", "https://visits.example.com/visit/" + visit.Code} {
		if !strings.Contains(body, want) {
			t.Errorf("message missing %q: %s", want, body)
		}
	}
	stored := decrypted(t, pii.Text, `SELECT body FROM reminders WHERE visit_id = $1::uuid AND kind = 'day_before'`, visit.ID)
	if stored != body {
		t.Error("the sent text must be kept on the reminder row for the staff log")
	}
}

func TestWorkerRechecksOptOutAtSendTime(t *testing.T) {
	requireDB(t)
	visit, leadID, start := bookedVisit(t, 310, true)
	// The buyer opts out by some route that did not touch the reminder rows.
	if _, err := testPool.Exec(context.Background(), `UPDATE leads SET opted_out = true WHERE id = $1::uuid`, leadID); err != nil {
		t.Fatal(err)
	}
	msgr := &recorder{sent: map[string]string{}}
	w := &reminders.Worker{Store: testStore, Messenger: msgr, Now: func() time.Time { return start.Add(-24*time.Hour + time.Minute) }}
	for {
		n, err := w.RunOnce(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if n == 0 {
			break
		}
	}
	status, reason := reminderStatus(t, visit.ID, models.ReminderDayBefore)
	if status != models.ReminderSkipped || reason == nil || *reason != reminders.SkipOptedOut {
		t.Fatalf("want skipped/opted_out, got %s/%v", status, reason)
	}
}

// Two (or more) servers sending reminders at once must never send one twice.
// FOR UPDATE SKIP LOCKED makes each worker skip rows another one holds.
func TestParallelWorkersSendEachReminderOnce(t *testing.T) {
	requireDB(t)
	var mine []string
	var latest time.Time
	for i := 0; i < 8; i++ {
		visit, _, start := bookedVisit(t, 320+i, true)
		mine = append(mine, visit.ID)
		if start.After(latest) {
			latest = start
		}
	}
	now := latest.Add(-24*time.Hour + time.Minute) // every day-before reminder above is due

	var mu sync.Mutex
	handled := map[string]int{} // visit code -> times handled
	var wg sync.WaitGroup
	for w := 0; w < 4; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				n, err := testStore.ProcessDueReminders(context.Background(), now, 5,
					func(_ context.Context, d store.DueReminder) store.ReminderResult {
						mu.Lock()
						handled[d.ID]++
						mu.Unlock()
						time.Sleep(10 * time.Millisecond) // hold the lock a moment, like a real send
						return store.ReminderResult{Status: "sent", Body: "x"}
					})
				if err != nil {
					t.Error(err)
					return
				}
				if n == 0 {
					return
				}
			}
		}()
	}
	wg.Wait()

	for _, visitID := range mine {
		id := scalar[string](t, `SELECT id::text FROM reminders WHERE visit_id = $1::uuid AND kind = 'day_before'`, visitID)
		if handled[id] != 1 {
			t.Errorf("reminder %s handled %d times, want exactly 1", id, handled[id])
		}
	}
}

func TestOptOutByVisitCodeAndPhoneSkipQueuedReminders(t *testing.T) {
	requireDB(t)
	ctx := context.Background()

	visit, leadID, _ := bookedVisit(t, 340, true)
	if err := testStore.OptOutByVisitCode(ctx, visit.Code); err != nil {
		t.Fatal(err)
	}
	if !scalar[bool](t, `SELECT opted_out FROM leads WHERE id = $1::uuid`, leadID) {
		t.Fatal("lead not opted out")
	}
	if n := scalar[int64](t, `SELECT count(*) FROM reminders WHERE visit_id = $1::uuid AND status = 'queued'`, visit.ID); n != 0 {
		t.Fatalf("%d reminders still queued after opt-out", n)
	}
	if err := testStore.OptOutByVisitCode(ctx, "nope"); err != store.ErrNotFound {
		t.Fatalf("unknown code: %v", err)
	}

	visit2, lead2, _ := bookedVisit(t, 345, true)
	phone := decrypted(t, pii.Phone, `SELECT phone FROM leads WHERE id = $1::uuid`, lead2)
	found, err := testStore.OptOutByPhone(ctx, phone)
	if err != nil || !found {
		t.Fatalf("opt out by phone: found=%v err=%v", found, err)
	}
	if status, _ := reminderStatus(t, visit2.ID, models.ReminderTwoHoursBefore); status != models.ReminderSkipped {
		t.Fatalf("reminder after STOP: %s", status)
	}
	if found, _ := testStore.OptOutByPhone(ctx, "+910000000000"); found {
		t.Error("an unknown number must report not found")
	}
}
