package httpapi_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"leadflow/internal/ai"
	"leadflow/internal/auth"
	"leadflow/internal/booking"
	"leadflow/internal/config"
	"leadflow/internal/httpapi"
	"leadflow/internal/models"
)

// ---- projects and the fact-sheet editor --------------------------------------

func TestProjectsArePublic(t *testing.T) {
	requireDB(t)
	rec := call(t, http.MethodGet, "/api/projects", nil, nil)
	wantStatus(t, rec, http.StatusOK)
	if !strings.Contains(rec.Body.String(), `"reraNumber":"TEST-RERA"`) {
		t.Errorf("project list: %s", rec.Body.String())
	}
	wantStatus(t, call(t, http.MethodGet, "/api/projects/nope", nil, nil), http.StatusNotFound)
	wantStatus(t, call(t, http.MethodGet, "/api/projects/Bad..Id", nil, nil), http.StatusNotFound)
}

func TestFactEditorIsHeadOnlyAndLiveForTheAssistant(t *testing.T) {
	requireDB(t)
	_, execCookie := newStaff(t, models.RoleExecutive)
	_, headCookie := newStaff(t, models.RoleHead)
	topic := "clubhouse_hours_" + randHex(2)
	body := map[string]string{"label": "Clubhouse hours", "value": "The clubhouse is open 6 am to 10 pm."}

	wantStatus(t, call(t, http.MethodPut, "/api/staff/projects/project-1/facts/"+topic, execCookie, body), http.StatusForbidden)
	wantStatus(t, call(t, http.MethodPut, "/api/staff/projects/project-1/facts/"+topic, headCookie, body), http.StatusOK)
	wantStatus(t, call(t, http.MethodPut, "/api/staff/projects/project-1/facts/Bad-Topic", headCookie, body), http.StatusBadRequest)
	wantStatus(t, call(t, http.MethodPut, "/api/staff/projects/project-1/facts/"+topic, headCookie,
		map[string]string{"label": "x", "value": ""}), http.StatusBadRequest)

	// Buyers can read it, and executives can see the editor view.
	pub := call(t, http.MethodGet, "/api/projects/project-1", nil, nil)
	if !strings.Contains(pub.Body.String(), "6 am to 10 pm") || strings.Contains(pub.Body.String(), "updatedBy") {
		t.Errorf("public project should show the fact but no editor details: %s", pub.Body.String())
	}
	wantStatus(t, call(t, http.MethodGet, "/api/staff/projects/project-1/facts", execCookie, nil), http.StatusOK)

	// The very next chat message is answered from the updated fact sheet.
	sawFact := false
	model.script(func(req ai.Request) (ai.Turn, error) {
		if strings.Contains(req.System, "6 am to 10 pm") {
			sawFact = true
		}
		return modelSays("The clubhouse is open 6 am to 10 pm."), nil
	})
	cookie, _ := startChat(t, "en")
	sendChat(t, cookie, "When is the clubhouse open?")
	if !sawFact {
		t.Error("the assistant's prompt did not include the fact just saved")
	}

	wantStatus(t, call(t, http.MethodDelete, "/api/staff/projects/project-1/facts/"+topic, headCookie, nil), http.StatusNoContent)
	wantStatus(t, call(t, http.MethodDelete, "/api/staff/projects/project-1/facts/"+topic, headCookie, nil), http.StatusNotFound)
}

// ---- dashboard and spend -----------------------------------------------------

