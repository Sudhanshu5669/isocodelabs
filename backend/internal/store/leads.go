package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"leadflow/internal/leads"
	"leadflow/internal/models"
	"leadflow/internal/pii"
)

// =============================================================================
// LEAD INGEST AND THE DUPLICATE CHECK
//
// Every enquiry, from every source, goes through IngestLead:
//
//  1. Exact duplicate: the phone number is the identity. The phone is stored
//     encrypted, but its blind index (leads.phone_bidx, an HMAC of the
//     normalised number) is UNIQUE, so the database itself still guarantees one
//     person is never two leads, even if two enquiries arrive at the same
//     instant. A repeat enquiry is folded into the existing lead and recorded
//     as a "touch" with its own source.
//
// Name and phone are personal data (DPDP): they pass through pii.Seal on the
// way in and pii.Open on the way out, and are never stored as plain text.
//  2. Fuzzy duplicate: a brand-new lead whose name looks like another lead's for
//     the same project is flagged (possible_duplicate_of). Nothing is merged
//     automatically; the sales head confirms or dismisses it (leads_merge.go).
//
// Files: leads.go (this file, writes) · leads_query.go (list and detail) ·
// leads_merge.go (confirm or dismiss a flagged duplicate).
// =============================================================================

// Outcomes of IngestLead.
const (
	OutcomeCreated = "created" // a new lead
	OutcomeMerged  = "merged"  // the phone number already existed; recorded as a repeat enquiry
)

// maxLookalikeCandidates bounds the fuzzy comparison. At about 3,000 enquiries a
// month this covers many months of one project's leads; see the TDD for the
// pg_trgm alternative if volume grows.
const maxLookalikeCandidates = 5000

// leadColumns is every column of the leads table, in the order scanLead expects.
const leadColumns = `id::text, code, name, phone, source, project_id, wants, budget_min_lakh, budget_max_lakh, timeline,
	language, status, consent, consent_at, opted_out, opted_out_at, assigned_to::text, first_response_at,
	possible_duplicate_of::text, created_at, updated_at`

// scanner is satisfied by both a single row and a row inside a result set.
type scanner interface{ Scan(dest ...any) error }

func scanLead(sc scanner, l *models.Lead) error {
	return sc.Scan(&l.ID, &l.Code, pii.Open(pii.Name, &l.Name), pii.Open(pii.Phone, &l.Phone), &l.Source, &l.ProjectID, &l.Wants, &l.BudgetMinLakh,
		&l.BudgetMaxLakh, &l.Timeline, &l.Language, &l.Status, &l.Consent, &l.ConsentAt, &l.OptedOut, &l.OptedOutAt,
		&l.AssignedTo, &l.FirstResponseAt, &l.PossibleDuplicateOf, &l.CreatedAt, &l.UpdatedAt)
}

// IngestResult tells the caller what happened to an enquiry.
type IngestResult struct {
	Lead            models.Lead `json:"lead"`
	Outcome         string      `json:"outcome"`                   // OutcomeCreated or OutcomeMerged
	DuplicateOfCode string      `json:"duplicateOfCode,omitempty"` // set when a look-alike was flagged
}

// IngestLead saves one enquiry. The input must already have passed Input.Clean.
//
// assignTo is the executive who should own a NEW lead ("" leaves it unassigned
// for the head to hand out). It is ignored when the enquiry merges into an
// existing lead, so a repeat enquiry never takes a lead away from its owner.
//
// Errors: *leads.ValidationError (unknown project).
func (s *Store) IngestLead(ctx context.Context, in leads.Input, assignTo string) (IngestResult, error) {
	var res IngestResult
	err := s.withTx(ctx, func(tx pgx.Tx) error {
		res = IngestResult{} // the transaction may be retried, so start clean

		created, err := insertLead(ctx, tx, in, assignTo, &res.Lead)
		if err != nil {
			return err
		}
		if created {
			res.Outcome = OutcomeCreated
			note := in.Note
			if note == "" {
				note = "First enquiry"
			}
			if err := addTouch(ctx, tx, res.Lead.ID, in.Source, in.ProjectID, note); err != nil {
				return err
			}
			res.DuplicateOfCode, err = flagLookalike(ctx, tx, &res.Lead)
			return err
		}

		res.Outcome = OutcomeMerged
		note := in.Note
		if note == "" {
			note = "Enquired again; merged by phone number"
		}
		return mergeRepeatEnquiry(ctx, tx, in, note, &res.Lead)
	})
	return res, err
}

