package pii

import (
	"sort"
	"strings"
	"unicode"
)

// Blind-index helpers. Each one normalises its input the same way every time,
// because a blind index only matches an EXACT input: "Rohan@X.test" and
// "rohan@x.test" must give the same index or sign-in would fail.

// EmailIndex is the blind index of an email address (case and outer spaces ignored).
func EmailIndex(email string) []byte {
	return Index(Email, strings.ToLower(strings.TrimSpace(email)))
}

// PhoneIndex is the blind index of a phone number. The number must already be
// normalised (leads.NormalizePhone), so "+91 98…" and "098…" match.
func PhoneIndex(normalizedPhone string) []byte {
	return Index(Phone, normalizedPhone)
}

// NameWords splits a name into lower-case words: letters, digits and the
// combining marks that Hindi and Gujarati vowel signs use. Repeated words
// appear once.
func NameWords(name string) []string {
	split := func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) && !unicode.IsMark(r) }
	seen := map[string]bool{}
	words := []string{}
	for _, w := range strings.FieldsFunc(strings.ToLower(name), split) {
		if !seen[w] {
			seen[w] = true
			words = append(words, w)
		}
	}
	sort.Strings(words)
	return words
}

// NameWordIndexes returns one blind index per word of a name. Stored as a
// bytea[] column, it lets staff search leads by any whole word of the name
// ("aarav" or "shah" finds "Aarav Shah") without the name being readable.
// It is never nil, so it can fill a NOT NULL column.
func NameWordIndexes(name string) [][]byte {
	words := NameWords(name)
	out := make([][]byte, len(words))
	for i, w := range words {
		out[i] = Index(NameWord, w)
	}
	return out
}
