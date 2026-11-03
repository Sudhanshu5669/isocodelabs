package store_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"leadflow/internal/booking"
	"leadflow/internal/models"
)

var ctx = context.Background()

func TestBookSlot_Success(t *testing.T) {
	requireDB(t)
	exec, lead := newExecutive(t), newLead(t, true)
	slot := newSlot(t, exec, futureStart(3))

	visit, err := testStore.BookSlot(ctx, time.Now(), lead, slot)
	if err != nil {
		t.Fatal(err)
	}
	if visit.Status != models.VisitBooked || len(visit.Code) != 24 {
		t.Errorf("unexpected visit: %+v", visit)
	}
	if got := scalar[string](t, `SELECT status FROM visit_slots WHERE id = $1::uuid`, slot); got != models.SlotBooked {
		t.Errorf("slot status = %s, want booked", got)
	}
	if got := scalar[string](t, `SELECT status FROM leads WHERE id = $1::uuid`, lead); got != models.LeadVisitBooked {
		t.Errorf("lead status = %s, want visit_booked", got)
	}
	if got := scalar[string](t, `SELECT assigned_to::text FROM leads WHERE id = $1::uuid`, lead); got != exec {
		t.Errorf("lead should be assigned to the slot's executive")
	}
	if got := scalar[int](t, `SELECT count(*) FROM reminders WHERE visit_id = $1::uuid AND status = 'queued'`, visit.ID); got != 2 {
		t.Errorf("want 2 queued reminders, got %d", got)
	}
}

// The headline guarantee: many buyers race for one slot, exactly one wins.
func TestBookSlot_FiftyBuyersRaceForOneSlot(t *testing.T) {
	requireDB(t)
	exec := newExecutive(t)
	slot := newSlot(t, exec, futureStart(4))

	const buyers = 50
	leads := make([]string, buyers)
	for i := range leads {
		leads[i] = newLead(t, true)
	}

	var (
		wg      sync.WaitGroup
		start   = make(chan struct{}) // closed to release every goroutine at once
		results = make(chan error, buyers)
	)
	for _, lead := range leads {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := testStore.BookSlot(ctx, time.Now(), lead, slot)
			results <- err
		}()
	}
	close(start)
	wg.Wait()
	close(results)

	wins, taken := 0, 0
	for err := range results {
		switch {
		case err == nil:
			wins++
		case errors.Is(err, booking.ErrSlotUnavailable):
			taken++
		default:
			t.Errorf("unexpected error: %v", err)
		}
	}
	if wins != 1 || taken != buyers-1 {
		t.Fatalf("wins=%d taken=%d, want exactly 1 and %d", wins, taken, buyers-1)
	}
	if live := scalar[int](t, `SELECT count(*) FROM visits WHERE slot_id = $1::uuid AND status <> 'cancelled'`, slot); live != 1 {
		t.Fatalf("database holds %d live visits for the slot, want 1", live)
	}
}

// One buyer clicking two slots at once must end up with exactly one booking,
// and the losing slot must go back to open.
func TestBookSlot_SameLeadTwoSlotsAtOnce(t *testing.T) {
	requireDB(t)
	exec := newExecutive(t)
	slotA, slotB := newSlot(t, exec, futureStart(5)), newSlot(t, exec, futureStart(6))
	lead := newLead(t, true)

	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i, slot := range []string{slotA, slotB} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = testStore.BookSlot(ctx, time.Now(), lead, slot)
		}()
	}
	wg.Wait()

	wins := 0
	for _, err := range errs {
		if err == nil {
			wins++
		} else if !errors.Is(err, booking.ErrLeadAlreadyBooked) {
			t.Errorf("unexpected error: %v", err)
		}
	}
	if wins != 1 {
		t.Fatalf("wins = %d, want 1", wins)
	}
	open := scalar[int](t, `SELECT count(*) FROM visit_slots WHERE id IN ($1::uuid, $2::uuid) AND status = 'open'`, slotA, slotB)
	if open != 1 {
		t.Errorf("the losing slot must be open again, but %d slots are open", open)
	}
}

