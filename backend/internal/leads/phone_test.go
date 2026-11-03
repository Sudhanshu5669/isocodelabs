package leads

import "testing"

func TestNormalizePhone(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string // empty means "must be rejected"
	}{
		{"plain ten digits", "9825012345", "+919825012345"},
		{"with country code and plus", "+919825012345", "+919825012345"},
		{"country code without plus", "919825012345", "+919825012345"},
		{"spaces", "98250 12345", "+919825012345"},
		{"dashes and brackets", "+91 (98250)-12345", "+919825012345"},
		{"dots", "98250.12345", "+919825012345"},
		{"trunk zero", "09825012345", "+919825012345"},
		{"double zero dialling", "00919825012345", "+919825012345"},
		{"surrounding space", "  9825012345  ", "+919825012345"},
		{"starts with 6", "6000000001", "+916000000001"},

		{"empty", "", ""},
		{"too short", "98250123", ""},
		{"too long", "98250123456", ""},
		{"landline style", "2212345678", ""},
		{"starts with 5", "5825012345", ""},
		{"letters", "98250abcde", ""},
		{"plus in the middle", "98250+12345", ""},
		{"sql-looking input", "9825012345; DROP TABLE leads", ""},
		{"wrong country code", "+14155550123", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NormalizePhone(tc.in)
			if tc.want == "" {
				if err == nil {
					t.Fatalf("NormalizePhone(%q) = %q, want an error", tc.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("NormalizePhone(%q) error: %v", tc.in, err)
			}
			if got != tc.want {
				t.Fatalf("NormalizePhone(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// Every way of writing one number must give the same key.
func TestNormalizePhoneIsStableAcrossFormats(t *testing.T) {
	forms := []string{"9825012345", "+91 98250 12345", "0091 98250 12345", "09825012345"}
	first, _ := NormalizePhone(forms[0])
	for _, f := range forms {
		got, err := NormalizePhone(f)
		if err != nil || got != first {
			t.Errorf("NormalizePhone(%q) = %q, %v; want %q", f, got, err, first)
		}
	}
}
