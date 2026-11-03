package store

import (
	"context"
	"time"
)

// =============================================================================
// THE SOURCE DASHBOARD
//
// Every number is about the leads that ARRIVED in the chosen period (a
// "cohort"), so the figures for one period always add up: a lead from October
// counts toward October's visits even if the visit happened in November.
//
// Definitions (also in the TDD, labelled as assumptions where the brief is silent):
//   - First response: time from the lead arriving to the first reply it got
//     (leads.first_response_at; the chat sets it on the assistant's first message).
//   - Unreached in 48 h: of the leads at least 48 h old, the share with no
//     reply within 48 h. Younger leads are left out: they still have time.
//   - Lead to visit: share of leads who actually visited (not just booked).
//   - Show-up rate: visited / (visited + no-show), over visits whose outcome
//     has been recorded.
//   - Cost per visit: spend on the source / visits completed. Spend is stored
//     per source and month, so every month the period touches counts in full,
//     and it cannot be split by project (cost figures are left empty then).
// =============================================================================

// Metrics is one row of the dashboard: all leads, or one source.
type Metrics struct {
	Source                     string   `json:"source,omitempty"` // empty for the totals row
	Leads                      int64    `json:"leads"`
	Responded                  int64    `json:"responded"`
	MedianFirstResponseSeconds *float64 `json:"medianFirstResponseSeconds"`
	RespondedWithin5Min        int64    `json:"respondedWithin5Min"`
	RespondedWithin5MinPct     *float64 `json:"respondedWithin5MinPct"`
	OldEnoughFor48h            int64    `json:"oldEnoughFor48h"`
	UnreachedIn48h             int64    `json:"unreachedIn48h"`
	UnreachedIn48hPct          *float64 `json:"unreachedIn48hPct"`
	LeadsWithVisitBooked       int64    `json:"leadsWithVisitBooked"`
	LeadsVisited               int64    `json:"leadsVisited"`
	LeadToVisitPct             *float64 `json:"leadToVisitPct"`
	VisitsDone                 int64    `json:"visitsDone"`
	NoShows                    int64    `json:"noShows"`
	ShowUpRatePct              *float64 `json:"showUpRatePct"`
	SpendINR                   *int64   `json:"spendInr"`
	CostPerLeadINR             *float64 `json:"costPerLeadInr"`
	CostPerVisitINR            *float64 `json:"costPerVisitInr"`
}

// DayPoint is one day of the trend chart.
type DayPoint struct {
	Date                       string   `json:"date"` // YYYY-MM-DD, Ahmedabad time
	Leads                      int64    `json:"leads"`
	MedianFirstResponseSeconds *float64 `json:"medianFirstResponseSeconds"`
}

// Dashboard is everything the dashboard screen shows.
type Dashboard struct {
	From      string     `json:"from"` // first day, inclusive
	To        string     `json:"to"`   // last day, inclusive
	ProjectID string     `json:"projectId,omitempty"`
	Totals    Metrics    `json:"totals"`
	BySource  []Metrics  `json:"bySource"`
	Daily     []DayPoint `json:"daily"`
	SpendNote string     `json:"spendNote,omitempty"`
}

