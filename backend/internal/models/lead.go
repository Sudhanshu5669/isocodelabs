package models

import "time"

// Where a lead came from (leads.source and lead_touches.source).
const (
	SourcePropertyPortal = "property_portal"
	SourceSocialAd       = "social_ad"
	SourceWebsite        = "website"
	SourceHoardingCall   = "hoarding_call"
	SourceManual         = "manual"
)

// Lead stages (leads.status).
const (
	LeadNew         = "new"
	LeadEngaged     = "engaged"
	LeadVisitBooked = "visit_booked"
	LeadVisited     = "visited"
	LeadLost        = "lost"
)

// Languages the assistant speaks (leads.language, conversations.language).
const (
	LangEnglish  = "en"
	LangHindi    = "hi"
	LangGujarati = "gu"
)

// Lead is a row of the `leads` table: one prospective buyer.
type Lead struct {
	ID    string `db:"id" json:"id"`
	Code  string `db:"code" json:"code"` // human-friendly, e.g. "LD-58210"
	Name  string `db:"name" json:"name"`
	Phone string `db:"phone" json:"phone"` // E.164, unique: the exact-duplicate guard

	Source    string  `db:"source" json:"source"`
	ProjectID *string `db:"project_id" json:"projectId"`
	Wants     *string `db:"wants" json:"wants"` // "2 BHK" or "3 BHK"

	BudgetMinLakh *int `db:"budget_min_lakh" json:"budgetMinLakh"`
	BudgetMaxLakh *int `db:"budget_max_lakh" json:"budgetMaxLakh"`

	Language string `db:"language" json:"language"`
	Status   string `db:"status" json:"status"`

	// DPDP Act: contact only with consent, and honour opt-outs.
	Consent    bool       `db:"consent" json:"consent"`
	ConsentAt  *time.Time `db:"consent_at" json:"consentAt"`
	OptedOut   bool       `db:"opted_out" json:"optedOut"`
	OptedOutAt *time.Time `db:"opted_out_at" json:"optedOutAt"`

	AssignedTo      *string    `db:"assigned_to" json:"assignedTo"`
	FirstResponseAt *time.Time `db:"first_response_at" json:"firstResponseAt"`

	// Set when a fuzzy match suggests this is the same person as another lead.
	// A human confirms it; leads are never merged silently.
	PossibleDuplicateOf *string `db:"possible_duplicate_of" json:"possibleDuplicateOf"`

	CreatedAt time.Time `db:"created_at" json:"createdAt"`
	UpdatedAt time.Time `db:"updated_at" json:"updatedAt"`
}