func TestBookSlot_RejectsPastAndTooSoonSlots(t *testing.T) {
	requireDB(t)
	exec, lead := newExecutive(t), newLead(t, true)

	// A slot 1 hour away is inside the minimum notice window.
	soon := newSlot(t, exec, time.Now().Add(time.Hour))
	if _, err := testStore.BookSlot(ctx, time.Now(), lead, soon); !errors.Is(err, booking.ErrSlotUnavailable) {
		t.Errorf("too-soon slot: err = %v, want ErrSlotUnavailable", err)
	}
	// An unknown slot id.
	if _, err := testStore.BookSlot(ctx, time.Now(), lead, "00000000-0000-0000-0000-000000000000"); !errors.Is(err, booking.ErrSlotUnavailable) {
		t.Errorf("unknown slot: err = %v, want ErrSlotUnavailable", err)
	}
	// An unknown lead.
	slot := newSlot(t, exec, futureStart(7))
	if _, err := testStore.BookSlot(ctx, time.Now(), "00000000-0000-0000-0000-000000000000", slot); !errors.Is(err, booking.ErrLeadNotFound) {
		t.Errorf("unknown lead: err = %v, want ErrLeadNotFound", err)
	}
	// Nothing above may have left the real slot booked.
	if got := scalar[string](t, `SELECT status FROM visit_slots WHERE id = $1::uuid`, slot); got != models.SlotOpen {
		t.Errorf("slot should still be open, is %s", got)
	}
}

func TestBookSlot_OptedOutBuyerGetsNoReminders(t *testing.T) {
	requireDB(t)
	exec, lead := newExecutive(t), newLead(t, true)
	if _, err := testPool.Exec(ctx, `UPDATE leads SET opted_out = true WHERE id = $1::uuid`, lead); err != nil {
		t.Fatal(err)
	}
	visit, err := testStore.BookSlot(ctx, time.Now(), lead, newSlot(t, exec, futureStart(3)))
	if err != nil {
		t.Fatal(err)
	}
	if got := scalar[int](t, `SELECT count(*) FROM reminders WHERE visit_id = $1::uuid AND status = 'skipped' AND skip_reason = 'opted_out'`, visit.ID); got != 2 {
		t.Errorf("want 2 skipped (opted_out) reminders, got %d", got)
	}
}

func TestCancelVisit_FreesSlotForSomeoneElse(t *testing.T) {
	requireDB(t)
	exec := newExecutive(t)
	slot := newSlot(t, exec, futureStart(3))
	first, second := newLead(t, true), newLead(t, true)

	visit, err := testStore.BookSlot(ctx, time.Now(), first, slot)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := testStore.BookSlot(ctx, time.Now(), second, slot); !errors.Is(err, booking.ErrSlotUnavailable) {
		t.Fatalf("second buyer should be refused while the slot is taken, got %v", err)
	}

	if err := testStore.CancelVisit(ctx, time.Now(), visit.ID); err != nil {
		t.Fatal(err)
	}
	if got := scalar[int](t, `SELECT count(*) FROM reminders WHERE visit_id = $1::uuid AND skip_reason = 'visit_cancelled'`, visit.ID); got != 2 {
		t.Errorf("cancelled visit should skip its reminders, got %d", got)
	}
	if got := scalar[string](t, `SELECT status FROM leads WHERE id = $1::uuid`, first); got != models.LeadEngaged {
		t.Errorf("lead status after cancel = %s, want engaged", got)
	}
	if _, err := testStore.BookSlot(ctx, time.Now(), second, slot); err != nil {
		t.Errorf("slot should be bookable again after cancel: %v", err)
	}
	if err := testStore.CancelVisit(ctx, time.Now(), visit.ID); !errors.Is(err, booking.ErrVisitNotChangeable) {
		t.Errorf("cancelling twice: err = %v, want ErrVisitNotChangeable", err)
	}
}

