package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"leadflow/internal/leads"
	"leadflow/internal/models"
)

// A lead flagged as a possible duplicate (possible_duplicate_of) waits here for
// a human. They either DISMISS the flag ("different people") or MERGE the later
// lead into the original. Merging is never automatic.

// DismissDuplicate clears the flag: these are two different people.
//
// Errors: leads.ErrNotFound, leads.ErrNotDuplicate.
func (s *Store) DismissDuplicate(ctx context.Context, leadID string) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE leads SET possible_duplicate_of = NULL, updated_at = now()
		WHERE id = $1::uuid AND possible_duplicate_of IS NOT NULL`, leadID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 1 {
		return nil
	}
	return s.missingOrNotDuplicate(ctx, leadID)
}

func (s *Store) missingOrNotDuplicate(ctx context.Context, leadID string) error {
	var exists bool
	if err := s.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM leads WHERE id = $1::uuid)`, leadID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return leads.ErrNotFound
	}
	return leads.ErrNotDuplicate
}

// MergeDuplicate folds the flagged lead into the original it points at and
// returns the original's id. Everything attached to the flagged lead (enquiry
// history, chats, visits) moves to the original, the combined details follow
// leads.Merge, and the flagged record is deleted. It all happens in one
// transaction.
//
// Errors: leads.ErrNotFound, leads.ErrNotDuplicate, leads.ErrMergeConflict
// (both leads have a booked visit; one must be cancelled first).
func (s *Store) MergeDuplicate(ctx context.Context, dropID string) (keepID string, err error) {
	err = s.withTx(ctx, func(tx pgx.Tx) error {
		keepID = ""
		var original *string // NULL when the lead is not flagged
		err := tx.QueryRow(ctx, `SELECT possible_duplicate_of::text FROM leads WHERE id = $1::uuid`, dropID).Scan(&original)
		if errors.Is(err, pgx.ErrNoRows) {
			return leads.ErrNotFound
		}
		if err != nil {
			return err
		}
		if original == nil {
			return leads.ErrNotDuplicate
		}
		keepID = *original

		keep, drop, err := lockPair(ctx, tx, keepID, dropID)
		if err != nil {
			return err
		}
		// Re-check under the lock: someone may have resolved it a moment ago.
		if drop.PossibleDuplicateOf == nil || *drop.PossibleDuplicateOf != keepID {
			return leads.ErrNotDuplicate
		}

		var booked int
		if err := tx.QueryRow(ctx, `
			SELECT count(*) FROM visits WHERE lead_id IN ($1::uuid, $2::uuid) AND status = 'booked'`,
			keepID, dropID).Scan(&booked); err != nil {
			return err
		}
		if booked > 1 {
			return leads.ErrMergeConflict
		}

		if err := moveChildren(ctx, tx, keepID, drop); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM leads WHERE id = $1::uuid`, dropID); err != nil {
			return err
		}
		return saveMerged(ctx, tx, leads.Merge(keep, drop))
	})
	return keepID, err
}

// lockPair locks both leads, always in id order, so two people merging related
// leads at once cannot deadlock each other.
func lockPair(ctx context.Context, tx pgx.Tx, keepID, dropID string) (keep, drop models.Lead, err error) {
	rows, err := tx.Query(ctx, `
		SELECT `+leadColumns+` FROM leads WHERE id IN ($1::uuid, $2::uuid) ORDER BY id FOR UPDATE`, keepID, dropID)
	if err != nil {
		return keep, drop, err
	}
	defer rows.Close()

	found := 0
	for rows.Next() {
		var l models.Lead
		if err := scanLead(rows, &l); err != nil {
			return keep, drop, err
		}
		if l.ID == keepID {
			keep = l
		} else {
			drop = l
		}
		found++
	}
	if err := rows.Err(); err != nil {
		return keep, drop, err
	}
	if found != 2 {
		return keep, drop, leads.ErrNotDuplicate
	}
	return keep, drop, nil
}

// moveChildren re-attaches the duplicate's history to the original.
func moveChildren(ctx context.Context, tx pgx.Tx, keepID string, drop models.Lead) error {
	steps := []struct {
		sql  string
		args []any
	}{
		{`UPDATE lead_touches SET lead_id = $1::uuid WHERE lead_id = $2::uuid`, []any{keepID, drop.ID}},
		{`UPDATE conversations SET lead_id = $1::uuid WHERE lead_id = $2::uuid`, []any{keepID, drop.ID}},
		{`UPDATE visits SET lead_id = $1::uuid WHERE lead_id = $2::uuid`, []any{keepID, drop.ID}},
		// Other leads flagged against the duplicate now point at the original.
		{`UPDATE leads SET possible_duplicate_of = $1::uuid WHERE possible_duplicate_of = $2::uuid AND id <> $1::uuid`, []any{keepID, drop.ID}},
		// A trail for later: the duplicate's own code no longer exists after the merge.
		{`INSERT INTO lead_touches (lead_id, source, project_id, note) VALUES ($1::uuid, $2, $3, $4)`,
			[]any{keepID, drop.Source, drop.ProjectID, "Merged duplicate " + drop.Code}},
	}
	for _, st := range steps {
		if _, err := tx.Exec(ctx, st.sql, st.args...); err != nil {
			return err
		}
	}
	return nil
}

// saveMerged writes the combined details onto the original lead.
func saveMerged(ctx context.Context, tx pgx.Tx, m models.Lead) error {
	_, err := tx.Exec(ctx, `
		UPDATE leads SET
			project_id = $2, wants = $3, budget_min_lakh = $4, budget_max_lakh = $5, status = $6,
			consent = $7, consent_at = $8, opted_out = $9, opted_out_at = $10,
			assigned_to = $11::uuid, first_response_at = $12,
			possible_duplicate_of = NULL, updated_at = now()
		WHERE id = $1::uuid`,
		m.ID, m.ProjectID, m.Wants, m.BudgetMinLakh, m.BudgetMaxLakh, m.Status,
		m.Consent, m.ConsentAt, m.OptedOut, m.OptedOutAt, m.AssignedTo, m.FirstResponseAt)
	return err
}
