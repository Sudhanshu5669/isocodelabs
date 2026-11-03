package httpapi_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"leadflow/internal/models"
)

type visitJSON struct {
	ID        string `json:"id"`
	Code      string `json:"code"`
	Status    string `json:"status"`
	SlotID    string `json:"slotId"`
	CanChange bool   `json:"canChange"`
}

func TestSlotsEndpoint(t *testing.T) {
	requireDB(t)
	exec, _ := newUser(t, models.RoleExecutive)
	newSlot(t, exec)

	ok := call(t, http.MethodGet, "/api/slots?project=project-1&from="+time.Now().Format("2006-01-02")+"&to="+time.Now().Add(30*24*time.Hour).Format("2006-01-02"), nil, nil)
	wantStatus(t, ok, http.StatusOK)
	body := decode[struct {
		Slots []struct {
			ID            string `json:"id"`
			ExecutiveName string `json:"executiveName"`
		} `json:"slots"`
	}](t, ok)
	if len(body.Slots) == 0 || body.Slots[0].ExecutiveName == "" {
		t.Errorf("expected slots that name their executive, got %+v", body)
	}

	for name, path := range map[string]string{
		"missing project":    "/api/slots",
		"malicious project":  "/api/slots?project=x'%3BDROP+TABLE+leads",
		"bad date":           "/api/slots?project=project-1&from=tomorrow",
		"range over 31 days": "/api/slots?project=project-1&from=2026-01-01&to=2026-06-01",
		"end before start":   "/api/slots?project=project-1&from=2026-11-10&to=2026-11-01",
	} {
		t.Run(name, func(t *testing.T) {
			wantStatus(t, call(t, http.MethodGet, path, nil, nil), http.StatusBadRequest)
		})
	}
}

// The buyer's journey through their unguessable link: view, move, cancel.
func TestBuyerVisitLinkFlow(t *testing.T) {
	requireDB(t)
	execID, _ := newUser(t, models.RoleExecutive)
	_, headCookie := newStaff(t, models.RoleHead)
	leadID, phone := newLead(t, "")
	slotA, slotB := newSlot(t, execID), newSlot(t, execID)

	// Staff books the visit.
	booked := call(t, http.MethodPost, "/api/staff/visits", headCookie, map[string]string{"leadId": leadID, "slotId": slotA})
	wantStatus(t, booked, http.StatusCreated)
	visit := decode[visitJSON](t, booked)

	// The buyer opens the link. It must not expose their phone number.
	view := call(t, http.MethodGet, "/api/visits/"+visit.Code, nil, nil)
	wantStatus(t, view, http.StatusOK)
	if strings.Contains(view.Body.String(), phone) || strings.Contains(view.Body.String(), "Sharma") {
		t.Error("the public visit page must not leak the buyer's phone or surname")
	}
	if d := decode[visitJSON](t, view); !d.CanChange || d.Status != models.VisitBooked {
		t.Errorf("unexpected detail: %+v", d)
	}

	// Reschedule to slot B, then try to move to a nonsense slot.
	moved := call(t, http.MethodPost, "/api/visits/"+visit.Code+"/reschedule", nil, map[string]string{"slotId": slotB})
	wantStatus(t, moved, http.StatusOK)
	if d := decode[visitJSON](t, moved); d.SlotID != slotB {
		t.Errorf("visit should now be on slot B, is on %s", d.SlotID)
	}
	wantStatus(t, call(t, http.MethodPost, "/api/visits/"+visit.Code+"/reschedule", nil, map[string]string{"slotId": "not-a-uuid"}), http.StatusBadRequest)
	wantStatus(t, call(t, http.MethodPost, "/api/visits/"+visit.Code+"/reschedule", nil, map[string]string{"slotId": slotB}), http.StatusBadRequest) // same slot

	// Cancel, then a second cancel is refused.
	cancelled := call(t, http.MethodPost, "/api/visits/"+visit.Code+"/cancel", nil, nil)
	wantStatus(t, cancelled, http.StatusOK)
	if d := decode[visitJSON](t, cancelled); d.Status != models.VisitCancelled || d.CanChange {
		t.Errorf("unexpected detail after cancel: %+v", d)
	}
	wantStatus(t, call(t, http.MethodPost, "/api/visits/"+visit.Code+"/cancel", nil, nil), http.StatusConflict)

	// Codes that are malformed or simply unknown give the same 404.
	wantStatus(t, call(t, http.MethodGet, "/api/visits/short", nil, nil), http.StatusNotFound)
	wantStatus(t, call(t, http.MethodGet, "/api/visits/"+strings.Repeat("a", 24), nil, nil), http.StatusNotFound)
}