func TestRescheduleVisit_MovesAtomically(t *testing.T) {
	requireDB(t)
	exec, lead := newExecutive(t), newLead(t, true)
	oldSlot, newSl := newSlot(t, exec, futureStart(3)), newSlot(t, exec, futureStart(5))

	visit, err := testStore.BookSlot(ctx, time.Now(), lead, oldSlot)
	if err != nil {
		t.Fatal(err)
	}
	moved, err := testStore.RescheduleVisit(ctx, time.Now(), visit.ID, newSl)
	if err != nil {
		t.Fatal(err)
	}
	if moved.ID != visit.ID || moved.Code != visit.Code {
		t.Error("rescheduling must keep the same visit and link code")
	}
	if moved.SlotID != newSl {
		t.Error("visit should now point at the new slot")
	}
	if got := scalar[string](t, `SELECT status FROM visit_slots WHERE id = $1::uuid`, oldSlot); got != models.SlotOpen {
		t.Errorf("old slot = %s, want open", got)
	}
	if got := scalar[string](t, `SELECT status FROM visit_slots WHERE id = $1::uuid`, newSl); got != models.SlotBooked {
		t.Errorf("new slot = %s, want booked", got)
	}
	// Reminders must follow the new time.
	due := scalar[time.Time](t, `SELECT due_at FROM reminders WHERE visit_id = $1::uuid AND kind = 'two_hours_before'`, visit.ID)
	newStart := scalar[time.Time](t, `SELECT starts_at FROM visit_slots WHERE id = $1::uuid`, newSl)
	if !due.Equal(newStart.Add(-2 * time.Hour)) {
		t.Errorf("2-hour reminder due %v, want %v", due, newStart.Add(-2*time.Hour))
	}
	if got := scalar[int](t, `SELECT count(*) FROM visits WHERE lead_id = $1::uuid AND status = 'booked'`, lead); got != 1 {
		t.Errorf("lead should have exactly 1 booked visit, has %d", got)
	}
}

func TestRescheduleVisit_FailureKeepsOriginalBooking(t *testing.T) {
	requireDB(t)
	exec := newExecutive(t)
	slotA, slotB := newSlot(t, exec, futureStart(3)), newSlot(t, exec, futureStart(5))
	leadA, leadB := newLead(t, true), newLead(t, true)

	visitA, err := testStore.BookSlot(ctx, time.Now(), leadA, slotA)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := testStore.BookSlot(ctx, time.Now(), leadB, slotB); err != nil {
		t.Fatal(err)
	}

	// Lead A tries to move into the slot lead B already holds.
	if _, err := testStore.RescheduleVisit(ctx, time.Now(), visitA.ID, slotB); !errors.Is(err, booking.ErrSlotUnavailable) {
		t.Fatalf("err = %v, want ErrSlotUnavailable", err)
	}
	if got := scalar[string](t, `SELECT slot_id::text FROM visits WHERE id = $1::uuid`, visitA.ID); got != slotA {
		t.Error("a failed reschedule must leave the visit on its original slot")
	}
	if got := scalar[string](t, `SELECT status FROM visit_slots WHERE id = $1::uuid`, slotA); got != models.SlotBooked {
		t.Errorf("original slot = %s, want still booked", got)
	}
	if _, err := testStore.RescheduleVisit(ctx, time.Now(), visitA.ID, slotA); !errors.Is(err, booking.ErrSameSlot) {
		t.Errorf("same slot: err = %v, want ErrSameSlot", err)
	}
}

func TestCreateSlot_OverlapRules(t *testing.T) {
	requireDB(t)
	exec := newExecutive(t)
	start := futureStart(8)
	newSlot(t, exec, start) // [start, start+1h)

	if _, err := testStore.CreateSlot(ctx, "project-1", exec, start.Add(30*time.Minute), start.Add(90*time.Minute)); !errors.Is(err, booking.ErrSlotOverlap) {
		t.Errorf("overlapping slot: err = %v, want ErrSlotOverlap", err)
	}
	if _, err := testStore.CreateSlot(ctx, "project-1", exec, start.Add(time.Hour), start.Add(2*time.Hour)); err != nil {
		t.Errorf("back-to-back slot should be allowed: %v", err)
	}
	// A different executive may hold the very same time.
	if _, err := testStore.CreateSlot(ctx, "project-1", newExecutive(t), start, start.Add(time.Hour)); err != nil {
		t.Errorf("another executive may use the same time: %v", err)
	}
	if _, err := testStore.CreateSlot(ctx, "project-1", "00000000-0000-0000-0000-000000000000", start.Add(5*time.Hour), start.Add(6*time.Hour)); !errors.Is(err, booking.ErrExecutiveNotFound) {
		t.Errorf("unknown executive: err = %v, want ErrExecutiveNotFound", err)
	}
	if _, err := testStore.CreateSlot(ctx, "no-such-project", exec, start.Add(5*time.Hour), start.Add(6*time.Hour)); !errors.Is(err, booking.ErrExecutiveNotFound) {
		t.Errorf("unknown project: err = %v, want ErrExecutiveNotFound", err)
	}
}

