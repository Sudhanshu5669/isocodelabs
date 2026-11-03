package leads_test

import (
	"testing"
	"time"

	"leadflow/internal/leads"
	"leadflow/internal/models"
)

func ptr[T any](v T) *T { return &v }

func TestMergeFillsGapsAndKeepsIdentity(t *testing.T) {
	keep := models.Lead{ID: "keep", Code: "LD-1", Phone: "+919825012345", Source: "website", Status: models.LeadNew}
	drop := models.Lead{
		ID: "drop", Code: "LD-2", Phone: "+919825099999", Source: "social_ad", Status: models.LeadNew,
		ProjectID: ptr("project-1"), Wants: ptr("3 BHK"),
		BudgetMinLakh: ptr(95), BudgetMaxLakh: ptr(120), AssignedTo: ptr("exec-1"),
	}

	got := leads.Merge(keep, drop)

	if got.ID != "keep" || got.Code != "LD-1" || got.Phone != "+919825012345" || got.Source != "website" {
		t.Errorf("identity changed: %+v", got)
	}
	if got.Wants == nil || *got.Wants != "3 BHK" || got.ProjectID == nil || *got.ProjectID != "project-1" {
		t.Errorf("gaps not filled: %+v", got)
	}
	if got.BudgetMinLakh == nil || *got.BudgetMinLakh != 95 || got.AssignedTo == nil || *got.AssignedTo != "exec-1" {
		t.Errorf("budget or assignee not filled: %+v", got)
	}
}

func TestMergeKeepsExistingValues(t *testing.T) {
	keep := models.Lead{Wants: ptr("2 BHK"), AssignedTo: ptr("exec-1")}
	drop := models.Lead{Wants: ptr("3 BHK"), AssignedTo: ptr("exec-2")}

	got := leads.Merge(keep, drop)
	if *got.Wants != "2 BHK" || *got.AssignedTo != "exec-1" {
		t.Errorf("the kept lead's values should win: %+v", got)
	}
}

func TestMergeConsentAndOptOut(t *testing.T) {
	t0 := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)
	t1 := t0.Add(time.Hour)

	tests := []struct {
		name                string
		keep, drop          models.Lead
		wantConsent, wantOO bool
	}{
		{"consent only on the duplicate counts", models.Lead{}, models.Lead{Consent: true, ConsentAt: &t1}, true, false},
		{"opt-out on either record wins", models.Lead{Consent: true}, models.Lead{Consent: true, OptedOut: true, OptedOutAt: &t1}, true, true},
		{"opt-out on the kept record stays", models.Lead{Consent: true, OptedOut: true, OptedOutAt: &t0}, models.Lead{Consent: true}, true, true},
		{"neither", models.Lead{}, models.Lead{}, false, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := leads.Merge(tc.keep, tc.drop)
			if got.Consent != tc.wantConsent || got.OptedOut != tc.wantOO {
				t.Errorf("Consent=%v OptedOut=%v, want %v %v", got.Consent, got.OptedOut, tc.wantConsent, tc.wantOO)
			}
		})
	}
}

func TestMergeKeepsEarliestTimes(t *testing.T) {
	early := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)
	late := early.Add(3 * time.Hour)

	got := leads.Merge(models.Lead{FirstResponseAt: &late}, models.Lead{FirstResponseAt: &early})
	if !got.FirstResponseAt.Equal(early) {
		t.Errorf("FirstResponseAt = %v, want the earlier %v", got.FirstResponseAt, early)
	}
	got = leads.Merge(models.Lead{}, models.Lead{FirstResponseAt: &late})
	if got.FirstResponseAt == nil || !got.FirstResponseAt.Equal(late) {
		t.Errorf("FirstResponseAt should be filled from the duplicate, got %v", got.FirstResponseAt)
	}
}

func TestMergeStatus(t *testing.T) {
	tests := []struct{ keep, drop, want string }{
		{models.LeadNew, models.LeadEngaged, models.LeadEngaged},
		{models.LeadEngaged, models.LeadNew, models.LeadEngaged},
		{models.LeadLost, models.LeadNew, models.LeadNew},
		{models.LeadVisited, models.LeadVisitBooked, models.LeadVisitBooked}, // a live booking outranks a past visit
		{models.LeadVisitBooked, models.LeadVisited, models.LeadVisitBooked},
		{models.LeadEngaged, models.LeadEngaged, models.LeadEngaged},
	}
	for _, tc := range tests {
		got := leads.Merge(models.Lead{Status: tc.keep}, models.Lead{Status: tc.drop})
		if got.Status != tc.want {
			t.Errorf("Merge(%s, %s).Status = %s, want %s", tc.keep, tc.drop, got.Status, tc.want)
		}
	}
}
