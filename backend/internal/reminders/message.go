// Package reminders sends the site-visit reminders (1 day and 2 hours before)
// and handles opt-outs.
//
//	message.go    the reminder text in English, Hindi and Gujarati, and STOP detection (pure)
//	decide.go     whether a due reminder may be sent at all (pure)
//	messenger.go  the Messenger interface and the mock used in this build
//	worker.go     the background loop that sends due reminders
//
// Compliance rules this package enforces:
//   - RERA: every message names the project's RERA registration number.
//   - DPDP Act 2023: nothing is sent without consent or after an opt-out, and
//     the check is repeated at send time, not only when the visit was booked.
//   - Every message tells the buyer how to stop messages (reply STOP).
package reminders

import (
	"strings"
	"time"
	"unicode"

	"leadflow/internal/booking"
	"leadflow/internal/models"
)

// Visit is what a reminder message talks about.
type Visit struct {
	Kind          string // models.ReminderDayBefore or models.ReminderTwoHoursBefore
	Language      string // en, hi or gu
	FirstName     string
	ProjectName   string
	ReraNumber    string
	Address       string
	ExecutiveName string
	StartsAt      time.Time
	Link          string // the buyer's visit page; may be empty
}

type texts struct {
	tomorrow, inTwoHours, body, link string
}

// {name} {project} {when} {at} {exec} {address} {link} {rera} are filled in.
var messages = map[string]texts{
	models.LangEnglish: {
		tomorrow:   "tomorrow",
		inTwoHours: "in 2 hours",
		body: "Hi {name}, a reminder: your site visit to {project} is {when}, at {at}, with {exec}. " +
			"Address: {address}.{link} RERA registration no. {rera}. Reply STOP to stop these messages.",
		link: " View, reschedule or cancel: {url}.",
	},
	models.LangHindi: {
		tomorrow:   "कल",
		inTwoHours: "2 घंटे में",
		body: "नमस्ते {name}, याद दिला दें: {project} पर आपकी साइट विज़िट {when} है, {at} बजे, {exec} के साथ। " +
			"पता: {address}।{link} RERA पंजीकरण संख्या {rera}। ये संदेश बंद करने के लिए STOP लिखें।",
		link: " विज़िट देखें, बदलें या रद्द करें: {url}।",
	},
	models.LangGujarati: {
		tomorrow:   "આવતીકાલે",
		inTwoHours: "2 કલાકમાં",
		body: "નમસ્તે {name}, યાદ અપાવીએ: {project} પર તમારી સાઇટ વિઝિટ {when} છે, {at} વાગ્યે, {exec} સાથે. " +
			"સરનામું: {address}.{link} RERA નોંધણી નં. {rera}. આ સંદેશા બંધ કરવા STOP લખો.",
		link: " વિઝિટ જુઓ, બદલો અથવા રદ કરો: {url}.",
	},
}

// Render writes the reminder in the buyer's language (English if unknown).
func Render(v Visit) string {
	t, ok := messages[v.Language]
	if !ok {
		t = messages[models.LangEnglish]
	}
	when := t.tomorrow
	if v.Kind == models.ReminderTwoHoursBefore {
		when = t.inTwoHours
	}
	link := ""
	if v.Link != "" {
		link = strings.ReplaceAll(t.link, "{url}", v.Link)
	}
	name := v.FirstName
	if name == "" {
		name = "there"
	}
	return strings.NewReplacer(
		"{name}", name,
		"{project}", v.ProjectName,
		"{when}", when,
		"{at}", v.StartsAt.In(booking.IST).Format("Mon 2 Jan, 3:04 PM"),
		"{exec}", v.ExecutiveName,
		"{address}", v.Address,
		"{link}", link,
		"{rera}", v.ReraNumber,
	).Replace(t.body)
}

// stopWords are the replies that mean "stop messaging me". Only the whole
// message counts, so "please don't stop the visit" is not an opt-out.
var stopWords = map[string]bool{
	"STOP": true, "STOP ALL": true, "UNSUBSCRIBE": true, "OPT OUT": true, "OPTOUT": true, "END": true,
	"बंद": true, "बंद करो": true, "रोकें": true, // Hindi
	"બંધ": true, "બંધ કરો": true, "રોકો": true, // Gujarati
}

// IsStop reports whether a buyer's reply asks to stop all messages.
func IsStop(text string) bool {
	// Upper-case, drop punctuation around the words, squeeze spaces.
	cleaned := strings.Map(func(r rune) rune {
		if unicode.IsPunct(r) {
			return -1
		}
		return r
	}, strings.ToUpper(text))
	return stopWords[strings.Join(strings.Fields(cleaned), " ")]
}
