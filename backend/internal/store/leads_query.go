package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"leadflow/internal/leads"
	"leadflow/internal/models"
	"leadflow/internal/pii"
)

// Who may see which lead:
//   - the sales head sees every lead;
//   - an executive sees only the leads assigned to them. Unassigned leads wait
//     for the head to hand them out.
// Both queries below enforce this in SQL, so no handler can forget it.

// Viewer is the signed-in staff member asking to see leads.
type Viewer struct {
	ID     string
	IsHead bool
}

// LeadRow is one line of the lead list.
type LeadRow struct {
	ID                   string     `json:"id"`
	Code                 string     `json:"code"`
	Name                 string     `json:"name"`
	Phone                string     `json:"phone"`
	Source               string     `json:"source"`
	ProjectID            *string    `json:"projectId"`
	ProjectName          *string    `json:"projectName"`
	Wants                *string    `json:"wants"`
	Status               string     `json:"status"`
	Language             string     `json:"language"`
	Consent              bool       `json:"consent"`
	OptedOut             bool       `json:"optedOut"`
	AssignedTo           *string    `json:"assignedTo"`
	AssigneeName         *string    `json:"assigneeName"`
	FirstResponseAt      *time.Time `json:"firstResponseAt"`
	FirstResponseSeconds *int64     `json:"firstResponseSeconds"` // how long the first reply took; null if none yet
	DuplicateOfCode      *string    `json:"duplicateOfCode"`      // set when flagged as a possible duplicate
	CreatedAt            time.Time  `json:"createdAt"`
}

// The columns and joins shared by the list and the detail query.
const (
	leadRowColumns = `l.id::text, l.code, l.name, l.phone, l.source, l.project_id, p.name, l.wants, l.status,
		l.language, l.consent, l.opted_out, l.assigned_to::text, u.name, l.first_response_at,
		EXTRACT(EPOCH FROM (l.first_response_at - l.created_at))::bigint, d.code, l.created_at`
	leadRowFrom = `FROM leads l
		LEFT JOIN projects p ON p.id = l.project_id
		LEFT JOIN users u ON u.id = l.assigned_to
		LEFT JOIN leads d ON d.id = l.possible_duplicate_of`
)

// scanLeadRow fills r, then any extra destinations the caller selected after it.
func scanLeadRow(sc scanner, r *LeadRow, extra ...any) error {
	dest := []any{&r.ID, &r.Code, pii.Open(pii.Name, &r.Name), pii.Open(pii.Phone, &r.Phone), &r.Source, &r.ProjectID,
		&r.ProjectName, &r.Wants, &r.Status, &r.Language, &r.Consent, &r.OptedOut, &r.AssignedTo,
		pii.OpenPtr(pii.Name, &r.AssigneeName), &r.FirstResponseAt,
		&r.FirstResponseSeconds, &r.DuplicateOfCode, &r.CreatedAt}
	return sc.Scan(append(dest, extra...)...)
}

// LeadFilter narrows the lead list. Zero values mean "no filter".
type LeadFilter struct {
	Source         string
	ProjectID      string
	Status         string
	Assignee       string // head only: "unassigned" or an executive id. Ignored for executives.
	NeedsAttention bool
	// Query matches part of a lead code, whole words of the name (every word
	// typed must appear, in any order: "shah aarav" finds "Aarav Shah"), or a
	// complete phone number. Names and phones are encrypted, so partial matches
	// on them are not possible; see internal/pii.
	Query  string
	Limit  int // default 25, at most 100
	Offset int
}

// NeedsAttentionSQL is what "needs attention" means: a lead that is still open
// and either has not had a first reply yet or is flagged as a possible duplicate.
const needsAttentionSQL = `(l.status IN ('new', 'engaged') AND (l.first_response_at IS NULL OR l.possible_duplicate_of IS NOT NULL))`

// LeadPage is one page of the lead list plus the total matching leads.
type LeadPage struct {
	Leads []LeadRow `json:"leads"`
	Total int       `json:"total"`
}