func TestStaffBookingAccessControl(t *testing.T) {
	requireDB(t)
	execA, cookieA := newStaff(t, models.RoleExecutive)
	execB, cookieB := newStaff(t, models.RoleExecutive)
	_, headCookie := newStaff(t, models.RoleHead)

	leadOfA, _ := newLead(t, execA)
	leadOfB, _ := newLead(t, execB)
	slot1, slot2, slot3 := newSlot(t, execA), newSlot(t, execA), newSlot(t, execA)

	// Not signed in.
	wantStatus(t, call(t, http.MethodPost, "/api/staff/visits", nil, map[string]string{"leadId": leadOfA, "slotId": slot1}), http.StatusUnauthorized)

	// An executive cannot book for another executive's lead...
	wantStatus(t, call(t, http.MethodPost, "/api/staff/visits", cookieA, map[string]string{"leadId": leadOfB, "slotId": slot1}), http.StatusForbidden)
	// ...but can for their own.
	own := call(t, http.MethodPost, "/api/staff/visits", cookieA, map[string]string{"leadId": leadOfA, "slotId": slot1})
	wantStatus(t, own, http.StatusCreated)
	visit := decode[visitJSON](t, own)

	// Another executive cannot cancel or mark that visit; the head can.
	wantStatus(t, call(t, http.MethodPost, "/api/staff/visits/"+visit.ID+"/cancel", cookieB, nil), http.StatusForbidden)
	wantStatus(t, call(t, http.MethodPost, "/api/staff/visits/"+visit.ID+"/outcome", cookieB, map[string]string{"outcome": "visited"}), http.StatusForbidden)
	wantStatus(t, call(t, http.MethodPost, "/api/staff/visits/"+visit.ID+"/outcome", cookieA, map[string]string{"outcome": "visited"}), http.StatusConflict) // visit is in the future
	wantStatus(t, call(t, http.MethodPost, "/api/staff/visits/"+visit.ID+"/outcome", cookieA, map[string]string{"outcome": "maybe"}), http.StatusBadRequest)
	wantStatus(t, call(t, http.MethodPost, "/api/staff/visits/"+visit.ID+"/cancel", headCookie, nil), http.StatusNoContent)

	// The head may book for any lead.
	wantStatus(t, call(t, http.MethodPost, "/api/staff/visits", headCookie, map[string]string{"leadId": leadOfB, "slotId": slot2}), http.StatusCreated)

	// Input validation.
	wantStatus(t, call(t, http.MethodPost, "/api/staff/visits", headCookie, map[string]string{"leadId": "x", "slotId": slot3}), http.StatusBadRequest)
	wantStatus(t, call(t, http.MethodPost, "/api/staff/visits", headCookie, map[string]any{"leadId": leadOfA, "slotId": slot3, "extra": 1}), http.StatusBadRequest)
	wantStatus(t, call(t, http.MethodPost, "/api/staff/visits/not-a-uuid/cancel", headCookie, nil), http.StatusNotFound)
}

func TestSlotManagementIsHeadOnly(t *testing.T) {
	requireDB(t)
	execID, execCookie := newStaff(t, models.RoleExecutive)
	_, headCookie := newStaff(t, models.RoleHead)

	start := time.Now().Add(20 * 24 * time.Hour).Truncate(time.Hour)
	body := func(s, e time.Time) map[string]string {
		return map[string]string{"projectId": "project-1", "executiveId": execID, "startsAt": s.Format(time.RFC3339), "endsAt": e.Format(time.RFC3339)}
	}

	// An executive is forbidden; nobody signed in is unauthorised.
	wantStatus(t, call(t, http.MethodPost, "/api/staff/slots", execCookie, body(start, start.Add(time.Hour))), http.StatusForbidden)
	wantStatus(t, call(t, http.MethodPost, "/api/staff/slots", nil, body(start, start.Add(time.Hour))), http.StatusUnauthorized)

	// The head can create a slot, but not an overlapping or invalid one.
	created := call(t, http.MethodPost, "/api/staff/slots", headCookie, body(start, start.Add(time.Hour)))
	wantStatus(t, created, http.StatusCreated)
	slot := decode[struct {
		ID string `json:"id"`
	}](t, created)
	wantStatus(t, call(t, http.MethodPost, "/api/staff/slots", headCookie, body(start.Add(30*time.Minute), start.Add(90*time.Minute))), http.StatusConflict)
	wantStatus(t, call(t, http.MethodPost, "/api/staff/slots", headCookie, body(time.Now().Add(-time.Hour), time.Now())), http.StatusBadRequest)
	wantStatus(t, call(t, http.MethodPost, "/api/staff/slots", headCookie, map[string]string{"projectId": "project-1", "executiveId": execID, "startsAt": "soon", "endsAt": "later"}), http.StatusBadRequest)

	// A slot with a booking cannot be deleted; an open one can.
	leadID, _ := newLead(t, "")
	wantStatus(t, call(t, http.MethodPost, "/api/staff/visits", headCookie, map[string]string{"leadId": leadID, "slotId": slot.ID}), http.StatusCreated)
	wantStatus(t, call(t, http.MethodDelete, "/api/staff/slots/"+slot.ID, headCookie, nil), http.StatusConflict)
	open := newSlot(t, execID)
	wantStatus(t, call(t, http.MethodDelete, "/api/staff/slots/"+open, execCookie, nil), http.StatusForbidden)
	wantStatus(t, call(t, http.MethodDelete, "/api/staff/slots/"+open, headCookie, nil), http.StatusNoContent)
}

func TestCrossSiteBookingIsBlocked(t *testing.T) {
	requireDB(t)
	execID, _ := newUser(t, models.RoleExecutive)
	_, headCookie := newStaff(t, models.RoleHead)
	leadID, _ := newLead(t, "")
	slotID := newSlot(t, execID)

	req := newJSONRequest(t, http.MethodPost, "/api/staff/visits", headCookie, map[string]string{"leadId": leadID, "slotId": slotID})
	req.Header.Set("Origin", "https://evil.example")
	rec := serve(req)
	wantStatus(t, rec, http.StatusForbidden)
}
