package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"leadflow/internal/booking"
	"leadflow/internal/models"
)

// =============================================================================
// THE NO-DOUBLE-BOOKING GUARANTEE
//
// Three layers protect a slot, so a bug in one cannot cause a double booking:
//
//  1. Claiming a slot is one atomic statement:
//         UPDATE visit_slots SET status='booked' WHERE id=$1 AND status='open'
//     Postgres locks the row. If two buyers race, the second one waits, then
//     finds status is no longer 'open' and updates zero rows.
//  2. The database has a unique index: a slot may hold only one live visit.
//  3. The slot claim and the visit insert happen in ONE transaction, so if
//     anything fails the slot goes back to 'open' automatically.
//
// The AI assistant never decides what is free. It can only ask this code.
// =============================================================================

// FreeSlot is an open slot a buyer can choose.
type FreeSlot struct {
	ID            string    `json:"id"`
	ProjectID     string    `json:"projectId"`
	StartsAt      time.Time `json:"startsAt"`
	EndsAt        time.Time `json:"endsAt"`
	ExecutiveID   string    `json:"-"`
	ExecutiveName string    `json:"executiveName"`
}

// FreeSlots lists bookable slots for a project between from (inclusive) and
// to (exclusive). Slots that start sooner than booking.MinNotice are hidden.
func (s *Store) FreeSlots(ctx context.Context, now time.Time, projectID string, from, to time.Time) ([]FreeSlot, error) {
	if earliest := now.Add(booking.MinNotice); from.Before(earliest) {
		from = earliest
	}

	rows, err := s.pool.Query(ctx, `
		SELECT s.id::text, s.project_id, s.starts_at, s.ends_at, s.executive_id::text, u.name
		FROM visit_slots s
		JOIN users u ON u.id = s.executive_id
		WHERE s.project_id = $1 AND s.status = 'open' AND s.starts_at >= $2 AND s.starts_at < $3
		ORDER BY s.starts_at, u.name`, projectID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	slots := []FreeSlot{}
	for rows.Next() {
		var f FreeSlot
		if err := rows.Scan(&f.ID, &f.ProjectID, &f.StartsAt, &f.EndsAt, &f.ExecutiveID, &f.ExecutiveName); err != nil {
			return nil, err
		}
		slots = append(slots, f)
	}
	return slots, rows.Err()
}

// BookSlot books a free slot for a lead. It either fully succeeds or changes nothing.
//
// Errors: ErrLeadNotFound, ErrSlotUnavailable, ErrLeadAlreadyBooked.
func (s *Store) BookSlot(ctx context.Context, now time.Time, leadID, slotID string) (models.Visit, error) {
	var visit models.Visit
	err := s.withTx(ctx, func(tx pgx.Tx) error {
		canContact, err := lockLead(ctx, tx, leadID)
		if err != nil {
			return err
		}
		slot, err := claimSlot(ctx, tx, now, slotID)
		if err != nil {
			return err
		}
		if visit, err = insertVisit(ctx, tx, leadID, slotID); err != nil {
			return err
		}
		if err := writeReminders(ctx, tx, now, visit.ID, slot.start, canContact); err != nil {
			return err
		}
		return markLeadBooked(ctx, tx, leadID, slot.execID)
	})
	return visit, err
}

// RescheduleVisit moves a booked visit to another free slot. The visit keeps
// its code, so links already sent to the buyer keep working. It is atomic: if
// the new slot is taken, the original booking is left exactly as it was.
//
// Errors: ErrVisitNotFound, ErrVisitNotChangeable, ErrSameSlot, ErrSlotUnavailable.
func (s *Store) RescheduleVisit(ctx context.Context, now time.Time, visitID, newSlotID string) (models.Visit, error) {
	var visit models.Visit
	err := s.withTx(ctx, func(tx pgx.Tx) error {
		cur, err := lockVisit(ctx, tx, visitID)
		if err != nil {
			return err
		}
		if cur.status != models.VisitBooked || !cur.start.After(now) {
			return booking.ErrVisitNotChangeable
		}
		if cur.slotID == newSlotID {
			return booking.ErrSameSlot
		}

		canContact, err := lockLead(ctx, tx, cur.leadID)
		if err != nil {
			return err
		}
		slot, err := claimSlot(ctx, tx, now, newSlotID)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE visits SET slot_id = $2::uuid, updated_at = now() WHERE id = $1::uuid`, visitID, newSlotID); err != nil {
			return err
		}
		// Free the old slot. It started in the future (checked above), so it is bookable again.
		if _, err := tx.Exec(ctx, `UPDATE visit_slots SET status = 'open' WHERE id = $1::uuid`, cur.slotID); err != nil {
			return err
		}
		if err := writeReminders(ctx, tx, now, visitID, slot.start, canContact); err != nil {
			return err
		}
		if err := markLeadBooked(ctx, tx, cur.leadID, slot.execID); err != nil {
			return err
		}
		visit, err = visitByID(ctx, tx, visitID)
		return err
	})
	return visit, err
}

// CancelVisit cancels a booked visit and frees its slot for someone else.
//
// Errors: ErrVisitNotFound, ErrVisitNotChangeable.
func (s *Store) CancelVisit(ctx context.Context, now time.Time, visitID string) error {
	return s.withTx(ctx, func(tx pgx.Tx) error {
		cur, err := lockVisit(ctx, tx, visitID)
		if err != nil {
			return err
		}
		if cur.status != models.VisitBooked || !cur.start.After(now) {
			return booking.ErrVisitNotChangeable
		}
		if _, err := tx.Exec(ctx, `UPDATE visits SET status = 'cancelled', updated_at = now() WHERE id = $1::uuid`, visitID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE visit_slots SET status = 'open' WHERE id = $1::uuid`, cur.slotID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			UPDATE reminders SET status = 'skipped', skip_reason = 'visit_cancelled'
			WHERE visit_id = $1::uuid AND status = 'queued'`, visitID); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `
			UPDATE leads SET status = 'engaged', updated_at = now()
			WHERE id = $1::uuid AND status = 'visit_booked'`, cur.leadID)
		return err
	})
}

// MarkOutcome records whether the buyer came (visited) or not (no_show). It is
// what makes the show-up rate on the dashboard possible. A visit can only be
// marked once its start time has passed.
//
// Errors: ErrVisitNotFound, ErrVisitNotMarkable.
func (s *Store) MarkOutcome(ctx context.Context, now time.Time, visitID, outcome string) error {
	if outcome != models.VisitVisited && outcome != models.VisitNoShow {
		return booking.ErrVisitNotMarkable
	}
	return s.withTx(ctx, func(tx pgx.Tx) error {
		cur, err := lockVisit(ctx, tx, visitID)
		if err != nil {
			return err
		}
		if cur.status != models.VisitBooked || cur.start.After(now) {
			return booking.ErrVisitNotMarkable
		}
		if _, err := tx.Exec(ctx, `
			UPDATE visits SET status = $2, outcome_at = $3, updated_at = now() WHERE id = $1::uuid`,
			visitID, outcome, now); err != nil {
			return err
		}
		leadStatus := models.LeadVisited
		if outcome == models.VisitNoShow {
			leadStatus = models.LeadEngaged // still a live lead, worth a follow-up
		}
		_, err = tx.Exec(ctx, `UPDATE leads SET status = $2, updated_at = now() WHERE id = $1::uuid`, cur.leadID, leadStatus)
		return err
	})
}

// CreateSlot adds a visit slot for an executive (used by the admin editor).
//
// Errors: ErrExecutiveNotFound, ErrSlotOverlap.
func (s *Store) CreateSlot(ctx context.Context, projectID, executiveID string, start, end time.Time) (models.VisitSlot, error) {
	var slot models.VisitSlot
	err := s.pool.QueryRow(ctx, `
		INSERT INTO visit_slots (project_id, executive_id, starts_at, ends_at)
		SELECT $1, u.id, $3, $4 FROM users u WHERE u.id = $2::uuid AND u.role = 'executive' AND u.active
		RETURNING id::text, project_id, executive_id::text, starts_at, ends_at, status`,
		projectID, executiveID, start, end,
	).Scan(&slot.ID, &slot.ProjectID, &slot.ExecutiveID, &slot.StartsAt, &slot.EndsAt, &slot.Status)

	switch {
	case errors.Is(err, pgx.ErrNoRows): // the INSERT ... SELECT found no such active executive
		return models.VisitSlot{}, booking.ErrExecutiveNotFound
	case pgError(err) != nil && pgError(err).Code == "23P01": // exclusion violation: overlapping slot
		return models.VisitSlot{}, booking.ErrSlotOverlap
	case pgError(err) != nil && pgError(err).Code == "23503": // foreign key: unknown project
		return models.VisitSlot{}, booking.ErrExecutiveNotFound
	}
	return slot, err
}

// CancelSlot removes an open slot. A slot with a booking must have its visit
// cancelled or moved first, so a buyer is never silently dropped.
//
// Errors: ErrSlotNotFound, ErrSlotHasBooking.
func (s *Store) CancelSlot(ctx context.Context, slotID string) error {
	tag, err := s.pool.Exec(ctx, `UPDATE visit_slots SET status = 'cancelled' WHERE id = $1::uuid AND status = 'open'`, slotID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 1 {
		return nil
	}
	var status string
	err = s.pool.QueryRow(ctx, `SELECT status FROM visit_slots WHERE id = $1::uuid`, slotID).Scan(&status)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return booking.ErrSlotNotFound
	case err != nil:
		return err
	case status == models.SlotBooked:
		return booking.ErrSlotHasBooking
	}
	return nil // already cancelled: nothing to do
}

// -----------------------------------------------------------------------------
// Lookups used by the HTTP layer
// -----------------------------------------------------------------------------

// VisitDetail is what the buyer's /visit/:code page shows. It deliberately
// contains no phone number and only the buyer's first name.
type VisitDetail struct {
	ID            string    `json:"id"`
	Code          string    `json:"code"`
	Status        string    `json:"status"`
	SlotID        string    `json:"slotId"`
	StartsAt      time.Time `json:"startsAt"`
	EndsAt        time.Time `json:"endsAt"`
	ProjectID     string    `json:"projectId"`
	ProjectName   string    `json:"projectName"`
	ReraNumber    string    `json:"reraNumber"`
	Address       string    `json:"address"`
	Latitude      float64   `json:"latitude"`
	Longitude     float64   `json:"longitude"`
	ExecutiveName string    `json:"executiveName"`
	LeadFirstName string    `json:"leadFirstName"`
	CanChange     bool      `json:"canChange"` // false once the visit has started or is closed
}

// VisitDetailByCode loads a visit by its public code.
func (s *Store) VisitDetailByCode(ctx context.Context, now time.Time, code string) (VisitDetail, error) {
	var d VisitDetail
	var leadName string
	err := s.pool.QueryRow(ctx, `
		SELECT v.id::text, v.code, v.status, v.slot_id::text, s.starts_at, s.ends_at,
		       p.id, p.name, p.rera_number, p.address, p.latitude, p.longitude,
		       u.name, l.name
		FROM visits v
		JOIN visit_slots s ON s.id = v.slot_id
		JOIN projects p ON p.id = s.project_id
		JOIN users u ON u.id = s.executive_id
		JOIN leads l ON l.id = v.lead_id
		WHERE v.code = $1`, code,
	).Scan(&d.ID, &d.Code, &d.Status, &d.SlotID, &d.StartsAt, &d.EndsAt,
		&d.ProjectID, &d.ProjectName, &d.ReraNumber, &d.Address, &d.Latitude, &d.Longitude,
		&d.ExecutiveName, &leadName)
	if errors.Is(err, pgx.ErrNoRows) {
		return VisitDetail{}, booking.ErrVisitNotFound
	}
	if err != nil {
		return VisitDetail{}, err
	}
	d.LeadFirstName = firstWord(leadName)
	d.CanChange = d.Status == models.VisitBooked && d.StartsAt.After(now)
	return d, nil
}

// VisitIDByCode resolves a public code to the visit id.
func (s *Store) VisitIDByCode(ctx context.Context, code string) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx, `SELECT id::text FROM visits WHERE code = $1`, code).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", booking.ErrVisitNotFound
	}
	return id, err
}

// LeadAssignee returns the executive a lead is assigned to (nil if nobody).
func (s *Store) LeadAssignee(ctx context.Context, leadID string) (*string, error) {
	var assigned *string
	err := s.pool.QueryRow(ctx, `SELECT assigned_to::text FROM leads WHERE id = $1::uuid`, leadID).Scan(&assigned)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, booking.ErrLeadNotFound
	}
	return assigned, err
}

// VisitExecutive returns the executive who owns a visit's slot.
func (s *Store) VisitExecutive(ctx context.Context, visitID string) (string, error) {
	var exec string
	err := s.pool.QueryRow(ctx, `
		SELECT s.executive_id::text FROM visits v JOIN visit_slots s ON s.id = v.slot_id
		WHERE v.id = $1::uuid`, visitID).Scan(&exec)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", booking.ErrVisitNotFound
	}
	return exec, err
}

func firstWord(s string) string {
	for i, r := range s {
		if r == ' ' {
			return s[:i]
		}
	}
	return s
}

// -----------------------------------------------------------------------------
// Building blocks used inside transactions
// -----------------------------------------------------------------------------

type claimedSlot struct {
	projectID string
	execID    string
	start     time.Time
}

// lockLead locks the lead's row for the rest of the transaction and reports
// whether we may contact them. Locking means two bookings for the same lead
// are handled one after the other instead of at the same time.
func lockLead(ctx context.Context, tx pgx.Tx, leadID string) (canContact bool, err error) {
	var consent, optedOut bool
	err = tx.QueryRow(ctx, `SELECT consent, opted_out FROM leads WHERE id = $1::uuid FOR UPDATE`, leadID).Scan(&consent, &optedOut)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, booking.ErrLeadNotFound
	}
	return consent && !optedOut, err
}

// claimSlot is the atomic "take this slot if it is still open" step.
func claimSlot(ctx context.Context, tx pgx.Tx, now time.Time, slotID string) (claimedSlot, error) {
	var c claimedSlot
	err := tx.QueryRow(ctx, `
		UPDATE visit_slots SET status = 'booked'
		WHERE id = $1::uuid AND status = 'open' AND starts_at >= $2
		RETURNING project_id, executive_id::text, starts_at`,
		slotID, now.Add(booking.MinNotice),
	).Scan(&c.projectID, &c.execID, &c.start)
	if errors.Is(err, pgx.ErrNoRows) {
		return claimedSlot{}, booking.ErrSlotUnavailable
	}
	return c, err
}

func insertVisit(ctx context.Context, tx pgx.Tx, leadID, slotID string) (models.Visit, error) {
	code, err := booking.NewVisitCode()
	if err != nil {
		return models.Visit{}, err
	}
	var v models.Visit
	err = tx.QueryRow(ctx, `
		INSERT INTO visits (code, lead_id, slot_id) VALUES ($1, $2::uuid, $3::uuid)
		RETURNING id::text, code, lead_id::text, slot_id::text, status, notes, outcome_at, created_at, updated_at`,
		code, leadID, slotID,
	).Scan(&v.ID, &v.Code, &v.LeadID, &v.SlotID, &v.Status, &v.Notes, &v.OutcomeAt, &v.CreatedAt, &v.UpdatedAt)
	if err != nil {
		// The database's own unique indexes are the last line of defence.
		switch constraintName(err) {
		case "visits_one_booked_per_lead":
			return models.Visit{}, booking.ErrLeadAlreadyBooked
		case "visits_one_live_per_slot":
			return models.Visit{}, booking.ErrSlotUnavailable
		}
		return models.Visit{}, err
	}
	return v, nil
}

// writeReminders stores (or updates, when rescheduling) the two reminders.
func writeReminders(ctx context.Context, tx pgx.Tx, now time.Time, visitID string, start time.Time, canContact bool) error {
	for _, p := range booking.PlanReminders(now, start, canContact) {
		_, err := tx.Exec(ctx, `
			INSERT INTO reminders (visit_id, kind, due_at, status, skip_reason)
			VALUES ($1::uuid, $2, $3, $4, $5)
			ON CONFLICT (visit_id, kind) DO UPDATE
			SET due_at = EXCLUDED.due_at, status = EXCLUDED.status, skip_reason = EXCLUDED.skip_reason,
			    body = NULL, sent_at = NULL`,
			visitID, p.Kind, p.DueAt, p.Status, nullable(p.SkipReason))
		if err != nil {
			return err
		}
	}
	return nil
}

// markLeadBooked moves the lead to "visit booked" and names the executive who
// will meet them as their salesperson.
func markLeadBooked(ctx context.Context, tx pgx.Tx, leadID, execID string) error {
	_, err := tx.Exec(ctx, `
		UPDATE leads SET status = 'visit_booked', assigned_to = $2::uuid, updated_at = now()
		WHERE id = $1::uuid`, leadID, execID)
	return err
}

type currentVisit struct {
	leadID string
	slotID string
	status string
	start  time.Time
}

// lockVisit loads a visit and locks its row until the transaction ends.
func lockVisit(ctx context.Context, tx pgx.Tx, visitID string) (currentVisit, error) {
	var c currentVisit
	err := tx.QueryRow(ctx, `
		SELECT v.lead_id::text, v.slot_id::text, v.status, s.starts_at
		FROM visits v JOIN visit_slots s ON s.id = v.slot_id
		WHERE v.id = $1::uuid
		FOR UPDATE OF v`, visitID,
	).Scan(&c.leadID, &c.slotID, &c.status, &c.start)
	if errors.Is(err, pgx.ErrNoRows) {
		return currentVisit{}, booking.ErrVisitNotFound
	}
	return c, err
}

func visitByID(ctx context.Context, q rowQuerier, visitID string) (models.Visit, error) {
	var v models.Visit
	err := q.QueryRow(ctx, `
		SELECT id::text, code, lead_id::text, slot_id::text, status, notes, outcome_at, created_at, updated_at
		FROM visits WHERE id = $1::uuid`, visitID,
	).Scan(&v.ID, &v.Code, &v.LeadID, &v.SlotID, &v.Status, &v.Notes, &v.OutcomeAt, &v.CreatedAt, &v.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return models.Visit{}, booking.ErrVisitNotFound
	}
	return v, err
}