func TestDashboardAndSpendAreHeadOnly(t *testing.T) {
	requireDB(t)
	_, execCookie := newStaff(t, models.RoleExecutive)
	_, headCookie := newStaff(t, models.RoleHead)

	wantStatus(t, call(t, http.MethodGet, "/api/staff/dashboard", execCookie, nil), http.StatusForbidden)
	rec := call(t, http.MethodGet, "/api/staff/dashboard?from=2026-10-01&to=2026-10-31", headCookie, nil)
	wantStatus(t, rec, http.StatusOK)
	for _, field := range []string{`"totals"`, `"bySource"`, `"daily"`, `"medianFirstResponseSeconds"`, `"costPerVisitInr"`} {
		if !strings.Contains(rec.Body.String(), field) {
			t.Errorf("dashboard missing %s", field)
		}
	}
	wantStatus(t, call(t, http.MethodGet, "/api/staff/dashboard?from=2026-12-01&to=2026-11-01", headCookie, nil), http.StatusBadRequest)
	wantStatus(t, call(t, http.MethodGet, "/api/staff/dashboard?from=2024-01-01&to=2026-01-01", headCookie, nil), http.StatusBadRequest)

	spend := map[string]any{"source": "social_ad", "month": "2019-06", "amountInr": 45000}
	wantStatus(t, call(t, http.MethodPut, "/api/staff/spend", execCookie, spend), http.StatusForbidden)
	wantStatus(t, call(t, http.MethodPut, "/api/staff/spend", headCookie, spend), http.StatusOK)
	list := call(t, http.MethodGet, "/api/staff/spend?from=2019-06&to=2019-06", headCookie, nil)
	if !strings.Contains(list.Body.String(), `"amountInr":45000`) {
		t.Errorf("spend list: %s", list.Body.String())
	}
	wantStatus(t, call(t, http.MethodPut, "/api/staff/spend", headCookie,
		map[string]any{"source": "manual", "month": "2019-06", "amountInr": 1}), http.StatusBadRequest)
	wantStatus(t, call(t, http.MethodPut, "/api/staff/spend", headCookie,
		map[string]any{"source": "social_ad", "month": "June", "amountInr": 1}), http.StatusBadRequest)
}

// ---- user management -----------------------------------------------------------

func TestHeadManagesTheTeam(t *testing.T) {
	requireDB(t)
	headID, headCookie := newStaff(t, models.RoleHead)
	_, execCookie := newStaff(t, models.RoleExecutive)
	email := randHex(5) + "@test.local"
	newExec := map[string]string{"email": email, "name": "Priya Desai", "role": "executive", "password": "first-pass-123"}

	wantStatus(t, call(t, http.MethodGet, "/api/staff/users", execCookie, nil), http.StatusForbidden)
	wantStatus(t, call(t, http.MethodPost, "/api/staff/users", execCookie, newExec), http.StatusForbidden)

	weak := map[string]string{"email": randHex(5) + "@test.local", "name": "X", "role": "executive", "password": "short"}
	wantStatus(t, call(t, http.MethodPost, "/api/staff/users", headCookie, weak), http.StatusBadRequest)
	rec := call(t, http.MethodPost, "/api/staff/users", headCookie, newExec)
	wantStatus(t, rec, http.StatusCreated)
	created := decode[struct{ ID string }](t, rec)
	wantStatus(t, call(t, http.MethodPost, "/api/staff/users", headCookie, newExec), http.StatusConflict)

	// The new executive can sign in with the first password.
	login := call(t, http.MethodPost, "/api/auth/login", nil, map[string]string{"email": email, "password": "first-pass-123"})
	wantStatus(t, login, http.StatusOK)
	var their *http.Cookie
	for _, c := range login.Result().Cookies() {
		if c.Name == auth.CookieName {
			their = c
		}
	}
	wantStatus(t, call(t, http.MethodGet, "/api/auth/me", their, nil), http.StatusOK)

	// A password reset signs them out everywhere.
	wantStatus(t, call(t, http.MethodPost, "/api/staff/users/"+created.ID+"/password", headCookie,
		map[string]string{"password": "second-pass-456"}), http.StatusNoContent)
	wantStatus(t, call(t, http.MethodGet, "/api/auth/me", their, nil), http.StatusUnauthorized)

	// Switching them off: login refused, and the head cannot switch themself off.
	wantStatus(t, call(t, http.MethodPost, "/api/staff/users/"+created.ID+"/active", headCookie,
		map[string]bool{"active": false}), http.StatusOK)
	wantStatus(t, call(t, http.MethodPost, "/api/auth/login", nil,
		map[string]string{"email": email, "password": "second-pass-456"}), http.StatusUnauthorized)
	wantStatus(t, call(t, http.MethodPost, "/api/staff/users/"+headID+"/active", headCookie,
		map[string]bool{"active": false}), http.StatusBadRequest)

	list := call(t, http.MethodGet, "/api/staff/users", headCookie, nil)
	if !strings.Contains(list.Body.String(), email) || strings.Contains(list.Body.String(), "password") {
		t.Errorf("user list must include the new user and no password data: %s", list.Body.String())
	}
}

