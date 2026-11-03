// Package leads holds the pure rules for turning an enquiry into a lead:
// cleaning and checking the input, normalising phone numbers, spotting
// look-alike names, and combining two records of the same person.
//
// It has no database code, so every rule here is tested instantly. The SQL that
// applies these rules lives in package store.
//
// File map:
//
//	leads.go   the input shape, its validation, and the errors this package returns
//	phone.go   phone normalisation (the exact-duplicate key)
//	names.go   fuzzy name comparison (the "possible duplicate" signal)
//	merge.go   how two records of one person combine
package leads

import (
	"errors"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"leadflow/internal/models"
)

// Errors callers compare with errors.Is.
var (
	ErrNotFound         = errors.New("leads: lead not found")
	ErrNotDuplicate     = errors.New("leads: lead is not flagged as a possible duplicate")
	ErrMergeConflict    = errors.New("leads: both leads have a booked visit")
	ErrAssigneeNotFound = errors.New("leads: executive not found")
)

// ValidationError means the input was wrong in a way the sender can fix. The
// message is written for people and is safe to show.
//
// Go lesson: a type with an Error() method is an error. Callers use
// errors.As to get the typed value back and read its fields.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string { return e.Message }

func invalid(field, msg string) error { return &ValidationError{Field: field, Message: msg} }

// Input is one enquiry as it arrives, from any source.
type Input struct {
	Name      string
	Phone     string
	Source    string // one of the models.Source* values
	ProjectID string // optional
	Wants     string // optional: "2 BHK" or "3 BHK"
	Language  string // optional: en, hi or gu (defaults to en)
	Consent   bool   // the person agreed to be contacted (DPDP Act)
	Note      string // optional, kept on the touch record
}

const (
	maxNameRunes = 80
	maxNoteRunes = 200
)

var reProject = regexp.MustCompile(`^[a-z0-9-]{1,40}$`)

var validSources = map[string]bool{
	models.SourcePropertyPortal: true,
	models.SourceSocialAd:       true,
	models.SourceWebsite:        true,
	models.SourceHoardingCall:   true,
	models.SourceManual:         true,
}

// ValidSource reports whether s is a lead source we know.
func ValidSource(s string) bool { return validSources[s] }

// ValidStatus reports whether s is a lead stage we know.
func ValidStatus(s string) bool {
	switch s {
	case models.LeadNew, models.LeadEngaged, models.LeadVisitBooked, models.LeadVisited, models.LeadLost:
		return true
	}
	return false
}

// Clean trims and checks an Input and returns a copy that is safe to store:
// the phone is in E.164 form and optional fields have their defaults.
func (in Input) Clean() (Input, error) {
	out := in

	out.Name = collapseSpaces(in.Name)
	if n := utf8.RuneCountInString(out.Name); n < 2 || n > maxNameRunes {
		return Input{}, invalid("name", "Enter a name of 2 to 80 characters.")
	}
	if hasControlChars(out.Name) {
		return Input{}, invalid("name", "The name contains characters that are not allowed.")
	}

	phone, err := NormalizePhone(in.Phone)
	if err != nil {
		return Input{}, err
	}
	out.Phone = phone

	if !ValidSource(in.Source) {
		return Input{}, invalid("source", "Choose a valid lead source.")
	}

	if in.ProjectID != "" && !reProject.MatchString(in.ProjectID) {
		return Input{}, invalid("projectId", "Choose a valid project.")
	}

	switch in.Wants {
	case "", "2 BHK", "3 BHK":
	default:
		return Input{}, invalid("wants", "Choose 2 BHK or 3 BHK.")
	}

	switch in.Language {
	case "":
		out.Language = models.LangEnglish
	case models.LangEnglish, models.LangHindi, models.LangGujarati:
	default:
		return Input{}, invalid("language", "Choose English, Hindi or Gujarati.")
	}

	out.Note = collapseSpaces(in.Note)
	if utf8.RuneCountInString(out.Note) > maxNoteRunes || hasControlChars(out.Note) {
		return Input{}, invalid("note", "The note is too long or contains characters that are not allowed.")
	}
	return out, nil
}

// collapseSpaces trims the ends and turns any run of white space into one space.
func collapseSpaces(s string) string { return strings.Join(strings.Fields(s), " ") }

func hasControlChars(s string) bool {
	for _, r := range s {
		if unicode.IsControl(r) {
			return true
		}
	}
	return false
}
