package httpapi

import (
	"context"
	"errors"
	"strings"
	"time"

	"leadflow/internal/ai/assistant"
	"leadflow/internal/booking"
	"leadflow/internal/store"
)

// chatActions carries out the assistant's tool calls for ONE chat, using the
// store. It is the bridge between package assistant (which decides what to do)
// and package store (which does it). Every action is bound to this chat's lead
// and project, so the model can never act on another buyer.
type chatActions struct {
	store *store.Store
	chat  store.ChatSession
	now   time.Time
}

// chatActions satisfies assistant.Actions; the compiler checks this line.
var _ assistant.Actions = (*chatActions)(nil)

// bookableWindow is how far ahead a buyer may book.
const bookableWindow = 60 * 24 * time.Hour

func (a *chatActions) FreeSlots(ctx context.Context, from time.Time, days int) ([]assistant.Slot, error) {
	found, err := a.store.FreeSlots(ctx, a.now, a.chat.ProjectID, from, from.Add(time.Duration(days)*24*time.Hour))
	if err != nil {
		return nil, err
	}
	slots := make([]assistant.Slot, len(found))
	for i, f := range found {
		slots[i] = assistant.Slot{ID: f.ID, StartsAt: f.StartsAt, EndsAt: f.EndsAt, ExecutiveName: f.ExecutiveName}
	}
	return slots, nil
}

// Book books a slot of THIS chat's project for THIS chat's lead, through the
// same engine staff use. Booking also hands the buyer to the executive who
// will meet them.
func (a *chatActions) Book(ctx context.Context, slotID string) (assistant.Booking, error) {
	// The phone number already belonged to a lead before this chat. Typing a
	// number does not prove owning it, so a person confirms by phone instead.
	if !a.chat.NewLead {
		return assistant.Booking{}, assistant.ErrNeedsPerson
	}
	// The engine checks the slot is open; this also checks it belongs to the
	// project the buyer is chatting about.
	free, err := a.store.FreeSlots(ctx, a.now, a.chat.ProjectID, a.now, a.now.Add(bookableWindow))
	if err != nil {
		return assistant.Booking{}, err
	}
	var slot *store.FreeSlot
	for i := range free {
		if strings.EqualFold(free[i].ID, slotID) {
			slot = &free[i]
		}
	}
	if slot == nil {
		return assistant.Booking{}, assistant.ErrSlotTaken
	}

	visit, err := a.store.BookSlot(ctx, a.now, a.chat.LeadID, slot.ID)
	switch {
	case errors.Is(err, booking.ErrSlotUnavailable):
		return assistant.Booking{}, assistant.ErrSlotTaken
	case errors.Is(err, booking.ErrLeadAlreadyBooked):
		return assistant.Booking{}, assistant.ErrAlreadyBooked
	case err != nil:
		return assistant.Booking{}, err
	}
	a.chat.HasBooking = true
	// Remember that THIS chat booked it: the only visit a chat may show.
	if err := a.store.SetChatVisit(ctx, a.chat.ConversationID, visit.ID); err != nil {
		return assistant.Booking{}, err
	}

	// The booking made this executive the lead's salesperson; record the handoff.
	if _, err := a.store.HandOffChat(ctx, a.chat.ConversationID, a.chat.LeadID, a.chat.ProjectID); err != nil {
		return assistant.Booking{}, err
	}
	a.chat.HandedOff, a.chat.ExecutiveName = true, slot.ExecutiveName
	return assistant.Booking{VisitCode: visit.Code, StartsAt: slot.StartsAt, ExecutiveName: slot.ExecutiveName}, nil
}

func (a *chatActions) SaveProfile(ctx context.Context, p assistant.Profile) error {
	return a.store.SaveChatProfile(ctx, a.chat.LeadID, a.chat.NewLead, p.Wants, p.BudgetMinLakh, p.BudgetMaxLakh, p.Timeline)
}

func (a *chatActions) Handoff(ctx context.Context, reason string) (string, error) {
	name, err := a.store.HandOffChat(ctx, a.chat.ConversationID, a.chat.LeadID, a.chat.ProjectID)
	if err == nil {
		a.chat.HandedOff, a.chat.ExecutiveName = true, name
	}
	return name, err
}

func (a *chatActions) Flag(ctx context.Context, reason string) error {
	return a.store.FlagChat(ctx, a.chat.ConversationID, reason)
}
