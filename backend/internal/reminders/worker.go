package reminders

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"leadflow/internal/store"
)

// Worker sends due reminders in the background.
//
// Go lesson: a "goroutine" is a function running at the same time as the rest
// of the program. main starts Run in one with `go worker.Run(ctx)`; when ctx is
// cancelled (the server is shutting down) Run returns.
type Worker struct {
	Store     *store.Store
	Messenger Messenger
	PublicURL string           // for the visit link in messages; may be empty
	Every     time.Duration    // how often to look for due reminders (default 30 s)
	Now       func() time.Time // the clock; tests replace it
}

// batch caps how many reminders one pass sends, so a backlog after downtime
// is worked through steadily instead of all at once.
const batch = 50

// Run checks for due reminders every w.Every until ctx is cancelled.
func (w *Worker) Run(ctx context.Context) {
	every := w.Every
	if every <= 0 {
		every = 30 * time.Second
	}
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		if n, err := w.RunOnce(ctx); err != nil && ctx.Err() == nil {
			slog.Error("reminders: pass failed", "error", err)
		} else if n > 0 {
			slog.Info("reminders: pass done", "processed", n)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// RunOnce processes every reminder that is due now (up to one batch) and
// reports how many it handled.
func (w *Worker) RunOnce(ctx context.Context) (int, error) {
	now := time.Now()
	if w.Now != nil {
		now = w.Now()
	}
	return w.Store.ProcessDueReminders(ctx, now, batch, func(ctx context.Context, d store.DueReminder) store.ReminderResult {
		return w.handle(ctx, now, d)
	})
}

func (w *Worker) handle(ctx context.Context, now time.Time, d store.DueReminder) store.ReminderResult {
	if reason := Decide(now, d.Kind, d.VisitStatus, d.Consent, d.OptedOut, d.StartsAt); reason != "" {
		return store.ReminderResult{Status: "skipped", SkipReason: reason}
	}
	link := ""
	if w.PublicURL != "" {
		link = w.PublicURL + "/visit/" + d.VisitCode
	}
	body := Render(Visit{
		Kind: d.Kind, Language: d.Language, FirstName: firstWord(d.LeadName),
		ProjectName: d.ProjectName, ReraNumber: d.ReraNumber, Address: d.Address,
		ExecutiveName: d.ExecutiveName, StartsAt: d.StartsAt, Link: link,
	})
	if err := w.Messenger.Send(ctx, d.Phone, body); err != nil {
		slog.Error("reminders: send failed", "reminder", d.ID, "error", err)
		return store.ReminderResult{Status: "failed", SkipReason: FailedSend, Body: body}
	}
	return store.ReminderResult{Status: "sent", Body: body}
}

func firstWord(s string) string {
	if f := strings.Fields(s); len(f) > 0 {
		return f[0]
	}
	return ""
}
