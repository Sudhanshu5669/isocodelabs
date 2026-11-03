package leads

import "strings"

// NormalizePhone turns the many ways people write an Indian mobile number into
// one canonical form, "+91" followed by ten digits. That form is the key for the
// exact-duplicate check, so "98250 12345", "+91-98250-12345" and "09825012345"
// all become "+919825012345".
//
// ASSUMPTION: only Indian mobile numbers (ten digits starting 6 to 9) are
// accepted. The brief describes an Ahmedabad market; overseas numbers are a
// later extension.
func NormalizePhone(raw string) (string, error) {
	var digits strings.Builder
	for i, r := range strings.TrimSpace(raw) {
		switch {
		case r >= '0' && r <= '9':
			digits.WriteRune(r)
		case r == '+' && i == 0: // a plus is only meaningful at the very start
		case r == ' ' || r == '-' || r == '(' || r == ')' || r == '.':
		default:
			return "", errBadPhone
		}
	}

	d := digits.String()
	d = strings.TrimPrefix(d, "00") // international dialling prefix: 0091...
	switch {
	case len(d) == 12 && strings.HasPrefix(d, "91"): // 91 98250 12345
		d = d[2:]
	case len(d) == 11 && strings.HasPrefix(d, "0"): // trunk prefix: 098250 12345
		d = d[1:]
	}

	if len(d) != 10 || d[0] < '6' || d[0] > '9' {
		return "", errBadPhone
	}
	return "+91" + d, nil
}

var errBadPhone = invalid("phone", "Enter a valid 10-digit Indian mobile number.")
