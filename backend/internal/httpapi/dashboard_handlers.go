package httpapi

import (
	"net/http"
	"net/url"
	"strconv"
	"time"

	"leadflow/internal/booking"
	"leadflow/internal/models"
)

const maxDashboardDays = 366

// dayRange reads ?from=YYYY-MM-DD&to=YYYY-MM-DD (Ahmedabad days, both
// inclusive). It returns [from, to) as instants. Missing dates default to the
// `defaultDays` days ending today.
func dayRange(q url.Values, today time.Time, defaultDays, maxDays int) (from, to time.Time, msg string) {
	to = today.AddDate(0, 0, 1)
	from = today.AddDate(0, 0, -(defaultDays - 1))
	if v := q.Get("from"); v != "" {
		t, err := time.ParseInLocation(time.DateOnly, v, booking.IST)
		if err != nil {
			return from, to, "The start date should look like 2026-11-03."
		}
		from = t
	}
	if v := q.Get("to"); v != "" {
		t, err := time.ParseInLocation(time.DateOnly, v, booking.IST)
		if err != nil {
			return from, to, "The end date should look like 2026-11-03."
		}
		to = t.AddDate(0, 0, 1)
	}
	if !to.After(from) || to.Sub(from) > time.Duration(maxDays)*24*time.Hour {
		return from, to, "Choose a date range of 1 to " + strconv.Itoa(maxDays) + " days, start before end."
	}
	return from, to, ""
}

// dashboard returns the source dashboard (head only), e.g.
// GET /api/staff/dashboard?from=2026-10-01&to=2026-10-31&project=project-1
// Defaults to the last 30 days, all projects.
func (s *Server) dashboard(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	now := s.now()
	from, to, msg := dayRange(q, startOfDay(now), 30, maxDashboardDays)
	if msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	project := q.Get("project")
	if project != "" && !reProject.MatchString(project) {
		writeError(w, http.StatusBadRequest, "Choose a valid project.")
		return
	}
	d, err := s.store.DashboardFor(r.Context(), now, from, to, project, booking.IST)
	if err != nil {
		s.serverError(w, "dashboard", err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

// spendSources are the sources money is spent on (manual entries cost nothing).
var spendSources = map[string]bool{
	models.SourcePropertyPortal: true,
	models.SourceSocialAd:       true,
	models.SourceWebsite:        true,
	models.SourceHoardingCall:   true,
}

// listSpend returns spend rows, e.g. GET /api/staff/spend?from=2026-06&to=2026-11
// (months, both inclusive; default: the last 6 months).
func (s *Server) listSpend(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	thisMonth := monthOf(s.now())
	from, to := thisMonth.AddDate(0, -5, 0), thisMonth
	var err error
	if v := q.Get("from"); v != "" {
		if from, err = time.Parse("2006-01", v); err != nil {
			writeError(w, http.StatusBadRequest, "Months should look like 2026-11.")
			return
		}
	}
	if v := q.Get("to"); v != "" {
		if to, err = time.Parse("2006-01", v); err != nil {
			writeError(w, http.StatusBadRequest, "Months should look like 2026-11.")
			return
		}
	}
	if to.Before(from) || to.Sub(from) > 3*366*24*time.Hour {
		writeError(w, http.StatusBadRequest, "Choose up to 36 months, start before end.")
		return
	}
	rows, err := s.store.SpendInRange(r.Context(), from, to)
	if err != nil {
		s.serverError(w, "list spend", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"spend": rows})
}

type spendRequest struct {
	Source    string `json:"source"`
	Month     string `json:"month"` // YYYY-MM
	AmountINR int64  `json:"amountInr"`
}

// saveSpend records spend for a source and month (head only).
func (s *Server) saveSpend(w http.ResponseWriter, r *http.Request) {
	var req spendRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	month, err := time.Parse("2006-01", req.Month)
	switch {
	case !spendSources[req.Source]:
		writeError(w, http.StatusBadRequest, "Choose a paid source: property_portal, social_ad, website or hoarding_call.")
		return
	case err != nil:
		writeError(w, http.StatusBadRequest, "The month should look like 2026-11.")
		return
	case req.AmountINR < 0 || req.AmountINR > 10_000_000_000:
		writeError(w, http.StatusBadRequest, "Enter an amount between ₹0 and ₹1,000 crore.")
		return
	}
	row, err := s.store.SaveSpend(r.Context(), req.Source, month, req.AmountINR)
	if err != nil {
		s.serverError(w, "save spend", err)
		return
	}
	writeJSON(w, http.StatusOK, row)
}

// monthOf returns the first day of t's month (Ahmedabad time), as a UTC date.
func monthOf(t time.Time) time.Time {
	t = t.In(booking.IST)
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
}