// ---- staff lists, lead status and opt-outs ---------------------------------------

func TestStaffListsShowExecutivesOnlyTheirOwn(t *testing.T) {
	requireDB(t)
	aID, aCookie := newStaff(t, models.RoleExecutive)
	bID, _ := newStaff(t, models.RoleExecutive)
	_, headCookie := newStaff(t, models.RoleHead)
	leadA, _ := newLead(t, aID)
	leadB, _ := newLead(t, bID)
	slotA, slotB := newSlot(t, aID), newSlot(t, bID)
	wantStatus(t, call(t, http.MethodPost, "/api/staff/visits", headCookie, map[string]string{"leadId": leadA, "slotId": slotA}), http.StatusCreated)
	wantStatus(t, call(t, http.MethodPost, "/api/staff/visits", headCookie, map[string]string{"leadId": leadB, "slotId": slotB}), http.StatusCreated)

	// newSlot puts slots a few days ahead, so the next 60 days cover them.
	today := time.Now().In(booking.IST)
	q := "?from=" + today.Format(time.DateOnly) + "&to=" + today.AddDate(0, 0, 60).Format(time.DateOnly)
	for _, path := range []string{"/api/staff/visits" + q, "/api/staff/slots" + q} {
		mine := call(t, http.MethodGet, path, aCookie, nil)
		wantStatus(t, mine, http.StatusOK)
		if strings.Contains(mine.Body.String(), bID) {
			t.Errorf("%s: executive A can see executive B's data", path)
		}
	}
	all := call(t, http.MethodGet, "/api/staff/visits"+q, headCookie, nil)
	if !strings.Contains(all.Body.String(), aID) || !strings.Contains(all.Body.String(), bID) {
		t.Error("the head should see every executive's visits")
	}
	wantStatus(t, call(t, http.MethodGet, "/api/staff/reminders", aCookie, nil), http.StatusOK)
	wantStatus(t, call(t, http.MethodGet, "/api/staff/reminders?status=lost", aCookie, nil), http.StatusBadRequest)
}

func TestLeadStatusAndStaffOptOut(t *testing.T) {
	requireDB(t)
	ownerID, ownerCookie := newStaff(t, models.RoleExecutive)
	_, otherCookie := newStaff(t, models.RoleExecutive)
	lead, _ := newLead(t, ownerID)

	wantStatus(t, call(t, http.MethodPost, "/api/staff/leads/"+lead+"/status", otherCookie, map[string]string{"status": "lost"}), http.StatusNotFound)
	wantStatus(t, call(t, http.MethodPost, "/api/staff/leads/"+lead+"/status", ownerCookie, map[string]string{"status": "visited"}), http.StatusBadRequest)
	wantStatus(t, call(t, http.MethodPost, "/api/staff/leads/"+lead+"/status", ownerCookie, map[string]string{"status": "lost"}), http.StatusNoContent)

	wantStatus(t, call(t, http.MethodPost, "/api/staff/leads/"+lead+"/opt-out", otherCookie, nil), http.StatusNotFound)
	wantStatus(t, call(t, http.MethodPost, "/api/staff/leads/"+lead+"/opt-out", ownerCookie, nil), http.StatusNoContent)
	var optedOut bool
	if err := testPool.QueryRow(context.Background(), `SELECT opted_out FROM leads WHERE id = $1::uuid`, lead).Scan(&optedOut); err != nil || !optedOut {
		t.Fatalf("lead not opted out (err %v)", err)
	}
}

