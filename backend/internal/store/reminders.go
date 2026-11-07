package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"leadflow/internal/leads"
	"leadflow/internal/pii"
)

// =============================================================================
// SENDING REMINDERS SAFELY
//
// Reminders are rows in the `reminders` table, written when a visit is booked
// (see writeReminders). A background worker sends the ones that are due.
//
// Each reminder is handled in its own short transaction:
//
//	SELECT ... WHERE status = 'queued' AND due_at <= now
//	ORDER BY due_at LIMIT 1
//	FOR UPDATE OF r SKIP LOCKED
//
// FOR UPDATE locks the row, and SKIP LOCKED makes a second worker (another
// server, or an overlapping run) skip a row someone else is already sending
// instead of waiting for it. So each reminder is picked by exactly one worker.
//
// The transaction is deliberately NOT retried automatically (runTx, not
// withTx): a retry would send the message a second time.
// =============================================================================

// DueReminder is everything needed to decide about, and write, one reminder.
type DueReminder struct {
	ID            string
	Kind          string
	DueAt         time.Time
	VisitCode     string
	VisitStatus   string
	StartsAt      time.Time
	LeadName      string
	Phone         string
	Language      string
	Consent       bool
	OptedOut      bool
	ProjectName   string
	ReraNumber    string
	Address       string
	ExecutiveName string
}

// ReminderResult is what happened to one reminder.
type ReminderResult struct {
	Status     string // sent, skipped or failed
	SkipReason string // why it was skipped or failed
	Body       string // the message text (kept for the reminder log)
}

// ProcessDueReminders hands up to limit due reminders, one at a time, to
// handle, and saves what it returns. It reports how many were processed.
func (s *Store) ProcessDueReminders(ctx context.Context, now time.Time, limit int,
	handle func(context.Context, DueReminder) ReminderResult) (int, error) {
	done := 0
	for done < limit {
		found := false
		err := s.runTx(ctx, func(tx pgx.Tx) error {
			var d DueReminder
			err := tx.QueryRow(ctx, `
				SELECT r.id::text, r.kind, r.due_at, v.code, v.status, sl.starts_at,
				       l.name, l.phone, l.language, l.consent, l.opted_out,
				       p.name, p.rera_number, p.address, u.name
				FROM reminders r
				JOIN visits v       ON v.id = r.visit_id
				JOIN visit_slots sl ON sl.id = v.slot_id
				JOIN leads l        ON l.id = v.lead_id
				JOIN projects p     ON p.id = sl.project_id
				JOIN users u        ON u.id = sl.executive_id
				WHERE r.status = 'queued' AND r.due_at <= $1::timestamptz
				ORDER BY r.due_at
				LIMIT 1
				FOR UPDATE OF r SKIP LOCKED`, now,
			).Scan(&d.ID, &d.Kind, &d.DueAt, &d.VisitCode, &d.VisitStatus, &d.StartsAt,
				pii.Open(pii.Name, &d.LeadName), pii.Open(pii.Phone, &d.Phone), &d.Language, &d.Consent, &d.OptedOut,
				&d.ProjectName, &d.ReraNumber, &d.Address, pii.Open(pii.Name, &d.ExecutiveName))
			if errors.Is(err, pgx.ErrNoRows) {
				return nil // nothing due
			}
			if err != nil {
				return err
			}
			found = true

			res := handle(ctx, d)
			var sentAt *time.Time
			if res.Status == "sent" {
				sentAt = &now
			}
			_, err = tx.Exec(ctx, `
				UPDATE reminders SET status = $2, skip_reason = $3, body = $4, sent_at = $5
				WHERE id = $1::uuid`, d.ID, res.Status, nullable(res.SkipReason), pii.SealOptional(pii.Text, res.Body), sentAt)
			return err
		})
		if err != nil {
			return done, err
		}
		if !found {
			break
		}
		done++
	}
	return done, nil
}

// ReminderLogEntry is one line of the staff reminder log.
type ReminderLogEntry struct {
	ID            string     `json:"id"`
	Kind          string     `json:"kind"`
	DueAt         time.Time  `json:"dueAt"`
	Status        string     `json:"status"`
	SkipReason    *string    `json:"skipReason"`
	Body          *string    `json:"body"`
	SentAt        *time.Time `json:"sentAt"`
	VisitID       string     `json:"visitId"`
	VisitStartsAt time.Time  `json:"visitStartsAt"`
	LeadID        string     `json:"leadId"`
	LeadCode      string     `json:"leadCode"`
	LeadName      string     `json:"leadName"`
	ProjectName   string     `json:"projectName"`
	ExecutiveName string     `json:"executiveName"`
}

// ReminderFilter narrows the reminder log. Zero values mean "no filter".
type ReminderFilter struct {
	Status string
	From   time.Time // due at or after
	To     time.Time // due before
}

