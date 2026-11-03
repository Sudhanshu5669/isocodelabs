package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"leadflow/internal/models"
)

// Projects lists every project, by name.
func (s *Store) Projects(ctx context.Context) ([]models.Project, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, name, rera_number, locality, address, latitude, longitude
		FROM projects ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.Project{}
	for rows.Next() {
		var p models.Project
		if err := rows.Scan(&p.ID, &p.Name, &p.ReraNumber, &p.Locality, &p.Address, &p.Latitude, &p.Longitude); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// Project loads one project.
//
// Errors: ErrNotFound.
func (s *Store) Project(ctx context.Context, id string) (models.Project, error) {
	var p models.Project
	err := s.pool.QueryRow(ctx, `
		SELECT id, name, rera_number, locality, address, latitude, longitude
		FROM projects WHERE id = $1`, id,
	).Scan(&p.ID, &p.Name, &p.ReraNumber, &p.Locality, &p.Address, &p.Latitude, &p.Longitude)
	if errors.Is(err, pgx.ErrNoRows) {
		return models.Project{}, ErrNotFound
	}
	return p, err
}

// SaveFact adds or changes one fact-sheet entry. The chat assistant reads the
// fact sheet on every message, so a change is live for the very next buyer.
//
// Errors: ErrNotFound (no such project).
func (s *Store) SaveFact(ctx context.Context, projectID, topic, label, value, userID string) (models.FactSheetEntry, error) {
	var f models.FactSheetEntry
	err := s.pool.QueryRow(ctx, `
		INSERT INTO fact_sheet_entries (project_id, topic, label, value, updated_by, updated_at)
		VALUES ($1, $2, $3, $4, $5::uuid, now())
		ON CONFLICT (project_id, topic) DO UPDATE
		SET label = EXCLUDED.label, value = EXCLUDED.value,
		    updated_by = EXCLUDED.updated_by, updated_at = now()
		RETURNING id::text, project_id, topic, label, value, updated_by::text, updated_at`,
		projectID, topic, label, value, userID,
	).Scan(&f.ID, &f.ProjectID, &f.Topic, &f.Label, &f.Value, &f.UpdatedBy, &f.UpdatedAt)
	if pe := pgError(err); pe != nil && pe.Code == "23503" { // foreign key: unknown project
		return models.FactSheetEntry{}, ErrNotFound
	}
	return f, err
}

// DeleteFact removes one fact-sheet entry. From then on the assistant treats
// the topic as unknown and hands such questions to a person.
//
// Errors: ErrNotFound.
func (s *Store) DeleteFact(ctx context.Context, projectID, topic string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM fact_sheet_entries WHERE project_id = $1 AND topic = $2`, projectID, topic)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// -----------------------------------------------------------------------------
// Marketing spend per source and month (for cost per visit)
// -----------------------------------------------------------------------------

// SpendInRange lists spend rows for the months from..to (both first-of-month dates).
func (s *Store) SpendInRange(ctx context.Context, from, to time.Time) ([]models.SourceSpend, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id::text, source, month, amount_inr FROM source_spend
		WHERE month >= $1::date AND month <= $2::date
		ORDER BY month, source`, from.Format(time.DateOnly), to.Format(time.DateOnly))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.SourceSpend{}
	for rows.Next() {
		var sp models.SourceSpend
		if err := rows.Scan(&sp.ID, &sp.Source, &sp.Month, &sp.AmountINR); err != nil {
			return nil, err
		}
		out = append(out, sp)
	}
	return out, rows.Err()
}

// SaveSpend records what was spent on a source in a month (month is its first day).
func (s *Store) SaveSpend(ctx context.Context, source string, month time.Time, amountINR int64) (models.SourceSpend, error) {
	var sp models.SourceSpend
	err := s.pool.QueryRow(ctx, `
		INSERT INTO source_spend (source, month, amount_inr) VALUES ($1, $2::date, $3)
		ON CONFLICT (source, month) DO UPDATE SET amount_inr = EXCLUDED.amount_inr
		RETURNING id::text, source, month, amount_inr`,
		source, month.Format(time.DateOnly), amountINR,
	).Scan(&sp.ID, &sp.Source, &sp.Month, &sp.AmountINR)
	return sp, err
}
