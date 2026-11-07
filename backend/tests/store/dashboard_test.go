package store_test

import (
	"context"
	"testing"
	"time"

	"leadflow/internal/booking"
	"leadflow/internal/models"
	"leadflow/internal/pii"
	"leadflow/internal/store"
)

// The dashboard is checked on a small, hand-made data set in January 2020, a
// month no other test writes to, so every expected number can be worked out
// by hand.
//
//	lead  source           arrived (IST)   first reply   visit
//	A     social_ad        10 Jan 10:00    2 min         visited
//	B     social_ad        11 Jan 10:00    10 min        no-show
//	C     property_portal  12 Jan 10:00    never         -
//	D     property_portal  20 Jan 10:00    3 days        -
//	spend: social_ad ₹10,000 in January 2020
func TestDashboardNumbers(t *testing.T) {
	requireDB(t)
	ctx := context.Background()
	ist := booking.IST
	exec := newExecutive(t)

	day := func(d int) time.Time { return time.Date(2020, 1, d, 10, 0, 0, 0, ist) }
	addLead := func(source string, created time.Time, reply time.Duration, visitStatus string, slotDay int) {
		t.Helper()
		var replied any
		if reply > 0 {
			replied = created.Add(reply)
		}
		var id string
		phone := uniquePhone()
		if err := testPool.QueryRow(ctx, `
			INSERT INTO leads (name, phone, phone_bidx, source, project_id, consent, created_at, first_response_at)
			VALUES ($1, $2, $3, $4, 'project-1', true, $5, $6) RETURNING id::text`,
			pii.Seal(pii.Name, "Dash Lead"), pii.Seal(pii.Phone, phone), pii.PhoneIndex(phone),
			source, created, replied).Scan(&id); err != nil {
			t.Fatal(err)
		}
		if visitStatus == "" {
			return
		}
		start := day(slotDay)
		var slot string
		if err := testPool.QueryRow(ctx, `
			INSERT INTO visit_slots (project_id, executive_id, starts_at, ends_at, status)
			VALUES ('project-1', $1::uuid, $2, $3, 'booked') RETURNING id::text`, exec, start, start.Add(time.Hour)).Scan(&slot); err != nil {
			t.Fatal(err)
		}
		if _, err := testPool.Exec(ctx, `INSERT INTO visits (code, lead_id, slot_id, status) VALUES ($1, $2::uuid, $3::uuid, $4)`,
			randHex(12), id, slot, visitStatus); err != nil {
			t.Fatal(err)
		}
	}
	addLead(models.SourceSocialAd, day(10), 2*time.Minute, models.VisitVisited, 14)
	addLead(models.SourceSocialAd, day(11), 10*time.Minute, models.VisitNoShow, 15)
	addLead(models.SourcePropertyPortal, day(12), 0, "", 0)
	addLead(models.SourcePropertyPortal, day(20), 72*time.Hour, "", 0)
	if _, err := testStore.SaveSpend(ctx, models.SourceSocialAd, time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), 10000); err != nil {
		t.Fatal(err)
	}

	from := time.Date(2020, 1, 1, 0, 0, 0, 0, ist)
	to := time.Date(2020, 2, 1, 0, 0, 0, 0, ist)
	now := time.Date(2020, 2, 15, 0, 0, 0, 0, ist)
	d, err := testStore.DashboardFor(ctx, now, from, to, "", ist)
	if err != nil {
		t.Fatal(err)
	}

	tot := d.Totals
	checkInt(t, "leads", tot.Leads, 4)
	checkInt(t, "responded", tot.Responded, 3)
	checkFloat(t, "median first response (s)", tot.MedianFirstResponseSeconds, 600) // median of 120, 600, 259200
	checkFloat(t, "within 5 min %", tot.RespondedWithin5MinPct, 25)
	checkFloat(t, "unreached in 48h %", tot.UnreachedIn48hPct, 50) // C never, D after 3 days
	checkInt(t, "leads with a visit booked", tot.LeadsWithVisitBooked, 2)
	checkFloat(t, "lead to visit %", tot.LeadToVisitPct, 25)
	checkFloat(t, "show-up rate %", tot.ShowUpRatePct, 50)
	if tot.SpendINR == nil || *tot.SpendINR != 10000 {
		t.Errorf("total spend: %v", tot.SpendINR)
	}

	social := find(t, d.BySource, models.SourceSocialAd)
	checkFloat(t, "social cost per visit", social.CostPerVisitINR, 10000)
	checkFloat(t, "social cost per lead", social.CostPerLeadINR, 5000)
	portal := find(t, d.BySource, models.SourcePropertyPortal)
	if portal.SpendINR != nil || portal.CostPerVisitINR != nil {
		t.Error("no spend recorded for the portal: its cost must be empty, not 0")
	}
	if portal.ShowUpRatePct != nil {
		t.Error("no visits for the portal: show-up rate must be empty, not 0%")
	}

	if len(d.Daily) != 31 || d.Daily[9].Date != "2020-01-10" || d.Daily[9].Leads != 1 || d.Daily[0].Leads != 0 {
		t.Errorf("daily series wrong: %d points, 10 Jan = %+v", len(d.Daily), d.Daily[9])
	}

	// Filtered to a project with no leads: zeros, and no cost figures.
	p2, err := testStore.DashboardFor(ctx, now, from, to, "project-2", ist)
	if err != nil {
		t.Fatal(err)
	}
	if p2.Totals.Leads != 0 || p2.Totals.SpendINR != nil || p2.SpendNote == "" {
		t.Errorf("project filter: %+v note=%q", p2.Totals, p2.SpendNote)
	}
}

func checkInt(t *testing.T, name string, got, want int64) {
	t.Helper()
	if got != want {
		t.Errorf("%s = %d, want %d", name, got, want)
	}
}

func checkFloat(t *testing.T, name string, got *float64, want float64) {
	t.Helper()
	if got == nil || *got < want-0.01 || *got > want+0.01 {
		t.Errorf("%s = %v, want %v", name, deref(got), want)
	}
}

func deref(f *float64) any {
	if f == nil {
		return nil
	}
	return *f
}

func find(t *testing.T, rows []store.Metrics, source string) store.Metrics {
	t.Helper()
	for _, r := range rows {
		if r.Source == source {
			return r
		}
	}
	t.Fatalf("no row for %s", source)
	return store.Metrics{}
}
