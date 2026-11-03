package httpapi

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"leadflow/internal/auth"
	"leadflow/internal/models"
	"leadflow/internal/store"
)

// staffVisits lists visits for the diary, e.g.
// GET /api/staff/visits?from=2026-11-03&to=2026-11-09&status=booked&project=project-1
// Defaults to today and the next 6 days. Executives see their own visits only.
func (s *Server) staffVisits(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.UserFromContext(r.Context())
	q := r.URL.Query()
	now := s.now()
	from, to, msg := dayRange(q, startOfDay(now), 7, 62)
	if q.Get("from") == "" && q.Get("to") == "" {
		from, to = startOfDay(now), startOfDay(now).AddDate(0, 0, 7) // the coming week
	}
	if msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	f := store.VisitFilter{From: from, To: to, Status: q.Get("status"), ProjectID: q.Get("project")}
	if f.Status != "" && !validVisitStatus[f.Status] {
		writeError(w, http.StatusBadRequest, "Status must be booked, visited, no_show or cancelled.")
		return
	}
	if f.ProjectID != "" && !reProject.MatchString(f.ProjectID) {
		writeError(w, http.StatusBadRequest, "Choose a valid project.")
		return
	}
	visits, err := s.store.StaffVisits(r.Context(), now, viewer(user), f)
	if err != nil {
		s.serverError(w, "staff visits", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"visits": visits})
}

var validVisitStatus = map[string]bool{
	models.VisitBooked: true, models.VisitVisited: true, models.VisitNoShow: true, models.VisitCancelled: true,
}

// staffSlots lists every slot (open and booked) for the slot manager, e.g.
// GET /api/staff/slots?project=project-1&from=2026-11-03&to=2026-11-16
// Defaults to the next 14 days. Executives see their own slots only.
func (s *Server) staffSlots(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.UserFromContext(r.Context())
	q := r.URL.Query()
	today := startOfDay(s.now())
	from, to, msg := dayRange(q, today, 14, 62)
	if q.Get("from") == "" && q.Get("to") == "" {
		from, to = today, today.AddDate(0, 0, 14)
	}
	if msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	project := q.Get("project")
	if project != "" && !reProject.MatchString(project) {
		writeError(w, http.StatusBadRequest, "Choose a valid project.")
		return
	}
	slots, err := s.store.StaffSlots(r.Context(), viewer(user), project, from, to)
	if err != nil {
		s.serverError(w, "staff slots", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"slots": slots})
}

// reminderLog lists reminders and what happened to each, e.g.
// GET /api/staff/reminders?status=skipped&from=2026-10-27&to=2026-11-09
// Defaults to 7 days back and 7 days ahead. Executives see their own visits only.
func (s *Server) reminderLog(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.UserFromContext(r.Context())
	q := r.URL.Query()
	today := startOfDay(s.now())
	from, to, msg := dayRange(q, today, 7, 92)
	if q.Get("from") == "" && q.Get("to") == "" {
		from, to = today.AddDate(0, 0, -7), today.AddDate(0, 0, 8)
	}
	if msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	status := q.Get("status")
	if status != "" && status != models.ReminderQueued && status != models.ReminderSent &&
		status != models.ReminderSkipped && status != models.ReminderFailed {
		writeError(w, http.StatusBadRequest, "Status must be queued, sent, skipped or failed.")
		return
	}
	log, err := s.store.ReminderLog(r.Context(), viewer(user), store.ReminderFilter{Status: status, From: from, To: to})
	if err != nil {
		s.serverError(w, "reminder log", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"reminders": log})
}

type leadStatusRequest struct {
	Status string `json:"status"` // "lost" or "engaged" (reopen)
}

// setLeadStatus marks a lead lost, or reopens it.
// POST /api/staff/leads/{id}/status {"status": "lost"}
func (s *Server) setLeadStatus(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.UserFromContext(r.Context())
	id := chi.URLParam(r, "id")
	if !reUUID.MatchString(id) {
		writeError(w, http.StatusNotFound, "We could not find that lead.")
		return
	}
	var req leadStatusRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Status != models.LeadLost && req.Status != models.LeadEngaged {
		writeError(w, http.StatusBadRequest, "Status must be lost or engaged. Visit stages are set by booking.")
		return
	}
	err := s.store.SetLeadStatus(r.Context(), viewer(user), id, req.Status)
	if errors.Is(err, store.ErrLeadHasBooking) {
		writeError(w, http.StatusConflict, "This lead has a booked visit. Cancel the visit first.")
		return
	}
	if err != nil {
		s.writeLeadError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// optOutLead records that a buyer asked not to be contacted (e.g. on a call).
// POST /api/staff/leads/{id}/opt-out
func (s *Server) optOutLead(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.UserFromContext(r.Context())
	id := chi.URLParam(r, "id")
	if !reUUID.MatchString(id) {
		writeError(w, http.StatusNotFound, "We could not find that lead.")
		return
	}
	if err := s.store.OptOutLead(r.Context(), viewer(user), id); err != nil {
		s.writeLeadError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