// insertLead inserts the lead unless the phone number already exists. It
// reports whether a row was created; when it was not, nothing is written.
func insertLead(ctx context.Context, tx pgx.Tx, in leads.Input, assignTo string, out *models.Lead) (created bool, err error) {
	err = scanLead(tx.QueryRow(ctx, `
		INSERT INTO leads (name, phone, phone_bidx, name_words, source, project_id, wants, language, consent, consent_at, assigned_to)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9::boolean, CASE WHEN $9::boolean THEN now() END, $10::uuid)
		ON CONFLICT (phone_bidx) DO NOTHING
		RETURNING `+leadColumns,
		pii.Seal(pii.Name, in.Name), pii.Seal(pii.Phone, in.Phone), pii.PhoneIndex(in.Phone), pii.NameWordIndexes(in.Name),
		in.Source, nullable(in.ProjectID), nullable(in.Wants), in.Language, in.Consent, nullable(assignTo),
	), out)

	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, pgx.ErrNoRows): // ON CONFLICT DO NOTHING returned no row: the phone exists
		return false, nil
	case pgError(err) != nil && pgError(err).Code == "23503": // foreign key: unknown project
		return false, &leads.ValidationError{Field: "projectId", Message: "That project does not exist."}
	}
	return false, err
}

// mergeRepeatEnquiry folds a repeat enquiry into the lead that owns the phone.
// The lead keeps its first source, name and owner; it only gains details it did
// not have, and a new consent. An opt-out is never cleared by a new enquiry.
func mergeRepeatEnquiry(ctx context.Context, tx pgx.Tx, in leads.Input, note string, out *models.Lead) error {
	var id string
	if err := tx.QueryRow(ctx, `SELECT id::text FROM leads WHERE phone_bidx = $1 FOR UPDATE`, pii.PhoneIndex(in.Phone)).Scan(&id); err != nil {
		return err
	}
	if err := addTouch(ctx, tx, id, in.Source, in.ProjectID, note); err != nil {
		return err
	}
	err := scanLead(tx.QueryRow(ctx, `
		UPDATE leads SET
			project_id = COALESCE(project_id, $2::text),
			wants      = COALESCE(wants, $3::text),
			consent_at = CASE WHEN NOT consent AND $4::boolean THEN now() ELSE consent_at END,
			consent    = consent OR $4::boolean,
			updated_at = now()
		WHERE id = $1::uuid
		RETURNING `+leadColumns,
		id, nullable(in.ProjectID), nullable(in.Wants), in.Consent), out)
	if pgError(err) != nil && pgError(err).Code == "23503" {
		return &leads.ValidationError{Field: "projectId", Message: "That project does not exist."}
	}
	return err
}

func addTouch(ctx context.Context, tx pgx.Tx, leadID, source, projectID, note string) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO lead_touches (lead_id, source, project_id, note) VALUES ($1::uuid, $2, $3, $4)`,
		leadID, source, nullable(projectID), pii.SealOptional(pii.Text, note))
	if pgError(err) != nil && pgError(err).Code == "23503" {
		return &leads.ValidationError{Field: "projectId", Message: "That project does not exist."}
	}
	return err
}

// flagLookalike looks for an older lead of the same project with a similar name
// and, if there is one, marks the new lead as its possible duplicate. It
// returns the older lead's code, or "" when nothing looked alike.
func flagLookalike(ctx context.Context, tx pgx.Tx, lead *models.Lead) (string, error) {
	id, code, err := findLookalike(ctx, tx, lead)
	if err != nil || id == "" {
		return "", err
	}
	if _, err := tx.Exec(ctx, `UPDATE leads SET possible_duplicate_of = $2::uuid WHERE id = $1::uuid`, lead.ID, id); err != nil {
		return "", err
	}
	lead.PossibleDuplicateOf = &id
	return code, nil
}

// findLookalike returns the OLDEST lead in the same project whose name is
// similar, so a chain of duplicates all point at the original record.
func findLookalike(ctx context.Context, tx pgx.Tx, lead *models.Lead) (id, code string, err error) {
	rows, err := tx.Query(ctx, `
		SELECT id::text, code, name FROM leads
		WHERE id <> $1::uuid AND project_id IS NOT DISTINCT FROM $2::text
		ORDER BY created_at DESC LIMIT $3`, lead.ID, lead.ProjectID, maxLookalikeCandidates)
	if err != nil {
		return "", "", err
	}
	defer rows.Close()

	// Names are encrypted, so the fuzzy comparison happens here, after
	// decryption, exactly as before; only the database can no longer read them.
	for rows.Next() { // newest first, so the last match is the oldest
		var cid, ccode, cname string
		if err := rows.Scan(&cid, &ccode, pii.Open(pii.Name, &cname)); err != nil {
			return "", "", err
		}
		if leads.NamesSimilar(lead.Name, cname) {
			id, code = cid, ccode
		}
	}
	return id, code, rows.Err()
}

// AssignLead hands a lead to an executive (used by the sales head).
//
// Errors: leads.ErrNotFound, leads.ErrAssigneeNotFound.
func (s *Store) AssignLead(ctx context.Context, leadID, executiveID string) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE leads SET assigned_to = u.id, updated_at = now()
		FROM users u
		WHERE leads.id = $1::uuid AND u.id = $2::uuid AND u.role = 'executive' AND u.active`,
		leadID, executiveID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 1 {
		return nil
	}
	var exists bool
	if err := s.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM leads WHERE id = $1::uuid)`, leadID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return leads.ErrNotFound
	}
	return leads.ErrAssigneeNotFound
}
