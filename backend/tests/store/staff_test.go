package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"leadflow/internal/leads"
	"leadflow/internal/models"
	"leadflow/internal/store"
)

func TestSwitchingOffAUserSignsThemOutAndWithdrawsTheirSlots(t *testing.T) {
	requireDB(t)
	ctx := context.Background()
	exec := newExecutive(t)
	open := newSlot(t, exec, futureStart(200))
	booked := newSlot(t, exec, futureStart(201))
	if _, err := testStore.BookSlot(ctx, time.Now(), newLead(t, true), booked); err != nil {
		t.Fatal(err)
	}
	if err := testStore.CreateSession(ctx, exec, []byte(randHex(16)), time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}

	res, err := testStore.SetUserActive(ctx, time.Now(), exec, false)
	if err != nil {
		t.Fatal(err)
	}
	if res.CancelledOpenSlots != 1 || res.BookedVisits != 1 {
		t.Errorf("result %+v, want 1 slot withdrawn and 1 visit to move", res)
	}
	if s := scalar[string](t, `SELECT status FROM visit_slots WHERE id = $1::uuid`, open); s != models.SlotCancelled {
		t.Errorf("open slot is %s, want cancelled so no buyer can pick it", s)
	}
	if n := scalar[int64](t, `SELECT count(*) FROM sessions WHERE user_id = $1::uuid`, exec); n != 0 {
		t.Error("sessions must be deleted at once")
	}
	if _, err := testStore.SetUserActive(ctx, time.Now(), "00000000-0000-4000-8000-000000000000", false); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("unknown user: %v", err)
	}
}

func TestCreateUserRejectsDuplicateEmail(t *testing.T) {
	requireDB(t)
	email := randHex(6) + "@test.local"
	if _, err := testStore.CreateUser(context.Background(), email, "New Exec", models.RoleExecutive, "hash"); err != nil {
		t.Fatal(err)
	}
	_, err := testStore.CreateUser(context.Background(), email, "Again", models.RoleExecutive, "hash")
	if !errors.Is(err, store.ErrEmailTaken) {
		t.Fatalf("want ErrEmailTaken, got %v", err)
	}
}

func TestSetLeadStatusRules(t *testing.T) {
	requireDB(t)
	ctx := context.Background()
	owner := newExecutive(t)
	other := newExecutive(t)
	lead := newLead(t, true)
	if _, err := testPool.Exec(ctx, `UPDATE leads SET assigned_to = $2::uuid WHERE id = $1::uuid`, lead, owner); err != nil {
		t.Fatal(err)
	}

	if err := testStore.SetLeadStatus(ctx, store.Viewer{ID: other}, lead, models.LeadLost); !errors.Is(err, leads.ErrNotFound) {
		t.Fatalf("another executive's lead must look missing: %v", err)
	}
	if err := testStore.SetLeadStatus(ctx, store.Viewer{ID: owner}, lead, models.LeadLost); err != nil {
		t.Fatal(err)
	}
	if s := scalar[string](t, `SELECT status FROM leads WHERE id = $1::uuid`, lead); s != models.LeadLost {
		t.Fatalf("status %s", s)
	}

	// A lead with a booked visit cannot be marked lost: cancel the visit first.
	booked := newLead(t, true)
	if _, err := testStore.BookSlot(ctx, time.Now(), booked, newSlot(t, owner, futureStart(210))); err != nil {
		t.Fatal(err)
	}
	if err := testStore.SetLeadStatus(ctx, store.Viewer{ID: owner}, booked, models.LeadLost); !errors.Is(err, store.ErrLeadHasBooking) {
		t.Fatalf("want ErrLeadHasBooking, got %v", err)
	}
}

func TestStaffVisitsAndSlotsAreScopedToTheExecutive(t *testing.T) {
	requireDB(t)
	ctx := context.Background()
	a, b := newExecutive(t), newExecutive(t)
	startA, startB := futureStart(220), futureStart(220).Add(2*time.Hour)
	if _, err := testStore.BookSlot(ctx, time.Now(), newLead(t, true), newSlot(t, a, startA)); err != nil {
		t.Fatal(err)
	}
	if _, err := testStore.BookSlot(ctx, time.Now(), newLead(t, true), newSlot(t, b, startB)); err != nil {
		t.Fatal(err)
	}
	f := store.VisitFilter{From: startA.Add(-time.Hour), To: startB.Add(time.Hour)}

	mine, err := testStore.StaffVisits(ctx, time.Now(), store.Viewer{ID: a}, f)
	if err != nil {
		t.Fatal(err)
	}
	if len(mine) != 1 || mine[0].ExecutiveID != a {
		t.Fatalf("executive A sees %d visits, want only their own", len(mine))
	}
	all, err := testStore.StaffVisits(ctx, time.Now(), store.Viewer{ID: a, IsHead: true}, f)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) < 2 {
		t.Fatalf("the head should see both visits, got %d", len(all))
	}

	slots, err := testStore.StaffSlots(ctx, store.Viewer{ID: b}, "", f.From, f.To)
	if err != nil {
		t.Fatal(err)
	}
	if len(slots) != 1 || slots[0].ExecutiveID != b || slots[0].LeadName == nil {
		t.Fatalf("executive B's slots: %+v", slots)
	}
}
