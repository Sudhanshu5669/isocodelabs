package httpapi_test

import (
	"fmt"
	"math/rand/v2"
	"net/http"
	"strings"
	"testing"

	"leadflow/internal/models"
)

// ---- helpers ---------------------------------------------------------------

func uniquePhone() string { return fmt.Sprintf("9%09d", rand.IntN(1_000_000_000)) }

// uniqueName gives two-word names that never look alike to the duplicate check.
func uniqueName() string { return randHex(4) + " " + randHex(4) }

func leadBody(name, phone string) map[string]any {
	return map[string]any{"name": name, "phone": phone, "source": "website", "projectId": "project-1", "consent": true}
}

type ingestJSON struct {
	Outcome         string `json:"outcome"`
	DuplicateOfCode string `json:"duplicateOfCode"`
	Lead            *struct {
		ID         string  `json:"id"`
		Code       string  `json:"code"`
		Name       string  `json:"name"`
		Phone      string  `json:"phone"`
		AssignedTo *string `json:"assignedTo"`
	} `json:"lead"`
}

func createLead(t *testing.T, cookie *http.Cookie, body map[string]any, want int) ingestJSON {
	t.Helper()
	rec := call(t, http.MethodPost, "/api/staff/leads", cookie, body)
	wantStatus(t, rec, want)
	return decode[ingestJSON](t, rec)
}

type leadPageJSON struct {
	Total int `json:"total"`
	Leads []struct {
		ID string `json:"id"`
	} `json:"leads"`
}

// ---- access control --------------------------------------------------------

func TestLeadRoutesNeedSignIn(t *testing.T) {
	requireDB(t)
	id := "00000000-0000-0000-0000-000000000001"
	routes := []struct{ method, path string }{
		{http.MethodGet, "/api/staff/leads"},
		{http.MethodPost, "/api/staff/leads"},
		{http.MethodPost, "/api/staff/leads/import"},
		{http.MethodGet, "/api/staff/leads/" + id},
		{http.MethodPost, "/api/staff/leads/" + id + "/assign"},
		{http.MethodPost, "/api/staff/leads/" + id + "/duplicate"},
	}
	for _, rt := range routes {
		t.Run(rt.method+" "+rt.path, func(t *testing.T) {
			wantStatus(t, call(t, rt.method, rt.path, nil, map[string]string{}), http.StatusUnauthorized)
		})
	}
}

func TestOnlyTheHeadCanAssignAndSettleDuplicates(t *testing.T) {
	requireDB(t)
	_, exec := newStaff(t, models.RoleExecutive)
	id := "00000000-0000-0000-0000-000000000001"

	wantStatus(t, call(t, http.MethodPost, "/api/staff/leads/"+id+"/assign", exec, map[string]string{"executiveId": id}), http.StatusForbidden)
	wantStatus(t, call(t, http.MethodPost, "/api/staff/leads/"+id+"/duplicate", exec, map[string]string{"action": "merge"}), http.StatusForbidden)
}

func TestExecutiveSeesOnlyTheirOwnLeads(t *testing.T) {
	requireDB(t)
	execA, cookieA := newStaff(t, models.RoleExecutive)
	_, cookieB := newStaff(t, models.RoleExecutive)
	_, head := newStaff(t, models.RoleHead)

	created := createLead(t, head, leadBody(uniqueName(), uniquePhone()), http.StatusCreated)
	path := "/api/staff/leads/" + created.Lead.ID

	// Unassigned: only the head can open it.
	wantStatus(t, call(t, http.MethodGet, path, cookieA, nil), http.StatusNotFound)
	wantStatus(t, call(t, http.MethodGet, path, head, nil), http.StatusOK)

	// The head hands it to executive A.
	wantStatus(t, call(t, http.MethodPost, path+"/assign", head, map[string]string{"executiveId": execA}), http.StatusNoContent)

	wantStatus(t, call(t, http.MethodGet, path, cookieA, nil), http.StatusOK)
	wantStatus(t, call(t, http.MethodGet, path, cookieB, nil), http.StatusNotFound)

	list := decode[leadPageJSON](t, call(t, http.MethodGet, "/api/staff/leads?q="+created.Lead.Code, cookieA, nil))
	if list.Total != 1 || list.Leads[0].ID != created.Lead.ID {
		t.Errorf("owner's list = %+v, want just their lead", list)
	}
	listB := decode[leadPageJSON](t, call(t, http.MethodGet, "/api/staff/leads?q="+created.Lead.Code, cookieB, nil))
	if listB.Total != 0 {
		t.Errorf("another executive's list should not include it, got %+v", listB)
	}
}

