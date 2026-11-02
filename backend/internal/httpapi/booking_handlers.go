package httpapi

import (
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"time"

	"github.com/go-chi/chi/v5"

	"leadflow/internal/auth"
	"leadflow/internal/booking"
	"leadflow/internal/models"
	"leadflow/internal/store"
)

// visitDetail is the buyer-safe view of a visit (no phone number).
type visitDetail = store.VisitDetail

// Input shapes. Anything that does not match is rejected before it reaches SQL.
var (
	reProject = regexp.MustCompile(`^[a-z0-9-]{1,40}$`)
	reUUID    = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	reCode    = regexp.MustCompile(`^[0-9a-f]{24}$`)
)

const maxSlotWindow = 31 * 24 * time.Hour

// ---------------------------------------------------------------------------
// Public endpoints (buyers). No login: a buyer reaches their own visit through
// the unguessable code in their link.
// ---------------------------------------------------------------------------

// listSlots returns the free slots for a project, e.g.
// GET /api/slots?project=project-1&from=2026-11-03&to=2026-11-10
// Dates are days in Ahmedabad time. Defaults to the next 7 days.
func (s *Server) listSlots(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	project := q.Get("project")
	if !reProject.MatchString(project) {
		writeError(w, http.StatusBadRequest, "Choose a project.")
		return
	}

	now := s.now()
	today := startOfDay(now)
	from, to := today, today.Add(7*24*time.Hour)

	if v := q.Get("from"); v != "" {
		t, err := time.ParseInLocation("2006-01-02", v, booking.IST)
		if err != nil {
			writeError(w, http.StatusBadRequest, "The start date should look like 2026-11-03.")
			return
		}
		from, to = t, t.Add(7*24*time.Hour)
	}
	if v := q.Get("to"); v != "" {
		t, err := time.ParseInLocation("2006-01-02", v, booking.IST)
		if err != nil {
			writeError(w, http.StatusBadRequest, "The end date should look like 2026-11-10.")
			return
		}
		to = t.Add(24 * time.Hour) // "to" is inclusive for the caller
	}
	if !to.After(from) || to.Sub(from) > maxSlotWindow {
		writeError(w, http.StatusBadRequest, "Choose a date range of up to 31 days.")
		return
	}

	slots, err := s.store.FreeSlots(r.Context(), now, project, from, to)
	if err != nil {
		s.serverError(w, "list slots", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"slots": slots})
}

