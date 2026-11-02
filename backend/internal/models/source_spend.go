package models

import "time"

// SourceSpend is a row of the `source_spend` table: what was spent on one
// lead source in one month. Used for cost per visit on the dashboard.
type SourceSpend struct {
	ID        string    `db:"id" json:"id"`
	Source    string    `db:"source" json:"source"`
	Month     time.Time `db:"month" json:"month"` // first day of the month
	AmountINR int64     `db:"amount_inr" json:"amountInr"`
}