// ---- adding leads ----------------------------------------------------------

func TestCreateLeadAndRepeatEnquiry(t *testing.T) {
	requireDB(t)
	_, head := newStaff(t, models.RoleHead)
	phone := uniquePhone()
	name := uniqueName()

	first := createLead(t, head, leadBody(name, phone), http.StatusCreated)
	if first.Outcome != "created" || first.Lead == nil || first.Lead.Phone != "+91"+phone {
		t.Fatalf("first = %+v", first)
	}

	// Same person, written differently, through another source: merged, status 200.
	again := leadBody("Someone Else", "+91 "+phone[:5]+" "+phone[5:])
	again["source"] = "social_ad"
	second := createLead(t, head, again, http.StatusOK)
	if second.Outcome != "merged" || second.Lead.ID != first.Lead.ID || second.Lead.Name != name {
		t.Fatalf("second = %+v, want the original lead back", second)
	}
}

func TestExecutiveWhoAddsALeadOwnsIt(t *testing.T) {
	requireDB(t)
	execID, cookie := newStaff(t, models.RoleExecutive)

	res := createLead(t, cookie, leadBody(uniqueName(), uniquePhone()), http.StatusCreated)
	if res.Lead == nil || res.Lead.AssignedTo == nil || *res.Lead.AssignedTo != execID {
		t.Errorf("the creating executive should own the lead: %+v", res.Lead)
	}
}

// Adding a phone number must not reveal who else's lead it belongs to.
func TestAddingSomeoneElsesPhoneRevealsNothing(t *testing.T) {
	requireDB(t)
	_, cookieA := newStaff(t, models.RoleExecutive)
	_, cookieB := newStaff(t, models.RoleExecutive)
	phone := uniquePhone()
	name := uniqueName()

	createLead(t, cookieA, leadBody(name, phone), http.StatusCreated)

	rec := call(t, http.MethodPost, "/api/staff/leads", cookieB, leadBody("Probe Person", phone))
	wantStatus(t, rec, http.StatusOK)
	if strings.Contains(rec.Body.String(), name) || strings.Contains(rec.Body.String(), phone) {
		t.Errorf("response leaked the other lead's details: %s", rec.Body.String())
	}
	if got := decode[ingestJSON](t, rec); got.Outcome != "merged" || got.Lead != nil {
		t.Errorf("got %+v, want merged with no lead details", got)
	}
}

func TestCreateLeadRejectsBadInput(t *testing.T) {
	requireDB(t)
	_, head := newStaff(t, models.RoleHead)

	tests := map[string]map[string]any{
		"bad phone":       {"name": "Asha Rao", "phone": "12345", "source": "website"},
		"unknown source":  {"name": "Asha Rao", "phone": uniquePhone(), "source": "newspaper"},
		"short name":      {"name": "A", "phone": uniquePhone(), "source": "website"},
		"unknown project": {"name": "Asha Rao", "phone": uniquePhone(), "source": "website", "projectId": "project-9"},
		"unknown field":   {"name": "Asha Rao", "phone": uniquePhone(), "source": "website", "isAdmin": true},
		"bad wants":       {"name": "Asha Rao", "phone": uniquePhone(), "source": "website", "wants": "10 BHK"},
	}
	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			wantStatus(t, call(t, http.MethodPost, "/api/staff/leads", head, body), http.StatusBadRequest)
		})
	}
}

func TestImportLeadsReportsEachRow(t *testing.T) {
	requireDB(t)
	_, head := newStaff(t, models.RoleHead)
	phone := uniquePhone()

	body := map[string]any{"leads": []map[string]any{
		leadBody(uniqueName(), phone),
		leadBody(uniqueName(), "not-a-phone"),
		leadBody(uniqueName(), phone), // the same person again
	}}
	rec := call(t, http.MethodPost, "/api/staff/leads/import", head, body)
	wantStatus(t, rec, http.StatusOK)

	got := decode[struct {
		Created, Merged, Failed int
		Results                 []struct {
			Row     int
			Outcome string
			Code    string
			Error   string
		}
	}](t, rec)
	if got.Created != 1 || got.Merged != 1 || got.Failed != 1 || len(got.Results) != 3 {
		t.Fatalf("got %+v, want 1 created, 1 merged, 1 failed", got)
	}
	if got.Results[1].Error == "" || got.Results[1].Row != 2 {
		t.Errorf("row 2 should explain the problem: %+v", got.Results[1])
	}
	if got.Results[0].Code == "" || got.Results[0].Code != got.Results[2].Code {
		t.Errorf("rows 1 and 3 should be the same lead: %+v", got.Results)
	}

	wantStatus(t, call(t, http.MethodPost, "/api/staff/leads/import", head, map[string]any{"leads": []any{}}), http.StatusBadRequest)
	tooMany := make([]map[string]any, 201)
	for i := range tooMany {
		tooMany[i] = leadBody("Name Here", "9000000000")
	}
	wantStatus(t, call(t, http.MethodPost, "/api/staff/leads/import", head, map[string]any{"leads": tooMany}), http.StatusBadRequest)
}