// ListLeads returns the leads the viewer may see, newest first.
func (s *Store) ListLeads(ctx context.Context, v Viewer, f LeadFilter) (LeadPage, error) {
	if f.Limit <= 0 || f.Limit > 100 {
		f.Limit = 25
	}
	if f.Offset < 0 {
		f.Offset = 0
	}

	// Conditions use "?" as a placeholder; add numbers them ($1, $2, ...). Only
	// fixed text from this function is ever placed in the SQL, never user input:
	// every value travels as a bound argument.
	var conds []string
	var args []any
	add := func(cond string, value any) {
		args = append(args, value)
		conds = append(conds, strings.ReplaceAll(cond, "?", fmt.Sprintf("$%d", len(args))))
	}

	if v.IsHead {
		switch f.Assignee {
		case "":
		case "unassigned":
			conds = append(conds, "l.assigned_to IS NULL")
		default:
			add("l.assigned_to = ?::uuid", f.Assignee)
		}
	} else {
		add("l.assigned_to = ?::uuid", v.ID)
	}
	if f.Source != "" {
		add("l.source = ?", f.Source)
	}
	if f.ProjectID != "" {
		add("l.project_id = ?", f.ProjectID)
	}
	if f.Status != "" {
		add("l.status = ?", f.Status)
	}
	if f.NeedsAttention {
		conds = append(conds, needsAttentionSQL)
	}
	if q := strings.TrimSpace(f.Query); q != "" {
		conds = append(conds, searchCondition(q, &args))
	}

	where := ""
	if len(conds) > 0 {
		where = "WHERE " + strings.Join(conds, " AND ")
	}
	args = append(args, f.Limit, f.Offset)
	sql := fmt.Sprintf(`SELECT %s, count(*) OVER() %s %s ORDER BY l.created_at DESC, l.id LIMIT $%d OFFSET $%d`,
		leadRowColumns, leadRowFrom, where, len(args)-1, len(args))

	rows, err := s.pool.Query(ctx, sql, args...)
	if err != nil {
		return LeadPage{}, err
	}
	defer rows.Close()

	page := LeadPage{Leads: []LeadRow{}}
	for rows.Next() {
		var r LeadRow
		if err := scanLeadRow(rows, &r, &page.Total); err != nil {
			return LeadPage{}, err
		}
		page.Leads = append(page.Leads, r)
	}
	return page, rows.Err()
}

// searchCondition builds the search filter and appends its values to args:
//
//	l.code ILIKE '%q%'                     the code is not personal, so it stays plain
//	l.name_words @> {HMAC(word), ...}      every typed word is a word of the name
//	l.phone_bidx = HMAC(normalised phone)  only when q is a whole phone number
//
// "@>" means "contains": the lead's array of name-word indexes must contain
// every index of the typed words. A GIN index makes it fast.
func searchCondition(q string, args *[]any) string {
	next := func(v any) string {
		*args = append(*args, v)
		return fmt.Sprintf("$%d", len(*args))
	}
	or := []string{"l.code ILIKE " + next("%"+escapeLike(q)+"%")}
	if words := pii.NameWordIndexes(q); len(words) > 0 {
		or = append(or, "l.name_words @> "+next(words)+"::bytea[]")
	}
	if phone, err := leads.NormalizePhone(q); err == nil {
		or = append(or, "l.phone_bidx = "+next(pii.PhoneIndex(phone)))
	}
	return "(" + strings.Join(or, " OR ") + ")"
}

// escapeLike makes %, _ and \ in a search term match literally.
func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

// LeadRef points at another lead.
type LeadRef struct {
	ID   string `json:"id"`
	Code string `json:"code"`
	Name string `json:"name"`
}

// LeadVisit is a lead's most relevant visit (the upcoming booked one, else the latest).
type LeadVisit struct {
	ID            string    `json:"id"`
	Code          string    `json:"code"`
	Status        string    `json:"status"`
	StartsAt      time.Time `json:"startsAt"`
	ProjectName   string    `json:"projectName"`
	ExecutiveName string    `json:"executiveName"`
}

