package leads_test

import (
	"errors"
	"strings"
	"testing"

	"leadflow/internal/leads"
)

func validInput() leads.Input {
	return leads.Input{Name: "Hardik Patel", Phone: "98250 12345", Source: "website"}
}

func TestCleanAcceptsAndNormalises(t *testing.T) {
	in := validInput()
	in.Name = "  Hardik    Patel "
	in.Wants = "3 BHK"
	in.ProjectID = "project-1"
	in.Note = "  asked   about parking "

	got, err := in.Clean()
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Hardik Patel" || got.Phone != "+919825012345" || got.Note != "asked about parking" {
		t.Errorf("not cleaned: %+v", got)
	}
	if got.Language != "en" {
		t.Errorf("Language = %q, want the default en", got.Language)
	}
}

func TestCleanRejects(t *testing.T) {
	tests := []struct {
		name  string
		edit  func(*leads.Input)
		field string
	}{
		{"short name", func(i *leads.Input) { i.Name = "A" }, "name"},
		{"blank name", func(i *leads.Input) { i.Name = "   " }, "name"},
		{"long name", func(i *leads.Input) { i.Name = strings.Repeat("a", 81) }, "name"},
		{"control character in name", func(i *leads.Input) { i.Name = "Hardik\x00Patel" }, "name"},
		{"bad phone", func(i *leads.Input) { i.Phone = "12345" }, "phone"},
		{"unknown source", func(i *leads.Input) { i.Source = "newspaper" }, "source"},
		{"empty source", func(i *leads.Input) { i.Source = "" }, "source"},
		{"bad project id", func(i *leads.Input) { i.ProjectID = "x'; DROP" }, "projectId"},
		{"bad wants", func(i *leads.Input) { i.Wants = "4 BHK" }, "wants"},
		{"bad language", func(i *leads.Input) { i.Language = "fr" }, "language"},
		{"long note", func(i *leads.Input) { i.Note = strings.Repeat("n", 201) }, "note"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			in := validInput()
			tc.edit(&in)
			_, err := in.Clean()

			var ve *leads.ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("err = %v, want a *ValidationError", err)
			}
			if ve.Field != tc.field {
				t.Errorf("Field = %q, want %q", ve.Field, tc.field)
			}
		})
	}
}