// ---- listing ---------------------------------------------------------------

func TestListLeadsRejectsBadParameters(t *testing.T) {
	requireDB(t)
	_, head := newStaff(t, models.RoleHead)

	bad := map[string]string{
		"source":        "source=newspaper",
		"status":        "status=maybe",
		"project":       "project=x'%3BDROP",
		"assignee":      "assignee=everyone",
		"limit zero":    "limit=0",
		"limit too big": "limit=101",
		"limit text":    "limit=ten",
		"negative page": "offset=-1",
		"long search":   "q=" + strings.Repeat("a", 81),
	}
	for name, query := range bad {
		t.Run(name, func(t *testing.T) {
			wantStatus(t, call(t, http.MethodGet, "/api/staff/leads?"+query, head, nil), http.StatusBadRequest)
		})
	}
	wantStatus(t, call(t, http.MethodGet, "/api/staff/leads?source=website&status=new&assignee=unassigned&needsAttention=true&limit=5&offset=0", head, nil), http.StatusOK)
	wantStatus(t, call(t, http.MethodGet, "/api/staff/leads/not-a-uuid", head, nil), http.StatusNotFound)
}

// ---- settling duplicates ---------------------------------------------------

func TestResolveDuplicateOverHTTP(t *testing.T) {
	requireDB(t)
	_, head := newStaff(t, models.RoleHead)

	flaggedPair := func() (orig, dup ingestJSON) {
		name := uniqueName()
		orig = createLead(t, head, leadBody(name, uniquePhone()), http.StatusCreated)
		dup = createLead(t, head, leadBody(name, uniquePhone()), http.StatusCreated)
		if dup.DuplicateOfCode != orig.Lead.Code {
			t.Fatalf("setup: expected %s to be flagged against %s, got %q", dup.Lead.Code, orig.Lead.Code, dup.DuplicateOfCode)
		}
		return orig, dup
	}
	duplicatePath := func(l ingestJSON) string { return "/api/staff/leads/" + l.Lead.ID + "/duplicate" }

	t.Run("merge", func(t *testing.T) {
		orig, dup := flaggedPair()
		rec := call(t, http.MethodPost, duplicatePath(dup), head, map[string]string{"action": "merge"})
		wantStatus(t, rec, http.StatusOK)
		if got := decode[map[string]string](t, rec)["leadId"]; got != orig.Lead.ID {
			t.Errorf("kept %s, want the original %s", got, orig.Lead.ID)
		}
		wantStatus(t, call(t, http.MethodGet, "/api/staff/leads/"+dup.Lead.ID, head, nil), http.StatusNotFound)
	})

	t.Run("dismiss", func(t *testing.T) {
		orig, dup := flaggedPair()
		wantStatus(t, call(t, http.MethodPost, duplicatePath(dup), head, map[string]string{"action": "dismiss"}), http.StatusNoContent)
		wantStatus(t, call(t, http.MethodGet, "/api/staff/leads/"+orig.Lead.ID, head, nil), http.StatusOK)
		wantStatus(t, call(t, http.MethodGet, "/api/staff/leads/"+dup.Lead.ID, head, nil), http.StatusOK)
		// Already settled: a second attempt is a conflict, not a crash.
		wantStatus(t, call(t, http.MethodPost, duplicatePath(dup), head, map[string]string{"action": "dismiss"}), http.StatusConflict)
	})

	t.Run("bad action", func(t *testing.T) {
		_, dup := flaggedPair()
		wantStatus(t, call(t, http.MethodPost, duplicatePath(dup), head, map[string]string{"action": "delete"}), http.StatusBadRequest)
	})
}
