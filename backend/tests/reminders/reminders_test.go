package reminders_test

import (
	"strings"
	"testing"
	"time"

	"leadflow/internal/models"
	"leadflow/internal/reminders"
)

func visit(lang, kind string) reminders.Visit {
	return reminders.Visit{
		Kind: kind, Language: lang, FirstName: "Asha", ProjectName: "Project 1", ReraNumber: "PR/GJ/AHMEDABAD/123",
		Address: "SG Highway, Ahmedabad", ExecutiveName: "Rohan Mehta",
		StartsAt: time.Date(2026, 11, 7, 5, 30, 0, 0, time.UTC), // 11:00 IST
	}
}

// RERA: every reminder must carry the project's registration number, and
// DPDP: every reminder must say how to stop messages. In every language.
func TestEveryReminderHasReraNumberAndStopInstruction(t *testing.T) {
	for _, lang := range []string{models.LangEnglish, models.LangHindi, models.LangGujarati, "xx"} {
		for _, kind := range []string{models.ReminderDayBefore, models.ReminderTwoHoursBefore} {
			body := reminders.Render(visit(lang, kind))
			for _, want := range []string{"PR/GJ/AHMEDABAD/123", "STOP", "Project 1", "Rohan Mehta", "Sat 7 Nov, 11:00 AM", "SG Highway"} {
				if !strings.Contains(body, want) {
					t.Errorf("%s/%s: missing %q in %q", lang, kind, want, body)
				}
			}
		}
	}
}

func TestReminderWordingAndLink(t *testing.T) {
	dayBefore := reminders.Render(visit("en", models.ReminderDayBefore))
	twoHours := reminders.Render(visit("en", models.ReminderTwoHoursBefore))
	if !strings.Contains(dayBefore, "tomorrow") || !strings.Contains(twoHours, "in 2 hours") {
		t.Errorf("wrong timing words:\n%s\n%s", dayBefore, twoHours)
	}
	if strings.Contains(dayBefore, "http") {
		t.Error("no link should appear when PUBLIC_URL is not set")
	}
	v := visit("hi", models.ReminderDayBefore)
	v.Link = "https://visits.example.com/visit/abc"
	if body := reminders.Render(v); !strings.Contains(body, v.Link) || !strings.Contains(body, "कल") {
		t.Errorf("Hindi reminder should say कल and include the link: %s", body)
	}
}

func TestIsStop(t *testing.T) {
	for _, yes := range []string{"STOP", "stop", " Stop! ", "unsubscribe", "Opt out", "बंद", "બંધ કરો"} {
		if !reminders.IsStop(yes) {
			t.Errorf("%q should opt out", yes)
		}
	}
	for _, no := range []string{"", "please don't stop my visit", "stopping by at 5", "cancel", "ok"} {
		if reminders.IsStop(no) {
			t.Errorf("%q must not opt out", no)
		}
	}
}

func TestDecideRechecksEverythingAtSendTime(t *testing.T) {
	now := time.Date(2026, 11, 6, 6, 0, 0, 0, time.UTC)
	start := now.Add(23 * time.Hour)
	cases := []struct {
		name       string
		kind       string
		status     string
		consent    bool
		optedOut   bool
		start      time.Time
		wantReason string
	}{
		{"all good", models.ReminderDayBefore, models.VisitBooked, true, false, start, ""},
		{"cancelled since booking", models.ReminderDayBefore, models.VisitCancelled, true, false, start, reminders.SkipVisitNotBooked},
		{"no consent", models.ReminderDayBefore, models.VisitBooked, false, false, start, reminders.SkipOptedOut},
		{"opted out since booking", models.ReminderTwoHoursBefore, models.VisitBooked, true, true, now.Add(time.Hour), reminders.SkipOptedOut},
		{"visit already started", models.ReminderTwoHoursBefore, models.VisitBooked, true, false, now.Add(-time.Minute), reminders.SkipTooLate},
		{"day-before reminder hours late", models.ReminderDayBefore, models.VisitBooked, true, false, now.Add(90 * time.Minute), reminders.SkipTooLate},
		{"two-hour reminder a bit late", models.ReminderTwoHoursBefore, models.VisitBooked, true, false, now.Add(90 * time.Minute), ""},
	}
	for _, c := range cases {
		if got := reminders.Decide(now, c.kind, c.status, c.consent, c.optedOut, c.start); got != c.wantReason {
			t.Errorf("%s: got %q, want %q", c.name, got, c.wantReason)
		}
	}
}

func TestMaskPhone(t *testing.T) {
	if got := reminders.MaskPhone("+919876543210"); got != "*********3210" {
		t.Errorf("got %s", got)
	}
}