// LeadDetail is everything the lead page shows. The chat history joins it in
// the AI phase.
type LeadDetail struct {
	LeadRow
	BudgetMinLakh *int               `json:"budgetMinLakh"`
	BudgetMaxLakh *int               `json:"budgetMaxLakh"`
	Timeline      *string            `json:"timeline"`
	ConsentAt     *time.Time         `json:"consentAt"`
	OptedOutAt    *time.Time         `json:"optedOutAt"`
	DuplicateOf   *LeadRef           `json:"duplicateOf"`
	Touches       []models.LeadTouch `json:"touches"`
	Visit         *LeadVisit         `json:"visit"`
	Chats         []ChatTranscript   `json:"chats"` // AI chat history, newest first
}

// GetLead loads one lead, if the viewer is allowed to see it.
//
// Errors: leads.ErrNotFound (also returned for a lead an executive may not see,
// so a lead's existence is not revealed to them).
func (s *Store) GetLead(ctx context.Context, v Viewer, id string) (LeadDetail, error) {
	var d LeadDetail
	var dupID, dupName *string
	err := scanLeadRow(s.pool.QueryRow(ctx, `
		SELECT `+leadRowColumns+`, l.budget_min_lakh, l.budget_max_lakh, l.timeline, l.consent_at, l.opted_out_at,
		       l.possible_duplicate_of::text, d.name
		`+leadRowFrom+`
		WHERE l.id = $1::uuid AND ($2::boolean OR l.assigned_to = $3::uuid)`, id, v.IsHead, v.ID),
		&d.LeadRow, &d.BudgetMinLakh, &d.BudgetMaxLakh, &d.Timeline, &d.ConsentAt, &d.OptedOutAt, &dupID,
		pii.OpenPtr(pii.Name, &dupName))
	if errors.Is(err, pgx.ErrNoRows) {
		return LeadDetail{}, leads.ErrNotFound
	}
	if err != nil {
		return LeadDetail{}, err
	}
	if dupID != nil && dupName != nil && d.DuplicateOfCode != nil {
		d.DuplicateOf = &LeadRef{ID: *dupID, Code: *d.DuplicateOfCode, Name: *dupName}
	}

	if d.Touches, err = s.leadTouches(ctx, id); err != nil {
		return LeadDetail{}, err
	}
	if d.Visit, err = s.leadVisit(ctx, id); err != nil {
		return LeadDetail{}, err
	}
	if d.Chats, err = s.LeadChats(ctx, id); err != nil {
		return LeadDetail{}, err
	}
	return d, nil
}

func (s *Store) leadTouches(ctx context.Context, leadID string) ([]models.LeadTouch, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id::text, lead_id::text, source, project_id, note, created_at
		FROM lead_touches WHERE lead_id = $1::uuid ORDER BY created_at, id`, leadID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	touches := []models.LeadTouch{}
	for rows.Next() {
		var t models.LeadTouch
		if err := rows.Scan(&t.ID, &t.LeadID, &t.Source, &t.ProjectID, pii.OpenPtr(pii.Text, &t.Note), &t.CreatedAt); err != nil {
			return nil, err
		}
		touches = append(touches, t)
	}
	return touches, rows.Err()
}

// leadVisit returns the booked visit if there is one, otherwise the latest.
func (s *Store) leadVisit(ctx context.Context, leadID string) (*LeadVisit, error) {
	var v LeadVisit
	err := s.pool.QueryRow(ctx, `
		SELECT v.id::text, v.code, v.status, s.starts_at, p.name, u.name
		FROM visits v
		JOIN visit_slots s ON s.id = v.slot_id
		JOIN projects p ON p.id = s.project_id
		JOIN users u ON u.id = s.executive_id
		WHERE v.lead_id = $1::uuid
		ORDER BY (v.status = 'booked') DESC, s.starts_at DESC LIMIT 1`, leadID,
	).Scan(&v.ID, &v.Code, &v.Status, &v.StartsAt, &v.ProjectName, pii.Open(pii.Name, &v.ExecutiveName))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &v, nil
}
