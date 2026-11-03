package httpapi

import (
	"errors"
	"net/http"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"

	"leadflow/internal/auth"
	"leadflow/internal/models"
	"leadflow/internal/store"
)

// reTopic is a fact-sheet topic: a short machine name such as "possession".
var reTopic = regexp.MustCompile(`^[a-z][a-z0-9_]{1,39}$`)

const (
	maxFactLabel = 80
	maxFactValue = 1000
)

// publicFact is a fact-sheet entry as buyers see it (no editor details).
type publicFact struct {
	Topic string `json:"topic"`
	Label string `json:"label"`
	Value string `json:"value"`
}

type projectWithFacts struct {
	models.Project
	Facts []publicFact `json:"facts"`
}

// listProjects returns every project (public: buyers pick one before chatting).
func (s *Server) listProjects(w http.ResponseWriter, r *http.Request) {
	projects, err := s.store.Projects(r.Context())
	if err != nil {
		s.serverError(w, "list projects", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"projects": projects})
}

// getProject returns one project with its fact sheet (public: the same facts
// the assistant answers from, so buyers can read them directly too).
func (s *Server) getProject(w http.ResponseWriter, r *http.Request) {
	p, ok := s.projectFromURL(w, r)
	if !ok {
		return
	}
	entries, err := s.store.ProjectFacts(r.Context(), p.ID)
	if err != nil {
		s.serverError(w, "project facts", err)
		return
	}
	out := projectWithFacts{Project: p, Facts: make([]publicFact, 0, len(entries))}
	for _, e := range entries {
		out.Facts = append(out.Facts, publicFact{Topic: e.Topic, Label: e.Label, Value: e.Value})
	}
	writeJSON(w, http.StatusOK, out)
}

// staffFacts returns the full fact sheet for the editor (who changed what, when).
func (s *Server) staffFacts(w http.ResponseWriter, r *http.Request) {
	p, ok := s.projectFromURL(w, r)
	if !ok {
		return
	}
	entries, err := s.store.ProjectFacts(r.Context(), p.ID)
	if err != nil {
		s.serverError(w, "project facts", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"project": p, "facts": entries})
}

type factRequest struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// saveFact adds or changes one fact (head only). PUT is used because saving
// the same fact twice has the same result as saving it once.
func (s *Server) saveFact(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.UserFromContext(r.Context())
	p, ok := s.projectFromURL(w, r)
	if !ok {
		return
	}
	topic := chi.URLParam(r, "topic")
	if !reTopic.MatchString(topic) {
		writeError(w, http.StatusBadRequest, "A topic is 2 to 40 lowercase letters, digits or underscores, starting with a letter.")
		return
	}
	var req factRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	req.Label, req.Value = strings.TrimSpace(req.Label), strings.TrimSpace(req.Value)
	switch {
	case req.Label == "" || utf8.RuneCountInString(req.Label) > maxFactLabel:
		writeError(w, http.StatusBadRequest, "Give the fact a title of up to 80 characters.")
		return
	case req.Value == "" || utf8.RuneCountInString(req.Value) > maxFactValue:
		writeError(w, http.StatusBadRequest, "The fact must be 1 to 1,000 characters.")
		return
	}
	fact, err := s.store.SaveFact(r.Context(), p.ID, topic, req.Label, req.Value, user.ID)
	if err != nil {
		s.serverError(w, "save fact", err)
		return
	}
	writeJSON(w, http.StatusOK, fact)
}

// deleteFact removes a fact (head only). The assistant then hands questions on
// that topic to a person instead of answering.
func (s *Server) deleteFact(w http.ResponseWriter, r *http.Request) {
	p, ok := s.projectFromURL(w, r)
	if !ok {
		return
	}
	topic := chi.URLParam(r, "topic")
	if !reTopic.MatchString(topic) {
		writeError(w, http.StatusNotFound, "We could not find that fact.")
		return
	}
	err := s.store.DeleteFact(r.Context(), p.ID, topic)
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "We could not find that fact.")
	case err != nil:
		s.serverError(w, "delete fact", err)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Server) projectFromURL(w http.ResponseWriter, r *http.Request) (models.Project, bool) {
	id := chi.URLParam(r, "id")
	if !reProject.MatchString(id) {
		writeError(w, http.StatusNotFound, "We could not find that project.")
		return models.Project{}, false
	}
	p, err := s.store.Project(r.Context(), id)
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "We could not find that project.")
		return models.Project{}, false
	case err != nil:
		s.serverError(w, "project", err)
		return models.Project{}, false
	}
	return p, true
}

// reUnitCode is a flat's number, e.g. "A-904".
var reUnitCode = regexp.MustCompile(`^[A-Z]{1,3}-[0-9]{1,5}$`)

// listUnits returns every flat for sale (public: the catalogue).
// GET /api/units
func (s *Server) listUnits(w http.ResponseWriter, r *http.Request) {
	units, err := s.store.AvailableUnits(r.Context())
	if err != nil {
		s.serverError(w, "list units", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"units": units})
}

type unitWithFacts struct {
	store.Unit
	Address   string       `json:"address"`
	Latitude  float64      `json:"latitude"`
	Longitude float64      `json:"longitude"`
	Facts     []publicFact `json:"facts"`
}

// getUnit returns one flat with its project's location and fact sheet.
// GET /api/units/{code}
func (s *Server) getUnit(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	if !reUnitCode.MatchString(code) {
		writeError(w, http.StatusNotFound, "We could not find that home.")
		return
	}
	u, err := s.store.UnitByCode(r.Context(), code)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "We could not find that home. It may have been sold.")
		return
	}
	if err != nil {
		s.serverError(w, "unit", err)
		return
	}
	p, err := s.store.Project(r.Context(), u.ProjectID)
	if err != nil {
		s.serverError(w, "unit project", err)
		return
	}
	entries, err := s.store.ProjectFacts(r.Context(), u.ProjectID)
	if err != nil {
		s.serverError(w, "unit facts", err)
		return
	}
	out := unitWithFacts{Unit: u, Address: p.Address, Latitude: p.Latitude, Longitude: p.Longitude,
		Facts: make([]publicFact, 0, len(entries))}
	for _, e := range entries {
		out.Facts = append(out.Facts, publicFact{Topic: e.Topic, Label: e.Label, Value: e.Value})
	}
	writeJSON(w, http.StatusOK, out)
}
