package store_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"leadflow/internal/leads"
	"leadflow/internal/store"
)

// ---- helpers ---------------------------------------------------------------

// uniquePhone returns a valid, random Indian mobile number.
func uniquePhone() string { return fmt.Sprintf("+919%09d", randInt()%1_000_000_000) }

// uniqueName returns a two-word name that is not similar to any other test name.
func uniqueName() string { return randHex(4) + " " + randHex(4) }

func baseInput() leads.Input {
	return leads.Input{Name: uniqueName(), Phone: uniquePhone(), Source: "website", ProjectID: "project-1", Consent: true}
}

// ingest cleans the input the way the HTTP layer does, then saves it.
func ingest(t *testing.T, in leads.Input, assignTo string) store.IngestResult {
	t.Helper()
	clean, err := in.Clean()
	if err != nil {
		t.Fatal(err)
	}
	res, err := testStore.IngestLead(context.Background(), clean, assignTo)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func newHead(t *testing.T) string {
	t.Helper()
	return scalar[string](t, `
		INSERT INTO users (email, name, role, password_hash) VALUES ($1, 'Test Head', 'head', 'x') RETURNING id::text`,
		randHex(6)+"@test.local")
}

func headViewer() store.Viewer          { return store.Viewer{ID: newHeadID, IsHead: true} }
func execViewer(id string) store.Viewer { return store.Viewer{ID: id} }

// The head's id only matters to be a valid uuid; the head's view ignores it.
const newHeadID = "00000000-0000-0000-0000-000000000001"

func touchCount(t *testing.T, leadID string) int {
	t.Helper()
	return scalar[int](t, `SELECT count(*) FROM lead_touches WHERE lead_id = $1::uuid`, leadID)
}

// ---- exact duplicates (same phone) -----------------------------------------

func TestIngestCreatesLeadAndTouch(t *testing.T) {
	requireDB(t)
	res := ingest(t, baseInput(), "")

	if res.Outcome != store.OutcomeCreated {
		t.Fatalf("Outcome = %q, want created", res.Outcome)
	}
	if len(res.Lead.Code) < 4 || res.Lead.Code[:3] != "LD-" {
		t.Errorf("Code = %q, want LD-...", res.Lead.Code)
	}
	if res.Lead.ConsentAt == nil || !res.Lead.Consent {
		t.Errorf("consent should be recorded with a time: %+v", res.Lead)
	}
	if res.Lead.AssignedTo != nil {
		t.Errorf("a new lead with no owner should be unassigned")
	}
	if n := touchCount(t, res.Lead.ID); n != 1 {
		t.Errorf("touches = %d, want 1", n)
	}
}

func TestIngestSamePhoneMergesIntoOneLead(t *testing.T) {
	requireDB(t)
	owner, other := newExecutive(t), newExecutive(t)

	first := baseInput()
	first.Source, first.ProjectID, first.Consent = "property_portal", "", false
	a := ingest(t, first, owner)

	// The same person, typed differently, arriving through another channel.
	national := first.Phone[3:]
	second := leads.Input{
		Name: "Someone Else", Phone: "+91 " + national[:5] + " " + national[5:],
		Source: "social_ad", ProjectID: "project-1", Wants: "3 BHK", Consent: true,
	}
	b := ingest(t, second, other)

	if b.Outcome != store.OutcomeMerged {
		t.Fatalf("Outcome = %q, want merged", b.Outcome)
	}
	l := b.Lead
	switch {
	case l.ID != a.Lead.ID:
		t.Errorf("a repeat enquiry created a second lead")
	case l.Source != "property_portal":
		t.Errorf("Source = %q, the first source must be kept", l.Source)
	case l.Name != first.Name:
		t.Errorf("Name = %q, the first name must be kept", l.Name)
	case l.ProjectID == nil || *l.ProjectID != "project-1" || l.Wants == nil || *l.Wants != "3 BHK":
		t.Errorf("missing details should be filled in: %+v", l)
	case !l.Consent || l.ConsentAt == nil:
		t.Errorf("the new consent should be recorded: %+v", l)
	case l.AssignedTo == nil || *l.AssignedTo != owner:
		t.Errorf("a repeat enquiry must not change the owner: %v", l.AssignedTo)
	}

	d, err := testStore.GetLead(context.Background(), headViewer(), a.Lead.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Touches) != 2 || d.Touches[0].Source != "property_portal" || d.Touches[1].Source != "social_ad" {
		t.Errorf("touches = %+v, want property_portal then social_ad", d.Touches)
	}
}

func TestIngestNeverClearsAnOptOut(t *testing.T) {
	requireDB(t)
	in := baseInput()
	a := ingest(t, in, "")
	if _, err := testPool.Exec(context.Background(), `UPDATE leads SET opted_out = true, opted_out_at = now() WHERE id = $1::uuid`, a.Lead.ID); err != nil {
		t.Fatal(err)
	}

	b := ingest(t, in, "")
	if !b.Lead.OptedOut {
		t.Error("a new enquiry must not clear an opt-out")
	}
}

func TestIngestConcurrentSamePhoneMakesOneLead(t *testing.T) {
	requireDB(t)
	in := baseInput()
	clean, err := in.Clean()
	if err != nil {
		t.Fatal(err)
	}

	const n = 20
	var wg sync.WaitGroup
	outcomes := make(chan string, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, err := testStore.IngestLead(context.Background(), clean, "")
			if err != nil {
				t.Error(err)
				return
			}
			outcomes <- res.Outcome
		}()
	}
	wg.Wait()
	close(outcomes)

	created, merged := 0, 0
	for o := range outcomes {
		if o == store.OutcomeCreated {
			created++
		} else {
			merged++
		}
	}
	if created != 1 || merged != n-1 {
		t.Errorf("created=%d merged=%d, want 1 and %d", created, merged, n-1)
	}
	if rows := scalar[int](t, `SELECT count(*) FROM leads WHERE phone = $1`, clean.Phone); rows != 1 {
		t.Errorf("%d lead rows for one phone number, want 1", rows)
	}
}