// ReminderLog lists reminders, newest due first (at most 200). An executive
// sees only reminders for visits in their own slots.
func (s *Store) ReminderLog(ctx context.Context, v Viewer, f ReminderFilter) ([]ReminderLogEntry, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT r.id::text, r.kind, r.due_at, r.status, r.skip_reason, r.body, r.sent_at,
		       v.id::text, sl.starts_at, l.id::text, l.code, l.name, p.name, u.name
		FROM reminders r
		JOIN visits v       ON v.id = r.visit_id
		JOIN visit_slots sl ON sl.id = v.slot_id
		JOIN leads l        ON l.id = v.lead_id
		JOIN projects p     ON p.id = sl.project_id
		JOIN users u        ON u.id = sl.executive_id
		WHERE ($1::boolean OR sl.executive_id = $2::uuid)
		  AND ($3::text = '' OR r.status = $3::text)
		  AND r.due_at >= $4::timestamptz AND r.due_at < $5::timestamptz
		ORDER BY r.due_at DESC
		LIMIT 200`, v.IsHead, v.ID, f.Status, f.From, f.To)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ReminderLogEntry{}
	for rows.Next() {
		var e ReminderLogEntry
		if err := rows.Scan(&e.ID, &e.Kind, &e.DueAt, &e.Status, &e.SkipReason, pii.OpenPtr(pii.Text, &e.Body), &e.SentAt,
			&e.VisitID, &e.VisitStartsAt, &e.LeadID, &e.LeadCode, pii.Open(pii.Name, &e.LeadName), &e.ProjectName,
			pii.Open(pii.Name, &e.ExecutiveName)); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// -----------------------------------------------------------------------------
// Opt-out (DPDP Act): a buyer can stop all messages at any time
// -----------------------------------------------------------------------------

// optOut marks a lead as opted out and skips every reminder still waiting to
// be sent for them. Calling it again is harmless.
func optOut(ctx context.Context, tx pgx.Tx, leadID, how string) error {
	if _, err := tx.Exec(ctx, `
		UPDATE leads SET opted_out = true, opted_out_at = COALESCE(opted_out_at, now()), updated_at = now()
		WHERE id = $1::uuid`, leadID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE reminders SET status = 'skipped', skip_reason = 'opted_out'
		WHERE status = 'queued' AND visit_id IN (SELECT id FROM visits WHERE lead_id = $1::uuid)`, leadID); err != nil {
		return err
	}
	return addTouch(ctx, tx, leadID, "manual", "", "Opted out of messages ("+how+")")
}

// OptOutByVisitCode opts out the buyer who owns a visit link.
//
// Errors: ErrNotFound.
func (s *Store) OptOutByVisitCode(ctx context.Context, code string) error {
	return s.withTx(ctx, func(tx pgx.Tx) error {
		var leadID string
		err := tx.QueryRow(ctx, `SELECT lead_id::text FROM visits WHERE code = $1`, code).Scan(&leadID)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		return optOut(ctx, tx, leadID, "visit page")
	})
}

// OptOutByPhone opts out whoever has this (already normalised) phone number,
// for example after they reply STOP. It reports whether a lead was found.
func (s *Store) OptOutByPhone(ctx context.Context, phone string) (bool, error) {
	found := false
	err := s.withTx(ctx, func(tx pgx.Tx) error {
		var leadID string
		// The phone is encrypted; its blind index finds the lead without decrypting anything.
		err := tx.QueryRow(ctx, `SELECT id::text FROM leads WHERE phone_bidx = $1`, pii.PhoneIndex(phone)).Scan(&leadID)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		found = true
		return optOut(ctx, tx, leadID, "replied STOP")
	})
	return found, err
}

// OptOutLead lets staff record an opt-out the buyer asked for, e.g. on a call.
// An executive may only do this for their own leads.
//
// Errors: leads.ErrNotFound (no such lead, or not theirs).
func (s *Store) OptOutLead(ctx context.Context, v Viewer, leadID string) error {
	return s.withTx(ctx, func(tx pgx.Tx) error {
		if err := checkLeadAccess(ctx, tx, v, leadID); err != nil {
			return err
		}
		return optOut(ctx, tx, leadID, "recorded by staff")
	})
}

// checkLeadAccess returns leads.ErrNotFound unless the viewer may see this lead.
// "Not found" rather than "forbidden", so ids of other people's leads cannot be probed.
func checkLeadAccess(ctx context.Context, q rowQuerier, v Viewer, leadID string) error {
	var ok bool
	err := q.QueryRow(ctx, `
		SELECT true FROM leads WHERE id = $1::uuid AND ($2::boolean OR assigned_to = $3::uuid)`,
		leadID, v.IsHead, v.ID).Scan(&ok)
	if errors.Is(err, pgx.ErrNoRows) {
		return leads.ErrNotFound
	}
	return err
}
