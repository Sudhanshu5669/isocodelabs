package store

import (
	"context"
	"errors"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"

	"leadflow/internal/models"
	"leadflow/internal/pii"
)

// Lists for the staff screens. As with leads, the rule "an executive sees only
// their own" is in the SQL, so no handler can forget it.

// StaffVisit is one row of the visits list (the executive's diary).
type StaffVisit struct {
	ID            string    `json:"id"`
	Code          string    `json:"code"`
	Status        string    `json:"status"`
	StartsAt      time.Time `json:"startsAt"`
	EndsAt        time.Time `json:"endsAt"`
	ProjectID     string    `json:"projectId"`
	ProjectName   string    `json:"projectName"`
	ExecutiveID   string    `json:"executiveId"`
	ExecutiveName string    `json:"executiveName"`
	LeadID        string    `json:"leadId"`
	LeadCode      string    `json:"leadCode"`
	LeadName      string    `json:"leadName"`
	LeadPhone     string    `json:"leadPhone"`
	CanMark       bool      `json:"canMark"` // the visit has started and has no outcome yet
}

// VisitFilter narrows the visits list.
type VisitFilter struct {
	From, To  time.Time
	Status    string
	ProjectID string
}

// StaffVisits lists visits in a time range, earliest first (at most 300).
// An executive sees only visits in their own slots.
func (s *Store) StaffVisits(ctx context.Context, now time.Time, v Viewer, f VisitFilter) ([]StaffVisit, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT vi.id::text, vi.code, vi.status, sl.starts_at, sl.ends_at, p.id, p.name,
		       u.id::text, u.name, l.id::text, l.code, l.name, l.phone
		FROM visits vi
		JOIN visit_slots sl ON sl.id = vi.slot_id
		JOIN projects p     ON p.id = sl.project_id
		JOIN users u        ON u.id = sl.executive_id
		JOIN leads l        ON l.id = vi.lead_id
		WHERE ($1::boolean OR sl.executive_id = $2::uuid)
		  AND sl.starts_at >= $3::timestamptz AND sl.starts_at < $4::timestamptz
		  AND ($5::text = '' OR vi.status = $5::text)
		  AND ($6::text = '' OR p.id = $6::text)
		ORDER BY sl.starts_at
		LIMIT 300`, v.IsHead, v.ID, f.From, f.To, f.Status, f.ProjectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []StaffVisit{}
	for rows.Next() {
		var sv StaffVisit
		if err := rows.Scan(&sv.ID, &sv.Code, &sv.Status, &sv.StartsAt, &sv.EndsAt, &sv.ProjectID, &sv.ProjectName,
			&sv.ExecutiveID, pii.Open(pii.Name, &sv.ExecutiveName), &sv.LeadID, &sv.LeadCode,
			pii.Open(pii.Name, &sv.LeadName), pii.Open(pii.Phone, &sv.LeadPhone)); err != nil {
			return nil, err
		}
		sv.CanMark = sv.Status == models.VisitBooked && !sv.StartsAt.After(now)
		out = append(out, sv)
	}
	return out, rows.Err()
}

// StaffSlot is one row of the slot manager: every slot, with who booked it.
type StaffSlot struct {
	ID            string    `json:"id"`
	ProjectID     string    `json:"projectId"`
	ProjectName   string    `json:"projectName"`
	ExecutiveID   string    `json:"executiveId"`
	ExecutiveName string    `json:"executiveName"`
	StartsAt      time.Time `json:"startsAt"`
	EndsAt        time.Time `json:"endsAt"`
	Status        string    `json:"status"` // open, booked or cancelled
	VisitID       *string   `json:"visitId"`
	LeadName      *string   `json:"leadName"`
}

// StaffSlots lists slots in a time range (at most 500), cancelled ones left out.
// An executive sees only their own slots.
func (s *Store) StaffSlots(ctx context.Context, v Viewer, projectID string, from, to time.Time) ([]StaffSlot, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT sl.id::text, p.id, p.name, u.id::text, u.name, sl.starts_at, sl.ends_at, sl.status,
		       vi.id::text, l.name
		FROM visit_slots sl
		JOIN projects p ON p.id = sl.project_id
		JOIN users u    ON u.id = sl.executive_id
		LEFT JOIN visits vi ON vi.slot_id = sl.id AND vi.status <> 'cancelled'
		LEFT JOIN leads l   ON l.id = vi.lead_id
		WHERE ($1::boolean OR sl.executive_id = $2::uuid)
		  AND ($3::text = '' OR sl.project_id = $3::text)
		  AND sl.starts_at >= $4::timestamptz AND sl.starts_at < $5::timestamptz
		  AND sl.status <> 'cancelled'
		ORDER BY sl.starts_at, sl.id
		LIMIT 500`, v.IsHead, v.ID, projectID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []StaffSlot{}
	for rows.Next() {
		var ss StaffSlot
		if err := rows.Scan(&ss.ID, &ss.ProjectID, &ss.ProjectName, &ss.ExecutiveID, pii.Open(pii.Name, &ss.ExecutiveName),
			&ss.StartsAt, &ss.EndsAt, &ss.Status, &ss.VisitID, pii.OpenPtr(pii.Name, &ss.LeadName)); err != nil {
			return nil, err
		}
		out = append(out, ss)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Same start time: alphabetical by executive (names are encrypted, so not in SQL).
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].StartsAt.Equal(out[j].StartsAt) {
			return out[i].StartsAt.Before(out[j].StartsAt)
		}
		return out[i].ExecutiveName < out[j].ExecutiveName
	})
	return out, nil
}

// -----------------------------------------------------------------------------
// Lead status changes by staff
// -----------------------------------------------------------------------------

// ErrLeadHasBooking means the lead has a booked visit, so it cannot be marked lost.
var ErrLeadHasBooking = errors.New("store: lead has a booked visit")

// SetLeadStatus lets staff mark a lead lost, or reopen it. The visit-driven
// stages (visit_booked, visited) are set only by the booking engine, so the
// dashboard's visit numbers cannot be edited by hand.
//
// Errors: leads.ErrNotFound (also for a lead an executive may not see),
// ErrLeadHasBooking.
func (s *Store) SetLeadStatus(ctx context.Context, v Viewer, leadID, status string) error {
	return s.withTx(ctx, func(tx pgx.Tx) error {
		if err := checkLeadAccess(ctx, tx, v, leadID); err != nil {
			return err
		}
		var booked bool
		if err := tx.QueryRow(ctx, `
			SELECT EXISTS (SELECT 1 FROM visits WHERE lead_id = $1::uuid AND status = 'booked')`, leadID).Scan(&booked); err != nil {
			return err
		}
		if booked {
			return ErrLeadHasBooking
		}
		_, err := tx.Exec(ctx, `UPDATE leads SET status = $2, updated_at = now() WHERE id = $1::uuid`, leadID, status)
		if err != nil {
			return err
		}
		return addTouch(ctx, tx, leadID, "manual", "", "Status set to "+status+" by staff")
	})
}
