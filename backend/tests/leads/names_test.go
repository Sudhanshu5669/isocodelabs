package leads_test

import (
	"testing"

	"leadflow/internal/leads"
)

func TestNamesSimilar(t *testing.T) {
	tests := []struct {
		a, b string
		want bool
	}{
		{"Hardik Patel", "Hardik Patel", true},
		{"Hardik Patel", "  hardik   PATEL ", true},
		{"Hardik Patel", "Patel Hardik", true},       // word order
		{"Hardik Patel", "Hardik Patil", true},       // one-letter spelling variation
		{"Hardik Kumar Patel", "Hardik Patel", true}, // middle name
		{"Hardik K. Patel", "Hardik Patel", true},    // initial and punctuation
		{"Nidhi Shah", "Nidhi Shan", true},           // typo
		{"હાર્દિક પટેલ", "હાર્દિક પટેલ", true},       // Gujarati, identical
		{"हार्दिक पटेल", "हार्दिक  पटेल", true},      // Hindi, extra space
		{"Raj Shah", "Ravi Shah", false},             // different first name
		{"Hardik Patel", "Kavita Joshi", false},      // unrelated
		{"Amit", "Amir", false},                      // too short to judge by distance
		{"", "Hardik Patel", false},
		{"Hardik Patel", "", false},
		{"!!!", "???", false}, // nothing but punctuation
	}
	for _, tc := range tests {
		t.Run(tc.a+" | "+tc.b, func(t *testing.T) {
			if got := leads.NamesSimilar(tc.a, tc.b); got != tc.want {
				t.Fatalf("NamesSimilar(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
			}
			if got := leads.NamesSimilar(tc.b, tc.a); got != tc.want {
				t.Fatalf("NamesSimilar is not symmetric for %q, %q", tc.a, tc.b)
			}
		})
	}
}

func TestEditDistance(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"", "", 0},
		{"abc", "abc", 0},
		{"abc", "", 3},
		{"kitten", "sitting", 3},
		{"patel", "patil", 1},
	}
	for _, tc := range tests {
		if got := leads.EditDistance([]rune(tc.a), []rune(tc.b)); got != tc.want {
			t.Errorf("EditDistance(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}