func TestIngestUnknownProjectIsAValidationError(t *testing.T) {
	requireDB(t)
	in := baseInput()
	in.ProjectID = "project-9"
	clean, err := in.Clean()
	if err != nil {
		t.Fatal(err)
	}

	_, err = testStore.IngestLead(context.Background(), clean, "")
	var ve *leads.ValidationError
	if !errors.As(err, &ve) || ve.Field != "projectId" {
		t.Fatalf("err = %v, want a projectId validation error", err)
	}
	if rows := scalar[int](t, `SELECT count(*) FROM leads WHERE phone = $1`, clean.Phone); rows != 0 {
		t.Error("a rejected enquiry must leave nothing behind")
	}
}

// ---- fuzzy duplicates (similar name) ---------------------------------------

func TestIngestFlagsLookalikesInTheSameProjectOnly(t *testing.T) {
	requireDB(t)
	orig := baseInput()
	first := ingest(t, orig, "")

	t.Run("same name, same project is flagged", func(t *testing.T) {
		in := baseInput()
		in.Name = orig.Name
		res := ingest(t, in, "")
		if res.Outcome != store.OutcomeCreated {
			t.Fatalf("a different phone is a new lead, got %q", res.Outcome)
		}
		if res.DuplicateOfCode != first.Lead.Code || res.Lead.PossibleDuplicateOf == nil || *res.Lead.PossibleDuplicateOf != first.Lead.ID {
			t.Errorf("not flagged against the original: %+v", res)
		}
	})

	t.Run("a later look-alike points at the oldest original", func(t *testing.T) {
		in := baseInput()
		in.Name = orig.Name
		res := ingest(t, in, "")
		if res.DuplicateOfCode != first.Lead.Code {
			t.Errorf("DuplicateOfCode = %q, want the original %q", res.DuplicateOfCode, first.Lead.Code)
		}
	})

	t.Run("same name, other project is not flagged", func(t *testing.T) {
		in := baseInput()
		in.Name, in.ProjectID = orig.Name, "project-2"
		if res := ingest(t, in, ""); res.DuplicateOfCode != "" {
			t.Errorf("flagged across projects: %+v", res)
		}
	})

	t.Run("a different name is not flagged", func(t *testing.T) {
		if res := ingest(t, baseInput(), ""); res.DuplicateOfCode != "" {
			t.Errorf("flagged an unrelated name: %+v", res)
		}
	})
}

// ---- who sees what, and filters --------------------------------------------

// leadSet creates four leads sharing a search marker, owned by two executives
// and one unassigned, and returns the marker and their ids.
type leadSet struct {
	marker         string
	execA, execB   string
	l1, l2, l3, l4 string
	l1Phone        string
}

