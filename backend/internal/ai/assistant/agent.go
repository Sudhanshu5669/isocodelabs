// Package assistant is the AI chat assistant: the loop that talks to the
// language model, the tools it may use, the system prompt, the guardrails that
// check every reply, and the fixed messages used when the model cannot be.
//
// It does no I/O of its own. The model is reached through ai.ChatCompletion
// (any provider in internal/ai/providers) and every action through the Actions
// interface (the HTTP layer implements it with the store). So the whole agent
// is tested in milliseconds with a scripted fake model.
//
// File map:
//
//	agent.go    the loop: one buyer message in, one checked reply out
//	tools.go    the five tools, their argument checks, and Actions
//	prompt.go   the system prompt, built from the fact sheet
//	guard.go    guardrails on every reply (money, discounts, guarantees)
//	scripts.go  fixed messages in English, Hindi and Gujarati
//	summary.go  the handoff summary for the salesperson
package assistant

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"leadflow/internal/ai"
)

// MaxSteps caps how many times the model may call tools before it must
// answer. A normal booking takes three (slots, book, answer).
const MaxSteps = 6

// Agent answers buyer messages.
type Agent struct {
	Model ai.ChatCompletion
}

// Input is one buyer message, with the chat it belongs to.
type Input struct {
	Chat    ChatContext
	History []ai.Turn // earlier messages of this chat, oldest first, text only
	Message string    // what the buyer just wrote
}

// Result is the agent's answer plus everything it did along the way.
type Result struct {
	Reply string // what the buyer sees

	Slots         []Slot   // free times found; the UI shows them as cards
	Booking       *Booking // set when a visit was booked
	HandedOff     bool     // a salesperson was asked to call
	ExecutiveName string   // who, when known

	Notes    []string // lines for the staff view of the chat (not shown to the buyer)
	Flags    []string // reasons this chat needs the sales head's review
	Fallback bool     // the model failed and a fixed message was sent
	Blocked  bool     // a guardrail replaced the model's reply
}

// run holds the state of one Reply call.
type run struct {
	actions Actions
	now     time.Time
	offered map[string]bool // slot ids the model was shown in this run
	res     Result
}

// Reply answers one buyer message. It always returns a reply the buyer can
// see: if the model fails, a fixed message is used and a person is asked to
// follow up, so no enquiry is ever left unanswered.
func (a *Agent) Reply(ctx context.Context, in Input, actions Actions) Result {
	r := &run{actions: actions, now: in.Chat.Now, offered: map[string]bool{}}

	if CheckInput(in.Message) {
		r.res.Flags = append(r.res.Flags, "Buyer message looks like an attempt to change the assistant's rules")
	}

	history := append(append([]ai.Turn{}, in.History...), ai.Turn{Role: ai.RoleUser, Parts: []ai.Part{{Text: in.Message}}})
	req := ai.Request{System: SystemPrompt(in.Chat), History: history, Tools: ToolSpecs}

	for step := 0; step < MaxSteps; step++ {
		turn, err := a.Model.Complete(ctx, req)
		if err != nil {
			slog.Warn("assistant: model failed", "error", err)
			return r.fallback(ctx, in.Chat)
		}

		calls := turn.Calls()
		if len(calls) == 0 {
			text := strings.TrimSpace(turn.Text())
			if text == "" {
				return r.fallback(ctx, in.Chat)
			}
			r.res.Reply = r.checked(text, in.Chat)
			return r.res
		}

		// Run every tool the model asked for, then send the results back.
		req.History = append(req.History, turn)
		results := ai.Turn{Role: ai.RoleUser}
		for _, c := range calls {
			results.Parts = append(results.Parts, ai.Part{Result: &ai.ToolResult{
				ID: c.ID, Name: c.Name, Response: r.runTool(ctx, c),
			}})
		}
		req.History = append(req.History, results)
	}

	slog.Warn("assistant: model kept calling tools without answering")
	return r.fallback(ctx, in.Chat)
}

// checked runs the guardrails on a reply and swaps in the safe message if
// needed. The original is kept in the notes for the sales head's review.
func (r *run) checked(text string, chat ChatContext) string {
	facts := FactText(chat.Facts) + chat.ReraNumber
	if ok, reason := CheckReply(text, facts); !ok {
		r.res.Blocked = true
		r.res.Flags = append(r.res.Flags, "Guardrail: reply "+reason)
		r.res.Notes = append(r.res.Notes, "Blocked reply (not shown to the buyer): "+text)
		return PriceSafeReply(chat.Lang)
	}
	return text
}

// fallback is used when the model fails. The buyer still gets an answer,
// a salesperson is asked to call, and free times are offered as cards.
func (r *run) fallback(ctx context.Context, chat ChatContext) Result {
	r.res.Fallback = true
	name := chat.ExecutiveName
	if !chat.HandedOff && !r.res.HandedOff {
		if n, err := r.actions.Handoff(ctx, "The assistant could not answer; please call the buyer."); err == nil {
			name = n
			r.res.HandedOff, r.res.ExecutiveName = true, n
			r.res.Notes = append(r.res.Notes, "Assistant unavailable; handed to sales.")
		}
	} else if r.res.HandedOff {
		name = r.res.ExecutiveName
	}
	if !chat.HasBooking && r.res.Booking == nil && len(r.res.Slots) == 0 {
		if slots, err := r.actions.FreeSlots(ctx, r.now, 14); err == nil {
			if len(slots) > 4 {
				slots = slots[:4]
			}
			r.res.Slots = slots
		}
	}
	r.res.Reply = FallbackReply(chat.Lang, name)
	return r.res
}