func TestBuyerOptsOutFromTheVisitPage(t *testing.T) {
	requireDB(t)
	execID, _ := newStaff(t, models.RoleExecutive)
	_, headCookie := newStaff(t, models.RoleHead)
	lead, _ := newLead(t, execID)
	rec := call(t, http.MethodPost, "/api/staff/visits", headCookie, map[string]string{"leadId": lead, "slotId": newSlot(t, execID)})
	wantStatus(t, rec, http.StatusCreated)
	visit := decode[models.Visit](t, rec)

	out := call(t, http.MethodPost, "/api/visits/"+visit.Code+"/opt-out", nil, nil)
	wantStatus(t, out, http.StatusOK)
	if !strings.Contains(out.Body.String(), `"optedOut":true`) {
		t.Errorf("visit page should now show opted out: %s", out.Body.String())
	}
	var queued int
	if err := testPool.QueryRow(context.Background(), `SELECT count(*) FROM reminders WHERE visit_id = $1::uuid AND status = 'queued'`, visit.ID).Scan(&queued); err != nil || queued != 0 {
		t.Fatalf("%d reminders still queued (err %v)", queued, err)
	}
	wantStatus(t, call(t, http.MethodPost, "/api/visits/"+strings.Repeat("0", 24)+"/opt-out", nil, nil), http.StatusNotFound)
}

func TestInboundStopWebhook(t *testing.T) {
	requireDB(t)
	// The shared test router has no secret, so the endpoint does not exist.
	wantStatus(t, call(t, http.MethodPost, "/api/webhooks/inbound", nil, map[string]string{"from": "x", "text": "STOP"}), http.StatusNotFound)

	withSecret := httpapi.NewRouter(config.Config{AppEnv: "development", InboundWebhookSecret: "s3cret-value"}, testPool, model, &background)
	send := func(secret, from, text string) *httptest.ResponseRecorder {
		req := newJSONRequest(t, http.MethodPost, "/api/webhooks/inbound", nil, map[string]string{"from": from, "text": text})
		if secret != "" {
			req.Header.Set("X-Webhook-Secret", secret)
		}
		rec := httptest.NewRecorder()
		withSecret.ServeHTTP(rec, req)
		return rec
	}
	lead, phone := newLead(t, "")
	wantStatus(t, send("", phone, "STOP"), http.StatusUnauthorized)
	wantStatus(t, send("wrong", phone, "STOP"), http.StatusUnauthorized)

	if rec := send("s3cret-value", phone, "what time is my visit?"); !strings.Contains(rec.Body.String(), `"optedOut":false`) {
		t.Errorf("a normal reply must not opt out: %s", rec.Body.String())
	}
	if rec := send("s3cret-value", phone, " stop "); !strings.Contains(rec.Body.String(), `"optedOut":true`) {
		t.Errorf("STOP must opt out: %s", rec.Body.String())
	}
	var optedOut bool
	if err := testPool.QueryRow(context.Background(), `SELECT opted_out FROM leads WHERE id = $1::uuid`, lead).Scan(&optedOut); err != nil || !optedOut {
		t.Fatal("lead not opted out after STOP")
	}
}

// ---- serving the web app ---------------------------------------------------------

func TestServesTheWebAppWithFallbackAndCSP(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<!doctype html><div id=root></div>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "assets", "app-3f2a.js"), []byte("console.log(1)"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := httpapi.NewRouter(config.Config{AppEnv: "development", StaticDir: dir}, testPool, model, &background)
	get := func(method, path string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
		return rec
	}

	for _, path := range []string{"/", "/visit/abc123", "/staff/leads"} {
		rec := get(http.MethodGet, path)
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "id=root") {
			t.Errorf("%s should serve index.html, got %d", path, rec.Code)
		}
		if csp := rec.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "script-src 'self'") {
			t.Errorf("%s: missing CSP, got %q", path, csp)
		}
	}
	asset := get(http.MethodGet, "/assets/app-3f2a.js")
	if asset.Code != http.StatusOK || !strings.Contains(asset.Header().Get("Cache-Control"), "immutable") {
		t.Errorf("asset: %d %q", asset.Code, asset.Header().Get("Cache-Control"))
	}
	if rec := get(http.MethodGet, "/../../etc/passwd"); strings.Contains(rec.Body.String(), "root:") {
		t.Error("path traversal must not escape the static folder")
	}
	if rec := get(http.MethodGet, "/api/does-not-exist"); strings.Contains(rec.Body.String(), "id=root") {
		t.Error("unknown API paths must not fall back to the web app")
	}
	if rec := get(http.MethodPost, "/somewhere"); rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST to a page: %d", rec.Code)
	}
}