func newLeadSet(t *testing.T) leadSet {
	t.Helper()
	s := leadSet{marker: randHex(5), execA: newExecutive(t), execB: newExecutive(t)}
	mk := func(source, project, owner string) (string, string) {
		in := leads.Input{Name: s.marker + " " + randHex(4), Phone: uniquePhone(), Source: source, ProjectID: project}
		res := ingest(t, in, owner)
		return res.Lead.ID, res.Lead.Phone
	}
	s.l1, s.l1Phone = mk("website", "project-1", s.execA)
	s.l2, _ = mk("social_ad", "project-2", s.execA)
	s.l3, _ = mk("website", "project-1", s.execB)
	s.l4, _ = mk("hoarding_call", "project-1", "")
	return s
}

func list(t *testing.T, v store.Viewer, f store.LeadFilter) store.LeadPage {
	t.Helper()
	p, err := testStore.ListLeads(context.Background(), v, f)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestListLeadsVisibility(t *testing.T) {
	requireDB(t)
	s := newLeadSet(t)
	q := store.LeadFilter{Query: s.marker}

	if got := list(t, headViewer(), q).Total; got != 4 {
		t.Errorf("head sees %d, want all 4", got)
	}
	if got := list(t, execViewer(s.execA), q).Total; got != 2 {
		t.Errorf("executive A sees %d, want their 2", got)
	}
	if got := list(t, execViewer(s.execB), q).Total; got != 1 {
		t.Errorf("executive B sees %d, want their 1", got)
	}

	// An executive cannot widen their view with the assignee filter.
	f := store.LeadFilter{Query: s.marker, Assignee: s.execB}
	if got := list(t, execViewer(s.execA), f).Total; got != 2 {
		t.Errorf("executive A with another assignee filter sees %d, want still 2", got)
	}
}

func TestListLeadsFilters(t *testing.T) {
	requireDB(t)
	s := newLeadSet(t)
	head := headViewer()

	tests := []struct {
		name string
		f    store.LeadFilter
		want int
	}{
		{"by source", store.LeadFilter{Source: "social_ad"}, 1},
		{"by project", store.LeadFilter{ProjectID: "project-1"}, 3},
		{"by status", store.LeadFilter{Status: "visited"}, 0},
		{"unassigned", store.LeadFilter{Assignee: "unassigned"}, 1},
		{"one executive", store.LeadFilter{Assignee: s.execA}, 2},
		{"by lead phone", store.LeadFilter{Query: s.l1Phone[3:]}, 1},
		{"literal percent matches nothing", store.LeadFilter{Query: "%"}, 0},
		{"needs attention: nobody replied yet", store.LeadFilter{NeedsAttention: true}, 4},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := tc.f
			if f.Query == "" {
				f.Query = s.marker // scope to this test's leads
			}
			if got := list(t, head, f).Total; got != tc.want {
				t.Errorf("Total = %d, want %d", got, tc.want)
			}
		})
	}

	// A lead that has had its first reply no longer needs attention.
	if _, err := testPool.Exec(context.Background(), `UPDATE leads SET first_response_at = now() WHERE id = $1::uuid`, s.l2); err != nil {
		t.Fatal(err)
	}
	if got := list(t, head, store.LeadFilter{Query: s.marker, NeedsAttention: true}).Total; got != 3 {
		t.Errorf("after one reply, needs attention = %d, want 3", got)
	}
}

func TestListLeadsPaging(t *testing.T) {
	requireDB(t)
	s := newLeadSet(t)

	p := list(t, headViewer(), store.LeadFilter{Query: s.marker, Limit: 3})
	if len(p.Leads) != 3 || p.Total != 4 {
		t.Fatalf("page 1: %d rows, total %d; want 3 and 4", len(p.Leads), p.Total)
	}
	p2 := list(t, headViewer(), store.LeadFilter{Query: s.marker, Limit: 3, Offset: 3})
	if len(p2.Leads) != 1 {
		t.Fatalf("page 2 has %d rows, want 1", len(p2.Leads))
	}
	for _, r := range p.Leads {
		if r.ID == p2.Leads[0].ID {
			t.Error("the same lead appeared on both pages")
		}
	}
}

func TestGetLeadAccess(t *testing.T) {
	requireDB(t)
	s := newLeadSet(t)
	ctx := context.Background()

	if d, err := testStore.GetLead(ctx, execViewer(s.execA), s.l1); err != nil || d.ID != s.l1 || len(d.Touches) != 1 {
		t.Errorf("owner: err=%v detail=%+v", err, d.LeadRow)
	}
	if _, err := testStore.GetLead(ctx, execViewer(s.execB), s.l1); !errors.Is(err, leads.ErrNotFound) {
		t.Errorf("another executive: err = %v, want ErrNotFound", err)
	}
	if _, err := testStore.GetLead(ctx, execViewer(s.execA), s.l4); !errors.Is(err, leads.ErrNotFound) {
		t.Errorf("executive reading an unassigned lead: err = %v, want ErrNotFound", err)
	}
	if _, err := testStore.GetLead(ctx, headViewer(), s.l4); err != nil {
		t.Errorf("head: %v", err)
	}
	if _, err := testStore.GetLead(ctx, headViewer(), "00000000-0000-0000-0000-0000000000ff"); !errors.Is(err, leads.ErrNotFound) {
		t.Errorf("unknown id: err = %v, want ErrNotFound", err)
	}
}

