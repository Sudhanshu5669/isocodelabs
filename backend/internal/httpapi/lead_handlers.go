package httpapi

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"

	"leadflow/internal/auth"
	"leadflow/internal/leads"
	"leadflow/internal/models"
	"leadflow/internal/store"
)

// All lead endpoints are for signed-in staff. The sales head sees and manages
// every lead; an executive sees only the leads assigned to them. That rule is
// enforced in the store's SQL, so a handler cannot forget it.

const (
	maxImportRows = 200
	maxSearchLen  = 80
	maxOffset     = 100000
)

// viewer describes the signed-in user for the store.
func viewer(u auth.User) store.Viewer {
	return store.Viewer{ID: u.ID, IsHead: u.Role == models.RoleHead}
}

// ---------------------------------------------------------------------------
// Adding leads: one at a time, or a batch from a portal or ad export
// ---------------------------------------------------------------------------

type leadRequest struct {
	Name      string `json:"name"`
	Phone     string `json:"phone"`
	Source    string `json:"source"`
	ProjectID string `json:"projectId"`
	Wants     string `json:"wants"`
	Language  string `json:"language"`
	Note      string `json:"note"`
	// Consent says the person agreed to be contacted (DPDP Act). The staff
	// member entering the lead is attesting to it.
	Consent bool `json:"consent"`
}

func (r leadRequest) input() leads.Input {
	return leads.Input{Name: r.Name, Phone: r.Phone, Source: r.Source, ProjectID: r.ProjectID,
		Wants: r.Wants, Language: r.Language, Note: r.Note, Consent: r.Consent}
}

// ingestResponse is what a caller learns about a saved enquiry. Lead is left
// out when the person already belongs to another executive, so adding a phone
// number can never be used to read someone else's lead.
type ingestResponse struct {
	Outcome         string       `json:"outcome"` // "created" or "merged"
	Lead            *models.Lead `json:"lead,omitempty"`
	DuplicateOfCode string       `json:"duplicateOfCode,omitempty"`
}

// createLead saves one enquiry, e.g. a hoarding call logged by the team.
// An executive who adds a new lead becomes its owner; for the head it starts
// unassigned.
func (s *Server) createLead(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.UserFromContext(r.Context())

	var req leadRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	res, err := s.ingest(r, user, req)
	if err != nil {
		s.writeLeadError(w, err)
		return
	}
	status := http.StatusCreated
	if res.Outcome == store.OutcomeMerged {
		status = http.StatusOK
	}
	writeJSON(w, status, respondFor(user, res))
}

type importRequest struct {
	Leads []leadRequest `json:"leads"`
}

type importRow struct {
	Row     int    `json:"row"` // 1-based position in the request
	Outcome string `json:"outcome,omitempty"`
	Code    string `json:"code,omitempty"`
	Error   string `json:"error,omitempty"`
}

type importResponse struct {
	Results []importRow `json:"results"`
	Created int         `json:"created"`
	Merged  int         `json:"merged"`
	Failed  int         `json:"failed"`
}

// importLeads saves up to 200 enquiries at once. Each row stands alone: a bad
// row is reported and the rest are still saved.
func (s *Server) importLeads(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.UserFromContext(r.Context())

	var req importRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(req.Leads) == 0 || len(req.Leads) > maxImportRows {
		writeError(w, http.StatusBadRequest, "Send between 1 and 200 leads at a time.")
		return
	}

	resp := importResponse{Results: make([]importRow, 0, len(req.Leads))}
	for i, row := range req.Leads {
		out := importRow{Row: i + 1}
		res, err := s.ingest(r, user, row)
		var ve *leads.ValidationError
		switch {
		case errors.As(err, &ve):
			out.Error = ve.Message
			resp.Failed++
		case err != nil:
			slog.Error("request failed", "what", "import lead", "row", i+1, "error", err)
			out.Error = "This lead could not be saved."
			resp.Failed++
		default:
			out.Outcome = res.Outcome
			if shown := respondFor(user, res); shown.Lead != nil {
				out.Code = shown.Lead.Code
			}
			if res.Outcome == store.OutcomeCreated {
				resp.Created++
			} else {
				resp.Merged++
			}
		}
		resp.Results = append(resp.Results, out)
	}
	writeJSON(w, http.StatusOK, resp)
}

// ingest validates one request and saves it.
func (s *Server) ingest(r *http.Request, user auth.User, req leadRequest) (store.IngestResult, error) {
	in, err := req.input().Clean()
	if err != nil {
		return store.IngestResult{}, err
	}
	owner := ""
	if user.Role != models.RoleHead {
		owner = user.ID
	}
	return s.store.IngestLead(r.Context(), in, owner)
}

// respondFor hides the lead's details from an executive who does not own it.
func respondFor(user auth.User, res store.IngestResult) ingestResponse {
	out := ingestResponse{Outcome: res.Outcome, DuplicateOfCode: res.DuplicateOfCode}
	if user.Role == models.RoleHead || (res.Lead.AssignedTo != nil && *res.Lead.AssignedTo == user.ID) {
		lead := res.Lead
		out.Lead = &lead
	}
	return out
}

// ---------------------------------------------------------------------------
// Reading leads
// ---------------------------------------------------------------------------

