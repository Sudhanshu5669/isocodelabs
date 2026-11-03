package assistant

import (
	"regexp"
	"strings"
	"unicode"
)

// =============================================================================
// GUARDRAILS
//
// The system prompt asks the model to behave, but a prompt is only a request.
// These checks run on every reply in plain Go, AFTER the model writes it and
// BEFORE the buyer sees it:
//
//   - Money: any amount in the reply (₹, Rs, lakh, crore, %, in English,
//     Hindi or Gujarati, with any digits) must appear in the fact sheet. The
//     fact sheet's indicative range is allowed; any other figure is blocked.
//   - Discounts and guarantees: words like "discount", "cashback", "assured
//     returns" are blocked outright, whatever the model meant.
//
// A blocked reply is replaced by a fixed, safe message and the chat is flagged
// for the sales head's daily review, with the original text kept for them.
//
// CheckInput spots obvious attempts to make the model ignore its rules. The
// model still answers (the output checks protect the buyer), but the chat is
// flagged so a person sees it.
// =============================================================================

// Reasons a reply was blocked.
const (
	BlockMoney     = "mentions a price or amount that is not on the fact sheet"
	BlockDiscount  = "mentions a discount, offer or negotiation"
	BlockGuarantee = "promises a guarantee or assured return"
)

// Amounts: a currency sign followed by a number, or a number followed by a unit.
// \p{N} matches digits in any script, so "६५ लाख" is caught as well as "65 lakh".
var (
	reCurrencyAmount = regexp.MustCompile(`(?i)(₹|\brs\.?|\binr\b|रु\.?|रुपये|રૂ\.?|રૂપિયા)\s*([\p{N}][\p{N},.]*)`)
	reUnitAmount     = regexp.MustCompile(`(?i)([\p{N}][\p{N},.]*)\s*(lakhs?|lacs?|crores?|cr\b|l\b|लाख|करोड़|करोड|લાખ|કરોડ|%|percent|प्रतिशत|ટકા)`)
)

// Phrases that are never allowed, in the three languages the assistant speaks.
var (
	discountWords = []string{
		"discount", "cashback", "cash back", "special price", "best price", "lowest price",
		"negotiable", "negotiate", "price drop", "free gift", "waive", "waiver", "% off", "percent off",
		"डिस्काउंट", "छूट", "कैशबैक",
		"ડિસ્કાઉન્ટ", "છૂટ", "કેશબેક",
	}
	guaranteeWords = []string{
		"guarantee", "assured return", "assured rental", "fixed return",
		"गारंटी", "गारण्टी", "सुनिश्चित रिटर्न",
		"ગેરંટી", "ગેરન્ટી",
	}
)

// CheckReply reports whether a reply may be shown. When it may not, reason
// says why (one of the Block* constants). facts is the fact sheet text.
func CheckReply(reply, facts string) (ok bool, reason string) {
	lower := strings.ToLower(reply)
	for _, w := range discountWords {
		if strings.Contains(lower, w) {
			return false, BlockDiscount
		}
	}
	for _, w := range guaranteeWords {
		if strings.Contains(lower, w) {
			return false, BlockGuarantee
		}
	}

	allowed := amounts(facts)
	for a := range amounts(reply) {
		if !allowed[a] {
			return false, BlockMoney
		}
	}
	return true, ""
}

// amounts finds every money-like figure in text and returns them in one
// normal form ("65 lakh", "1.2 crore", "rs 5000", "8 %"), so the same amount
// written different ways compares equal.
func amounts(text string) map[string]bool {
	found := map[string]bool{}
	for _, m := range reUnitAmount.FindAllStringSubmatch(text, -1) {
		found[normNumber(m[1])+" "+normUnit(m[2])] = true
	}
	for _, idx := range reCurrencyAmount.FindAllStringSubmatchIndex(text, -1) {
		// "₹65 lakh" is already counted above as "65 lakh". Only count the
		// currency form when no unit follows the number, e.g. "₹65,00,000".
		num := text[idx[4]:idx[5]]
		if loc := reUnitAmount.FindStringIndex(text[idx[4]:]); loc != nil && loc[0] == 0 {
			continue
		}
		found["rs "+normNumber(num)] = true
	}
	return found
}

// normNumber turns digits of any script into ASCII and drops grouping commas
// and a trailing full stop: "६५" -> "65", "65,00,000" -> "6500000".
func normNumber(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == ',':
			continue
		case unicode.IsDigit(r):
			b.WriteRune('0' + digitValue(r))
		default:
			b.WriteRune(r)
		}
	}
	return strings.TrimRight(b.String(), ".")
}

// digitValue returns 0-9 for a decimal digit in any script.
func digitValue(r rune) rune {
	// Unicode keeps each script's digits 0-9 in a run of ten code points.
	for _, zero := range []rune{'0', '०', '૦'} {
		if r >= zero && r <= zero+9 {
			return r - zero
		}
	}
	return 0
}

func normUnit(u string) string {
	switch strings.ToLower(u) {
	case "lakh", "lakhs", "lac", "lacs", "l", "लाख", "લાખ":
		return "lakh"
	case "crore", "crores", "cr", "करोड़", "करोड", "કરોડ":
		return "crore"
	default:
		return "%"
	}
}

// Phrases that try to make the model drop its rules. Flagging is cheap, so
// the list leans towards catching more.
var injectionPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)ignore (all |any |the )?(previous|prior|above|earlier|your) (instructions|rules|prompt)`),
	regexp.MustCompile(`(?i)(system|developer) (prompt|message|instructions)`),
	regexp.MustCompile(`(?i)\byou are now\b|\bact as\b|\bpretend (to be|you)\b|\bjailbreak\b|\bdan mode\b|\bdeveloper mode\b`),
	regexp.MustCompile(`(?i)(reveal|show|print|repeat) (me )?(your|the) (instructions|prompt|rules)`),
	regexp.MustCompile(`(?i)disregard (the|your|all) (rules|instructions)`),
}

// CheckInput reports whether a buyer's message looks like an attempt to
// override the assistant's rules.
func CheckInput(text string) (suspicious bool) {
	for _, re := range injectionPatterns {
		if re.MatchString(text) {
			return true
		}
	}
	return false
}