func TestGetLeadShowsVisitAndDuplicate(t *testing.T) {
	requireDB(t)
	exec := newExecutive(t)
	orig := ingest(t, baseInput(), exec)
	in := baseInput()
	in.Name = orig.Lead.Name
	dup := ingest(t, in, exec)

	slot := newSlot(t, exec, futureStart(5))
	if _, err := testStore.BookSlot(context.Background(), time.Now(), dup.Lead.ID, slot); err != nil {
		t.Fatal(err)
	}

	d, err := testStore.GetLead(context.Background(), execViewer(exec), dup.Lead.ID)
	if err != nil {
		t.Fatal(err)
	}
	if d.Visit == nil || d.Visit.Status != "booked" {
		t.Errorf("Visit = %+v, want a booked visit", d.Visit)
	}
	if d.DuplicateOf == nil || d.DuplicateOf.Code != orig.Lead.Code {
		t.Errorf("DuplicateOf = %+v, want %s", d.DuplicateOf, orig.Lead.Code)
	}
}

func TestAssignLead(t *testing.T) {
	requireDB(t)
	ctx := context.Background()
	exec := newExecutive(t)
	lead := ingest(t, baseInput(), "")

	if err := testStore.AssignLead(ctx, lead.Lead.ID, exec); err != nil {
		t.Fatal(err)
	}
	if d, err := testStore.GetLead(ctx, execViewer(exec), lead.Lead.ID); err != nil || d.AssigneeName == nil {
		t.Errorf("assigned executive should now see the lead: err=%v", err)
	}
	if err := testStore.AssignLead(ctx, lead.Lead.ID, newHead(t)); !errors.Is(err, leads.ErrAssigneeNotFound) {
		t.Errorf("assigning to a non-executive: err = %v, want ErrAssigneeNotFound", err)
	}
	if err := testStore.AssignLead(ctx, "00000000-0000-0000-0000-0000000000ff", exec); !errors.Is(err, leads.ErrNotFound) {
		t.Errorf("unknown lead: err = %v, want ErrNotFound", err)
	}
}

// ---- resolving flagged duplicates ------------------------------------------

// flaggedPair returns an original lead and a later duplicate flagged against it.
func flaggedPair(t *testing.T) (orig, dup store.IngestResult) {
	t.Helper()
	first := baseInput()
	first.Consent = false
	orig = ingest(t, first, "")

	second := baseInput()
	second.Name, second.Source, second.Wants, second.Consent = first.Name, "social_ad", "3 BHK", true
	dup = ingest(t, second, "")
	if dup.Lead.PossibleDuplicateOf == nil {
		t.Fatal("setup: the second lead should have been flagged")
	}
	return orig, dup
}

func TestMergeDuplicateMovesHistoryAndRemovesTheDuplicate(t *testing.T) {
	requireDB(t)
	ctx := context.Background()
	exec := newExecutive(t)
	orig, dup := flaggedPair(t)

	// Give the duplicate a visit and a chat, so we can see them move.
	slot := newSlot(t, exec, futureStart(6))
	visit, err := testStore.BookSlot(ctx, time.Now(), dup.Lead.ID, slot)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(ctx, `INSERT INTO conversations (lead_id) VALUES ($1::uuid)`, dup.Lead.ID); err != nil {
		t.Fatal(err)
	}

	keepID, err := testStore.MergeDuplicate(ctx, dup.Lead.ID)
	if err != nil {
		t.Fatal(err)
	}
	if keepID != orig.Lead.ID {
		t.Fatalf("kept %s, want the original %s", keepID, orig.Lead.ID)
	}

	if n := scalar[int](t, `SELECT count(*) FROM leads WHERE id = $1::uuid`, dup.Lead.ID); n != 0 {
		t.Error("the duplicate should be deleted")
	}
	if got := scalar[string](t, `SELECT lead_id::text FROM visits WHERE id = $1::uuid`, visit.ID); got != orig.Lead.ID {
		t.Error("the visit should now belong to the original")
	}
	if n := scalar[int](t, `SELECT count(*) FROM conversations WHERE lead_id = $1::uuid`, orig.Lead.ID); n != 1 {
		t.Errorf("conversations on the original = %d, want 1", n)
	}
	// original's first enquiry + duplicate's first enquiry + the merge note
	if n := touchCount(t, orig.Lead.ID); n != 3 {
		t.Errorf("touches = %d, want 3", n)
	}

	d, err := testStore.GetLead(ctx, headViewer(), orig.Lead.ID)
	if err != nil {
		t.Fatal(err)
	}
	switch {
	case d.Phone != orig.Lead.Phone || d.Source != "website":
		t.Errorf("the original's identity changed: %+v", d.LeadRow)
	case d.Wants == nil || *d.Wants != "3 BHK":
		t.Errorf("Wants should come from the duplicate: %v", d.Wants)
	case !d.Consent:
		t.Error("consent given on the duplicate should count")
	case d.Status != "visit_booked":
		t.Errorf("Status = %q, want visit_booked", d.Status)
	case d.DuplicateOf != nil:
		t.Error("the merged lead should no longer be flagged")
	}
}

