package models

import "time"

// FactSheetEntry is a row of the `fact_sheet_entries` table. These rows are the
// only facts the chat assistant is allowed to state.
type FactSheetEntry struct {
	ID        string    `db:"id" json:"id"`
	ProjectID string    `db:"project_id" json:"projectId"`
	Topic     string    `db:"topic" json:"topic"` // machine name, e.g. "possession"
	Label     string    `db:"label" json:"label"` // title shown in the admin editor
	Value     string    `db:"value" json:"value"`
	UpdatedBy *string   `db:"updated_by" json:"updatedBy"` // nil if that user was deleted
	UpdatedAt time.Time `db:"updated_at" json:"updatedAt"`
}