// getVisit returns what the buyer's visit page shows.
func (s *Server) getVisit(w http.ResponseWriter, r *http.Request) {
	detail, ok := s.visitByCode(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

// cancelVisitByCode lets a buyer cancel their own visit.
func (s *Server) cancelVisitByCode(w http.ResponseWriter, r *http.Request) {
	detail, ok := s.visitByCode(w, r)
	if !ok {
		return
	}
	if err := s.store.CancelVisit(r.Context(), s.now(), detail.ID); err != nil {
		s.writeBookingError(w, err)
		return
	}
	s.respondWithVisit(w, r, detail.Code)
}

type rescheduleRequest struct {
	SlotID string `json:"slotId"`
}

// rescheduleVisitByCode lets a buyer move their visit to another free slot.
func (s *Server) rescheduleVisitByCode(w http.ResponseWriter, r *http.Request) {
	detail, ok := s.visitByCode(w, r)
	if !ok {
		return
	}
	var req rescheduleRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !reUUID.MatchString(req.SlotID) {
		writeError(w, http.StatusBadRequest, "Choose a time slot.")
		return
	}
	if _, err := s.store.RescheduleVisit(r.Context(), s.now(), detail.ID, req.SlotID); err != nil {
		s.writeBookingError(w, err)
		return
	}
	s.respondWithVisit(w, r, detail.Code)
}

func (s *Server) visitByCode(w http.ResponseWriter, r *http.Request) (detail visitDetail, ok bool) {
	code := chi.URLParam(r, "code")
	if !reCode.MatchString(code) {
		writeError(w, http.StatusNotFound, "We could not find that visit.")
		return visitDetail{}, false
	}
	d, err := s.store.VisitDetailByCode(r.Context(), s.now(), code)
	if err != nil {
		s.writeBookingError(w, err)
		return visitDetail{}, false
	}
	return d, true
}

func (s *Server) respondWithVisit(w http.ResponseWriter, r *http.Request, code string) {
	d, err := s.store.VisitDetailByCode(r.Context(), s.now(), code)
	if err != nil {
		s.writeBookingError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

// ---------------------------------------------------------------------------
// Staff endpoints (signed in)
// ---------------------------------------------------------------------------

type staffBookRequest struct {
	LeadID string `json:"leadId"`
	SlotID string `json:"slotId"`
}

// staffBookVisit books a visit for a lead on their behalf. An executive may do
// this only for leads assigned to them (or not yet assigned); the head for any.
func (s *Server) staffBookVisit(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.UserFromContext(r.Context())

	var req staffBookRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !reUUID.MatchString(req.LeadID) || !reUUID.MatchString(req.SlotID) {
		writeError(w, http.StatusBadRequest, "Choose a lead and a time slot.")
		return
	}

	if user.Role != models.RoleHead {
		assignee, err := s.store.LeadAssignee(r.Context(), req.LeadID)
		if err != nil {
			s.writeBookingError(w, err)
			return
		}
		if assignee != nil && *assignee != user.ID {
			writeError(w, http.StatusForbidden, "This lead is assigned to another executive.")
			return
		}
	}

	visit, err := s.store.BookSlot(r.Context(), s.now(), req.LeadID, req.SlotID)
	if err != nil {
		s.writeBookingError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, visit)
}

// staffCancelVisit cancels a visit. Allowed for the head and for the executive who owns the slot.
func (s *Server) staffCancelVisit(w http.ResponseWriter, r *http.Request) {
	id, ok := s.authorizeVisit(w, r)
	if !ok {
		return
	}
	if err := s.store.CancelVisit(r.Context(), s.now(), id); err != nil {
		s.writeBookingError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type outcomeRequest struct {
	Outcome string `json:"outcome"` // "visited" or "no_show"
}

// staffVisitOutcome records whether the buyer showed up.
func (s *Server) staffVisitOutcome(w http.ResponseWriter, r *http.Request) {
	id, ok := s.authorizeVisit(w, r)
	if !ok {
		return
	}
	var req outcomeRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Outcome != models.VisitVisited && req.Outcome != models.VisitNoShow {
		writeError(w, http.StatusBadRequest, "Outcome must be visited or no_show.")
		return
	}
	if err := s.store.MarkOutcome(r.Context(), s.now(), id, req.Outcome); err != nil {
		s.writeBookingError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// authorizeVisit checks the {id} in the URL and that the signed-in user may act
// on that visit: the head always, an executive only for their own slots.
func (s *Server) authorizeVisit(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := chi.URLParam(r, "id")
	if !reUUID.MatchString(id) {
		writeError(w, http.StatusNotFound, "We could not find that visit.")
		return "", false
	}
	user, _ := auth.UserFromContext(r.Context())
	if user.Role == models.RoleHead {
		return id, true
	}
	owner, err := s.store.VisitExecutive(r.Context(), id)
	if err != nil {
		s.writeBookingError(w, err)
		return "", false
	}
	if owner != user.ID {
		writeError(w, http.StatusForbidden, "This visit belongs to another executive.")
		return "", false
	}
	return id, true
}

// ---------------------------------------------------------------------------
// Head-only: manage the slots buyers can pick
// ---------------------------------------------------------------------------

type createSlotRequest struct {
	ProjectID   string `json:"projectId"`
	ExecutiveID string `json:"executiveId"`
	StartsAt    string `json:"startsAt"` // RFC 3339, e.g. 2026-11-05T11:00:00+05:30
	EndsAt      string `json:"endsAt"`
}

func (s *Server) createSlot(w http.ResponseWriter, r *http.Request) {
	var req createSlotRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	start, errS := time.Parse(time.RFC3339, req.StartsAt)
	end, errE := time.Parse(time.RFC3339, req.EndsAt)
	if !reProject.MatchString(req.ProjectID) || !reUUID.MatchString(req.ExecutiveID) || errS != nil || errE != nil {
		writeError(w, http.StatusBadRequest, "Choose a project, an executive, and valid start and end times.")
		return
	}
	if err := booking.ValidateSlotTimes(s.now(), start, end); err != nil {
		s.writeBookingError(w, err)
		return
	}
	slot, err := s.store.CreateSlot(r.Context(), req.ProjectID, req.ExecutiveID, start, end)
	if err != nil {
		s.writeBookingError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, slot)
}

func (s *Server) deleteSlot(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !reUUID.MatchString(id) {
		writeError(w, http.StatusNotFound, "We could not find that slot.")
		return
	}
	if err := s.store.CancelSlot(r.Context(), id); err != nil {
		s.writeBookingError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------------------------
// Shared helpers
// ---------------------------------------------------------------------------

// writeBookingError turns an engine error into a clear message and status.
// Anything unexpected is logged in full but shown to the user only as a
// generic message, so internal details never leak.
func (s *Server) writeBookingError(w http.ResponseWriter, err error) {
	type reply struct {
		status int
		msg    string
	}
	known := []struct {
		err error
		reply
	}{
		{booking.ErrSlotUnavailable, reply{http.StatusConflict, "That time was just taken. Please choose another slot."}},
		{booking.ErrLeadAlreadyBooked, reply{http.StatusConflict, "A visit is already booked for this person."}},
		{booking.ErrVisitNotChangeable, reply{http.StatusConflict, "This visit can no longer be changed."}},
		{booking.ErrVisitNotMarkable, reply{http.StatusConflict, "This visit cannot be marked yet."}},
		{booking.ErrSlotOverlap, reply{http.StatusConflict, "That executive already has a slot at this time."}},
		{booking.ErrSlotHasBooking, reply{http.StatusConflict, "This slot has a booking. Cancel or move the visit first."}},
		{booking.ErrSameSlot, reply{http.StatusBadRequest, "That is already the booked time."}},
		{booking.ErrInvalidSlotTimes, reply{http.StatusBadRequest, "Slots must be in the future and between 15 minutes and 4 hours long."}},
		{booking.ErrLeadNotFound, reply{http.StatusNotFound, "We could not find that lead."}},
		{booking.ErrVisitNotFound, reply{http.StatusNotFound, "We could not find that visit."}},
		{booking.ErrSlotNotFound, reply{http.StatusNotFound, "We could not find that slot."}},
		{booking.ErrExecutiveNotFound, reply{http.StatusBadRequest, "That executive or project does not exist."}},
	}
	for _, k := range known {
		if errors.Is(err, k.err) {
			writeError(w, k.status, k.msg)
			return
		}
	}
	s.serverError(w, "booking", err)
}

func (s *Server) serverError(w http.ResponseWriter, what string, err error) {
	slog.Error("request failed", "what", what, "error", err)
	writeError(w, http.StatusInternalServerError, "Something went wrong. Please try again.")
}

// startOfDay returns midnight (Ahmedabad time) of the day containing t.
func startOfDay(t time.Time) time.Time {
	t = t.In(booking.IST)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, booking.IST)
}