func TestMergeDuplicateRefusesWhenBothHaveABookedVisit(t *testing.T) {
	requireDB(t)
	ctx := context.Background()
	exec := newExecutive(t)
	orig, dup := flaggedPair(t)
	for i, id := range []string{orig.Lead.ID, dup.Lead.ID} {
		if _, err := testStore.BookSlot(ctx, time.Now(), id, newSlot(t, exec, futureStart(7+i))); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := testStore.MergeDuplicate(ctx, dup.Lead.ID); !errors.Is(err, leads.ErrMergeConflict) {
		t.Fatalf("err = %v, want ErrMergeConflict", err)
	}
	if n := scalar[int](t, `SELECT count(*) FROM leads WHERE id = $1::uuid AND possible_duplicate_of IS NOT NULL`, dup.Lead.ID); n != 1 {
		t.Error("a refused merge must change nothing")
	}
}

func TestMergeDuplicateRepointsOtherFlags(t *testing.T) {
	requireDB(t)
	ctx := context.Background()
	orig, dup := flaggedPair(t)
	third := ingest(t, baseInput(), "")
	if _, err := testPool.Exec(ctx, `UPDATE leads SET possible_duplicate_of = $2::uuid WHERE id = $1::uuid`, third.Lead.ID, dup.Lead.ID); err != nil {
		t.Fatal(err)
	}

	if _, err := testStore.MergeDuplicate(ctx, dup.Lead.ID); err != nil {
		t.Fatal(err)
	}
	if got := scalar[string](t, `SELECT possible_duplicate_of::text FROM leads WHERE id = $1::uuid`, third.Lead.ID); got != orig.Lead.ID {
		t.Errorf("a lead flagged against the merged duplicate should now point at the original")
	}
}

func TestResolveDuplicateErrors(t *testing.T) {
	requireDB(t)
	ctx := context.Background()
	plain := ingest(t, baseInput(), "")
	missing := "00000000-0000-0000-0000-0000000000ff"

	if _, err := testStore.MergeDuplicate(ctx, plain.Lead.ID); !errors.Is(err, leads.ErrNotDuplicate) {
		t.Errorf("merge of an unflagged lead: err = %v, want ErrNotDuplicate", err)
	}
	if _, err := testStore.MergeDuplicate(ctx, missing); !errors.Is(err, leads.ErrNotFound) {
		t.Errorf("merge of an unknown lead: err = %v, want ErrNotFound", err)
	}
	if err := testStore.DismissDuplicate(ctx, plain.Lead.ID); !errors.Is(err, leads.ErrNotDuplicate) {
		t.Errorf("dismiss of an unflagged lead: err = %v, want ErrNotDuplicate", err)
	}
	if err := testStore.DismissDuplicate(ctx, missing); !errors.Is(err, leads.ErrNotFound) {
		t.Errorf("dismiss of an unknown lead: err = %v, want ErrNotFound", err)
	}
}

func TestDismissDuplicateKeepsBothLeads(t *testing.T) {
	requireDB(t)
	ctx := context.Background()
	orig, dup := flaggedPair(t)

	if err := testStore.DismissDuplicate(ctx, dup.Lead.ID); err != nil {
		t.Fatal(err)
	}
	if n := scalar[int](t, `SELECT count(*) FROM leads WHERE id IN ($1::uuid, $2::uuid) AND possible_duplicate_of IS NULL`, orig.Lead.ID, dup.Lead.ID); n != 2 {
		t.Errorf("both leads should remain, unflagged; found %d", n)
	}
}