// DashboardFor computes the dashboard for leads that arrived in [from, to).
// from and to are midnights in Ahmedabad time; projectID "" means all projects.
func (s *Store) DashboardFor(ctx context.Context, now, from, to time.Time, projectID string, loc *time.Location) (Dashboard, error) {
	d := Dashboard{
		From:      from.In(loc).Format(time.DateOnly),
		To:        to.Add(-time.Nanosecond).In(loc).Format(time.DateOnly),
		ProjectID: projectID,
		BySource:  []Metrics{},
	}

	rows, err := s.pool.Query(ctx, `
		WITH base AS (
			SELECT l.source, l.first_response_at, l.created_at,
			       EXTRACT(EPOCH FROM (l.first_response_at - l.created_at))::float8 AS resp,
			       vc.booked, vc.visited, vc.no_show
			FROM leads l
			CROSS JOIN LATERAL (
				SELECT count(*) FILTER (WHERE v.status <> 'cancelled') AS booked,
				       count(*) FILTER (WHERE v.status = 'visited')    AS visited,
				       count(*) FILTER (WHERE v.status = 'no_show')    AS no_show
				FROM visits v WHERE v.lead_id = l.id
			) vc
			WHERE l.created_at >= $1::timestamptz AND l.created_at < $2::timestamptz
			  AND ($3::text = '' OR l.project_id = $3::text)
		)
		SELECT COALESCE(source, ''), count(*), count(first_response_at),
		       percentile_cont(0.5) WITHIN GROUP (ORDER BY resp),
		       count(*) FILTER (WHERE resp <= 300),
		       count(*) FILTER (WHERE created_at <= $4::timestamptz - interval '48 hours'),
		       count(*) FILTER (WHERE created_at <= $4::timestamptz - interval '48 hours'
		                          AND (resp IS NULL OR resp > 48 * 3600)),
		       count(*) FILTER (WHERE booked > 0),
		       count(*) FILTER (WHERE visited > 0),
		       COALESCE(sum(visited), 0)::bigint, COALESCE(sum(no_show), 0)::bigint
		FROM base
		GROUP BY GROUPING SETS ((source), ())
		ORDER BY source NULLS FIRST`, from, to, projectID, now)
	if err != nil {
		return Dashboard{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var m Metrics
		var isTotal bool
		if err := rows.Scan(&m.Source, &m.Leads, &m.Responded, &m.MedianFirstResponseSeconds,
			&m.RespondedWithin5Min, &m.OldEnoughFor48h, &m.UnreachedIn48h,
			&m.LeadsWithVisitBooked, &m.LeadsVisited, &m.VisitsDone, &m.NoShows); err != nil {
			return Dashboard{}, err
		}
		isTotal = m.Source == ""
		if isTotal {
			d.Totals = m
		} else {
			d.BySource = append(d.BySource, m)
		}
	}
	if err := rows.Err(); err != nil {
		return Dashboard{}, err
	}
	rows.Close()

	// Spend, per source, for every month the period touches.
	if projectID != "" {
		d.SpendNote = "Spend is recorded per source, not per project, so cost figures are shown for all projects only."
	} else {
		spend, err := s.spendBySource(ctx, firstOfMonth(from.In(loc)), firstOfMonth(to.Add(-time.Nanosecond).In(loc)))
		if err != nil {
			return Dashboard{}, err
		}
		var total int64
		for i := range d.BySource {
			if amt, ok := spend[d.BySource[i].Source]; ok {
				d.BySource[i].SpendINR = &amt
				total += amt
			}
		}
		// Spend on a source with no leads in the period still counts in the total.
		for src, amt := range spend {
			if !hasSource(d.BySource, src) {
				total += amt
			}
		}
		if len(spend) > 0 {
			d.Totals.SpendINR = &total
		}
	}

	fill(&d.Totals)
	for i := range d.BySource {
		fill(&d.BySource[i])
	}

	d.Daily, err = s.dailyLeads(ctx, from, to, projectID, loc)
	return d, err
}

// fill works out the percentages and costs from the counts. A rate with
// nothing to divide by stays nil ("no data"), never a misleading 0%.
func fill(m *Metrics) {
	m.RespondedWithin5MinPct = pct(m.RespondedWithin5Min, m.Leads)
	m.UnreachedIn48hPct = pct(m.UnreachedIn48h, m.OldEnoughFor48h)
	m.LeadToVisitPct = pct(m.LeadsVisited, m.Leads)
	m.ShowUpRatePct = pct(m.VisitsDone, m.VisitsDone+m.NoShows)
	if m.SpendINR != nil {
		m.CostPerLeadINR = ratio(*m.SpendINR, m.Leads)
		m.CostPerVisitINR = ratio(*m.SpendINR, m.VisitsDone)
	}
}

func pct(part, whole int64) *float64 {
	if whole == 0 {
		return nil
	}
	v := float64(part) * 100 / float64(whole)
	return &v
}

func ratio(a, b int64) *float64 {
	if b == 0 {
		return nil
	}
	v := float64(a) / float64(b)
	return &v
}

func hasSource(ms []Metrics, src string) bool {
	for _, m := range ms {
		if m.Source == src {
			return true
		}
	}
	return false
}

func firstOfMonth(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
}

func (s *Store) spendBySource(ctx context.Context, fromMonth, toMonth time.Time) (map[string]int64, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT source, sum(amount_inr)::bigint FROM source_spend
		WHERE month >= $1::date AND month <= $2::date
		GROUP BY source`, fromMonth.Format(time.DateOnly), toMonth.Format(time.DateOnly))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int64{}
	for rows.Next() {
		var src string
		var amt int64
		if err := rows.Scan(&src, &amt); err != nil {
			return nil, err
		}
		out[src] = amt
	}
	return out, rows.Err()
}

// dailyLeads returns one point per day in the period, including days with no leads.
func (s *Store) dailyLeads(ctx context.Context, from, to time.Time, projectID string, loc *time.Location) ([]DayPoint, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT to_char(l.created_at AT TIME ZONE 'Asia/Kolkata', 'YYYY-MM-DD'), count(*),
		       percentile_cont(0.5) WITHIN GROUP (ORDER BY EXTRACT(EPOCH FROM (l.first_response_at - l.created_at)))
		FROM leads l
		WHERE l.created_at >= $1::timestamptz AND l.created_at < $2::timestamptz
		  AND ($3::text = '' OR l.project_id = $3::text)
		GROUP BY 1`, from, to, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	byDay := map[string]DayPoint{}
	for rows.Next() {
		var p DayPoint
		if err := rows.Scan(&p.Date, &p.Leads, &p.MedianFirstResponseSeconds); err != nil {
			return nil, err
		}
		byDay[p.Date] = p
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := []DayPoint{}
	for day := from.In(loc); day.Before(to); day = day.AddDate(0, 0, 1) {
		key := day.Format(time.DateOnly)
		p, ok := byDay[key]
		if !ok {
			p = DayPoint{Date: key}
		}
		out = append(out, p)
	}
	return out, nil
}
