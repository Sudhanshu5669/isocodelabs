package models

import "time"

// Conversation is a row of the `conversations` table: one chat with the assistant.
type Conversation struct {
	ID       string `db:"id" json:"id"`
	LeadID   string `db:"lead_id" json:"leadId"`
	Language string `db:"language" json:"language"`

	Summary     *string    `db:"summary" json:"summary"` // AI-written handoff summary
	HandedOffAt *time.Time `db:"handed_off_at" json:"handedOffAt"`

	// Flagged chats appear in the sales head's daily review queue.
	Flagged        bool       `db:"flagged" json:"flagged"`
	FlagReason     *string    `db:"flag_reason" json:"flagReason"`
	FlagReviewedAt *time.Time `db:"flag_reviewed_at" json:"flagReviewedAt"`
	FlagReviewedBy *string    `db:"flag_reviewed_by" json:"flagReviewedBy"`

	CreatedAt time.Time `db:"created_at" json:"createdAt"`
}