func TestCancelSlot(t *testing.T) {
	requireDB(t)
	exec := newExecutive(t)
	open, booked := newSlot(t, exec, futureStart(9)), newSlot(t, exec, futureStart(10))
	if _, err := testStore.BookSlot(ctx, time.Now(), newLead(t, true), booked); err != nil {
		t.Fatal(err)
	}

	if err := testStore.CancelSlot(ctx, open); err != nil {
		t.Errorf("cancel open slot: %v", err)
	}
	if err := testStore.CancelSlot(ctx, booked); !errors.Is(err, booking.ErrSlotHasBooking) {
		t.Errorf("slot with a booking: err = %v, want ErrSlotHasBooking", err)
	}
	if err := testStore.CancelSlot(ctx, "00000000-0000-0000-0000-000000000000"); !errors.Is(err, booking.ErrSlotNotFound) {
		t.Errorf("unknown slot: err = %v, want ErrSlotNotFound", err)
	}
	if _, err := testStore.BookSlot(ctx, time.Now(), newLead(t, true), open); !errors.Is(err, booking.ErrSlotUnavailable) {
		t.Errorf("a cancelled slot must not be bookable, err = %v", err)
	}
}

func TestFreeSlots_ListsOnlyBookableSlots(t *testing.T) {
	requireDB(t)
	exec := newExecutive(t)
	base := futureStart(20)
	free1, free2 := newSlot(t, exec, base), newSlot(t, exec, base.Add(2*time.Hour))
	taken := newSlot(t, exec, base.Add(4*time.Hour))
	cancelled := newSlot(t, exec, base.Add(6*time.Hour))
	if _, err := testStore.BookSlot(ctx, time.Now(), newLead(t, true), taken); err != nil {
		t.Fatal(err)
	}
	if err := testStore.CancelSlot(ctx, cancelled); err != nil {
		t.Fatal(err)
	}

	slots, err := testStore.FreeSlots(ctx, time.Now(), "project-1", base.Add(-time.Hour), base.Add(12*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, s := range slots {
		ids = append(ids, s.ID)
		if s.ExecutiveName == "" {
			t.Error("each slot must name its executive")
		}
	}
	if len(ids) != 2 || ids[0] != free1 || ids[1] != free2 {
		t.Errorf("free slots = %v, want [%s %s] in time order", ids, free1, free2)
	}
}

func TestMarkOutcome(t *testing.T) {
	requireDB(t)
	exec, lead := newExecutive(t), newLead(t, true)

	// A visit still in the future cannot be marked.
	future, err := testStore.BookSlot(ctx, time.Now(), lead, newSlot(t, exec, futureStart(3)))
	if err != nil {
		t.Fatal(err)
	}
	if err := testStore.MarkOutcome(ctx, time.Now(), future.ID, models.VisitVisited); !errors.Is(err, booking.ErrVisitNotMarkable) {
		t.Errorf("future visit: err = %v, want ErrVisitNotMarkable", err)
	}

	// A visit whose time has passed can be, exactly once. The past slot is
	// inserted directly because the booking API rightly refuses past slots.
	pastLead := newLead(t, true)
	var slotID, visitID string
	past := time.Now().Add(-3 * time.Hour)
	if err := testPool.QueryRow(ctx, `INSERT INTO visit_slots (project_id, executive_id, starts_at, ends_at, status)
		VALUES ('project-1', $1::uuid, $2, $3, 'booked') RETURNING id::text`, exec, past, past.Add(time.Hour)).Scan(&slotID); err != nil {
		t.Fatal(err)
	}
	if err := testPool.QueryRow(ctx, `INSERT INTO visits (code, lead_id, slot_id) VALUES ($1, $2::uuid, $3::uuid) RETURNING id::text`,
		randHex(12), pastLead, slotID).Scan(&visitID); err != nil {
		t.Fatal(err)
	}

	if err := testStore.MarkOutcome(ctx, time.Now(), visitID, models.VisitVisited); err != nil {
		t.Fatal(err)
	}
	if got := scalar[string](t, `SELECT status FROM leads WHERE id = $1::uuid`, pastLead); got != models.LeadVisited {
		t.Errorf("lead status = %s, want visited", got)
	}
	if err := testStore.MarkOutcome(ctx, time.Now(), visitID, models.VisitNoShow); !errors.Is(err, booking.ErrVisitNotMarkable) {
		t.Errorf("marking twice: err = %v, want ErrVisitNotMarkable", err)
	}
	if err := testStore.MarkOutcome(ctx, time.Now(), visitID, "whatever"); !errors.Is(err, booking.ErrVisitNotMarkable) {
		t.Errorf("invalid outcome: err = %v, want ErrVisitNotMarkable", err)
	}
}