// listLeads returns a page of leads, e.g.
// GET /api/staff/leads?source=social_ad&project=project-2&needsAttention=true&q=patel&limit=25&offset=0
func (s *Server) listLeads(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.UserFromContext(r.Context())
	q := r.URL.Query()

	f := store.LeadFilter{
		Source:         q.Get("source"),
		ProjectID:      q.Get("project"),
		Status:         q.Get("status"),
		Assignee:       q.Get("assignee"),
		Query:          q.Get("q"),
		NeedsAttention: q.Get("needsAttention") == "true",
	}
	switch {
	case f.Source != "" && !leads.ValidSource(f.Source):
		writeError(w, http.StatusBadRequest, "That lead source is not recognised.")
		return
	case f.ProjectID != "" && !reProject.MatchString(f.ProjectID):
		writeError(w, http.StatusBadRequest, "That project is not recognised.")
		return
	case f.Status != "" && !leads.ValidStatus(f.Status):
		writeError(w, http.StatusBadRequest, "That status is not recognised.")
		return
	case f.Assignee != "" && f.Assignee != "unassigned" && !reUUID.MatchString(f.Assignee):
		writeError(w, http.StatusBadRequest, "That executive is not recognised.")
		return
	case utf8.RuneCountInString(f.Query) > maxSearchLen:
		writeError(w, http.StatusBadRequest, "The search is too long.")
		return
	}

	var ok bool
	if f.Limit, ok = intParam(w, q.Get("limit"), 25, 1, 100, "limit"); !ok {
		return
	}
	if f.Offset, ok = intParam(w, q.Get("offset"), 0, 0, maxOffset, "offset"); !ok {
		return
	}

	page, err := s.store.ListLeads(r.Context(), viewer(user), f)
	if err != nil {
		s.serverError(w, "list leads", err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

// intParam reads an optional whole number from the query string.
func intParam(w http.ResponseWriter, raw string, def, lo, hi int, name string) (int, bool) {
	if raw == "" {
		return def, true
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < lo || n > hi {
		writeError(w, http.StatusBadRequest, "The "+name+" must be a whole number from "+strconv.Itoa(lo)+" to "+strconv.Itoa(hi)+".")
		return 0, false
	}
	return n, true
}

// getLead returns one lead with its enquiry history and visit.
func (s *Server) getLead(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.UserFromContext(r.Context())
	id := chi.URLParam(r, "id")
	if !reUUID.MatchString(id) {
		writeError(w, http.StatusNotFound, "We could not find that lead.")
		return
	}
	detail, err := s.store.GetLead(r.Context(), viewer(user), id)
	if err != nil {
		s.writeLeadError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

// ---------------------------------------------------------------------------
// Sales head only: hand out leads and settle possible duplicates
// ---------------------------------------------------------------------------

type assignRequest struct {
	ExecutiveID string `json:"executiveId"`
}

func (s *Server) assignLead(w http.ResponseWriter, r *http.Request) {
	id, ok := leadIDParam(w, r)
	if !ok {
		return
	}
	var req assignRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !reUUID.MatchString(req.ExecutiveID) {
		writeError(w, http.StatusBadRequest, "Choose an executive.")
		return
	}
	if err := s.store.AssignLead(r.Context(), id, req.ExecutiveID); err != nil {
		s.writeLeadError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type duplicateRequest struct {
	Action string `json:"action"` // "merge" or "dismiss"
}

// resolveDuplicate settles a lead flagged as a possible duplicate: merge it into
// the original, or dismiss the flag because they are different people.
func (s *Server) resolveDuplicate(w http.ResponseWriter, r *http.Request) {
	id, ok := leadIDParam(w, r)
	if !ok {
		return
	}
	var req duplicateRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	switch req.Action {
	case "merge":
		keepID, err := s.store.MergeDuplicate(r.Context(), id)
		if err != nil {
			s.writeLeadError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"leadId": keepID}) // the lead that remains
	case "dismiss":
		if err := s.store.DismissDuplicate(r.Context(), id); err != nil {
			s.writeLeadError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		writeError(w, http.StatusBadRequest, "Action must be merge or dismiss.")
	}
}

func leadIDParam(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := chi.URLParam(r, "id")
	if !reUUID.MatchString(id) {
		writeError(w, http.StatusNotFound, "We could not find that lead.")
		return "", false
	}
	return id, true
}

// writeLeadError turns a lead error into a clear message and status. Anything
// unexpected is logged in full but shown only as a generic message.
func (s *Server) writeLeadError(w http.ResponseWriter, err error) {
	var ve *leads.ValidationError
	switch {
	case errors.As(err, &ve):
		writeError(w, http.StatusBadRequest, ve.Message)
	case errors.Is(err, leads.ErrNotFound):
		writeError(w, http.StatusNotFound, "We could not find that lead.")
	case errors.Is(err, leads.ErrNotDuplicate):
		writeError(w, http.StatusConflict, "This lead is not flagged as a possible duplicate.")
	case errors.Is(err, leads.ErrMergeConflict):
		writeError(w, http.StatusConflict, "Both leads have a booked visit. Cancel one visit first.")
	case errors.Is(err, leads.ErrAssigneeNotFound):
		writeError(w, http.StatusBadRequest, "Choose an active executive.")
	default:
		s.serverError(w, "lead", err)
	}
}
