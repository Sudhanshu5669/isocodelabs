package leads

import (
	"sort"
	"strings"
	"unicode"
)

// NamesSimilar reports whether two names plausibly belong to the same person.
// It is the "fuzzy" half of the duplicate check: a hit never merges anything,
// it only flags the lead so a human can confirm.
//
// A pair is similar when, ignoring case, punctuation and word order:
//   - the names are identical ("Patel Hardik" and "hardik patel"), or
//   - the first and last words match ("Hardik Kumar Patel" and "Hardik Patel"), or
//   - they differ by at most about one letter in seven ("Hardik Patel" and
//     "Hardik Patil"), which covers typing and spelling variations.
func NamesSimilar(a, b string) bool {
	ta, tb := tokens(a), tokens(b)
	if len(ta) == 0 || len(tb) == 0 {
		return false
	}

	if len(ta) >= 2 && len(tb) >= 2 &&
		ta[0] == tb[0] && ta[len(ta)-1] == tb[len(tb)-1] {
		return true
	}

	sa, sb := sortedJoin(ta), sortedJoin(tb)
	if sa == sb {
		return true
	}

	longest := max(len([]rune(sa)), len([]rune(sb)))
	if longest < 5 { // too short to judge by edit distance
		return false
	}
	return editDistance([]rune(sa), []rune(sb))*100/longest <= 15
}

// tokens lower-cases a name and splits it into words, dropping punctuation.
// Combining marks are kept, because in Hindi and Gujarati the vowel signs are
// marks and splitting on them would break words apart.
func tokens(s string) []string {
	return strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsMark(r) && !unicode.IsDigit(r)
	})
}

func sortedJoin(words []string) string {
	c := append([]string(nil), words...)
	sort.Strings(c)
	return strings.Join(c, " ")
}

// editDistance is the number of single-letter inserts, deletes or changes that
// turn a into b (the Levenshtein distance), computed one row at a time.
func editDistance(a, b []rune) int {
	prev := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur := make([]int, len(b)+1)
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[len(b)]
}
