package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"

	"leadflow/internal/ai"
	"leadflow/internal/ai/assistant"
	"leadflow/internal/auth"
	"leadflow/internal/leads"
	"leadflow/internal/models"
	"leadflow/internal/store"
)

// =============================================================================
// BUYER CHAT ENDPOINTS (no login)
//
//	POST /api/chat/start     name, phone, project, language, consent -> greeting
//	GET  /api/chat           the chat so far (to resume after a reload)
//	POST /api/chat/messages  one buyer message -> the assistant's reply
//	POST /api/chat/book      book a slot card directly (no model involved)
//	POST /api/chat/handoff   the "talk to a person" button
//
// Who is who: /start sets an HttpOnly cookie with a random token. Every other
// chat endpoint finds the chat by that token's hash, and every action (booking
// included) is tied to that chat's lead. There is no lead id or phone number
// in any chat request, so one buyer cannot act for another.
// =============================================================================

const (
	ChatCookie        = "chat"
	chatCookieTTL     = 30 * 24 * time.Hour
	maxChatMessage    = 1000 // characters per buyer message
	maxBuyerMessages  = 60   // per chat; after that a person takes over
	historyForModel   = 30   // earlier messages sent to the model with each request
	maxSummaryRuntime = 40 * time.Second
)

// chatLocks makes sure one chat handles one message at a time, so two quick
// messages cannot both book or both reply out of order. It is in memory, which
// is enough for the single server this app runs as.
type chatLocks struct {
	mu   sync.Mutex
	busy map[string]bool
}

func (l *chatLocks) tryLock(id string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.busy[id] {
		return false
	}
	l.busy[id] = true
	return true
}

func (l *chatLocks) unlock(id string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.busy, id)
}

// ChatState is what the buyer's chat screen shows.
type ChatState struct {
	FirstName     string             `json:"firstName"`
	Language      string             `json:"language"`
	ProjectID     string             `json:"projectId"`
	ProjectName   string             `json:"projectName"`
	ReraNumber    string             `json:"reraNumber"` // shown in the chat footer
	Messages      []models.Message   `json:"messages"`
	HandedOff     bool               `json:"handedOff"`
	ExecutiveName string             `json:"executiveName,omitempty"`
	Visit         *store.VisitDetail `json:"visit"` // the booked visit, if any
}

// ChatReply is the answer to one buyer action.
type ChatReply struct {
	Messages      []models.Message   `json:"messages"` // the new messages, buyer's and assistant's
	Slots         []assistant.Slot   `json:"slots"`    // free times to show as cards (may be empty)
	Visit         *store.VisitDetail `json:"visit"`    // set when a visit was booked
	HandedOff     bool               `json:"handedOff"`
	ExecutiveName string             `json:"executiveName,omitempty"`
}

// ---------------------------------------------------------------------------
// Start and resume
// ---------------------------------------------------------------------------

type startChatRequest struct {
	Name      string `json:"name"`
	Phone     string `json:"phone"`
	ProjectID string `json:"projectId"`
	Language  string `json:"language"`
	Consent   bool   `json:"consent"`
}

func (s *Server) startChat(w http.ResponseWriter, r *http.Request) {
	var req startChatRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	// DPDP Act: we only talk to, and later remind, people who agreed to it.
	if !req.Consent {
		writeError(w, http.StatusBadRequest, "Please agree to be contacted about your enquiry to start the chat.")
		return
	}
	if req.ProjectID == "" {
		writeError(w, http.StatusBadRequest, "Choose a project.")
		return
	}
	in, err := leads.Input{
		Name: req.Name, Phone: req.Phone, Source: models.SourceWebsite, ProjectID: req.ProjectID,
		Language: req.Language, Consent: true, Note: "Started a website chat",
	}.Clean()
	if err != nil {
		s.writeLeadError(w, err)
		return
	}

	token, hash, err := auth.NewSessionToken()
	if err != nil {
		s.serverError(w, "chat token", err)
		return
	}
	chat, err := s.store.StartChat(r.Context(), in, hash)
	if err != nil {
		s.writeLeadError(w, err)
		return
	}

	// The greeting is sent at once, without the model, so every enquiry is
	// answered within seconds whatever the model is doing.
	greeting := assistant.Greeting(chat.Language, firstName(in.Name), chat.ProjectName)
	if _, err := s.store.AddChatMessages(r.Context(), chat.ConversationID, chat.LeadID,
		[]store.NewMessage{{Role: models.MessageAssistant, Content: greeting}}); err != nil {
		s.serverError(w, "chat greeting", err)
		return
	}

	s.setChatCookie(w, token)
	s.writeChatState(w, r, chat, firstName(in.Name), http.StatusCreated)
}

func (s *Server) getChat(w http.ResponseWriter, r *http.Request) {
	chat, ok := s.chatFromCookie(w, r)
	if !ok {
		return
	}
	s.writeChatState(w, r, chat, firstName(chat.BuyerName), http.StatusOK)
}

func (s *Server) writeChatState(w http.ResponseWriter, r *http.Request, chat store.ChatSession, first string, status int) {
	msgs, err := s.store.ChatMessages(r.Context(), chat.ConversationID, 200, false)
	if err != nil {
		s.serverError(w, "chat messages", err)
		return
	}
	visit, err := s.store.ChatVisit(r.Context(), chat.ConversationID)
	if err != nil {
		s.serverError(w, "chat visit", err)
		return
	}
	writeJSON(w, status, ChatState{
		FirstName: first, Language: chat.Language, ProjectID: chat.ProjectID, ProjectName: chat.ProjectName,
		ReraNumber: chat.ReraNumber, Messages: msgs, HandedOff: chat.HandedOff,
		ExecutiveName: shownExecutive(chat), Visit: visit,
	})
}

// ---------------------------------------------------------------------------
// One buyer message
// ---------------------------------------------------------------------------

type chatMessageRequest struct {
	Text string `json:"text"`
}

func (s *Server) chatMessage(w http.ResponseWriter, r *http.Request) {
	var req chatMessageRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	text := strings.TrimSpace(req.Text)
	if text == "" || utf8.RuneCountInString(text) > maxChatMessage {
		writeError(w, http.StatusBadRequest, "Write a message of up to 1,000 characters.")
		return
	}

	chat, ok := s.lockedChat(w, r)
	if !ok {
		return
	}
	defer s.chatLocks.unlock(chat.ConversationID)
	ctx := r.Context()

	// Load the history BEFORE saving the new message, so the model sees it once.
	history, err := s.store.ChatMessages(ctx, chat.ConversationID, historyForModel, false)
	if err != nil {
		s.serverError(w, "chat history", err)
		return
	}
	// Save the buyer's words first: whatever happens next, they are on record.
	saved, err := s.store.AddChatMessages(ctx, chat.ConversationID, chat.LeadID,
		[]store.NewMessage{{Role: models.MessageUser, Content: text}})
	if err != nil {
		s.serverError(w, "chat save", err)
		return
	}

	actions := &chatActions{store: s.store, chat: chat, now: s.now()}
	var res assistant.Result
	if chat.UserMessages >= maxBuyerMessages {
		// A very long chat is better continued by a person (and caps model cost).
		name, _ := actions.Handoff(ctx, "Long chat; please continue by phone.")
		res = assistant.Result{Reply: assistant.HandoffReply(chat.Language, name), HandedOff: true, ExecutiveName: name}
	} else {
		in, err := s.assistantInput(ctx, chat, history, text)
		if err != nil {
			s.serverError(w, "chat facts", err)
			return
		}
		res = s.agent.Reply(ctx, in, actions)
	}

	more, err := s.saveResult(ctx, chat, res)
	if err != nil {
		s.serverError(w, "chat save reply", err)
		return
	}
	s.respondChat(w, r, actions.chat, append(saved, more...), res)
}

// assistantInput gathers what the agent needs: the fact sheet, the chat's
// details and the history as model turns.
func (s *Server) assistantInput(ctx context.Context, chat store.ChatSession, history []models.Message, text string) (assistant.Input, error) {
	entries, err := s.store.ProjectFacts(ctx, chat.ProjectID)
	if err != nil {
		return assistant.Input{}, err
	}
	facts := make([]assistant.Fact, 0, len(entries))
	for _, e := range entries {
		if e.Topic == "rera" { // given separately, from the project itself
			continue
		}
		facts = append(facts, assistant.Fact{Topic: e.Topic, Label: e.Label, Value: e.Value})
	}
	return assistant.Input{
		Chat: assistant.ChatContext{
			Lang: chat.Language, ProjectName: chat.ProjectName, ReraNumber: chat.ReraNumber,
			BuyerName: chat.BuyerName, Facts: facts, Now: s.now(),
			HasBooking: chat.HasBooking, HandedOff: chat.HandedOff, ExecutiveName: chat.ExecutiveName,
		},
		History: toTurns(history),
		Message: text,
	}, nil
}

// toTurns turns saved messages into model turns. Messages from the same side
// in a row are joined, and a history that opens with our greeting gets a
// short buyer turn first, because models expect the buyer to speak first.
func toTurns(msgs []models.Message) []ai.Turn {
	var turns []ai.Turn
	for _, m := range msgs {
		role := ai.RoleUser
		if m.Role == models.MessageAssistant {
			role = ai.RoleModel
		}
		if len(turns) == 0 && role == ai.RoleModel {
			turns = append(turns, ai.Turn{Role: ai.RoleUser, Parts: []ai.Part{{Text: "(The buyer opened the chat.)"}}})
		}
		if n := len(turns); n > 0 && turns[n-1].Role == role {
			turns[n-1].Parts = append(turns[n-1].Parts, ai.Part{Text: "\n" + m.Content})
			continue
		}
		turns = append(turns, ai.Turn{Role: role, Parts: []ai.Part{{Text: m.Content}}})
	}
	return turns
}

// saveResult stores what the assistant did: staff notes, the reply, and any
// flags. A handoff or booking also starts writing the salesperson's summary.
func (s *Server) saveResult(ctx context.Context, chat store.ChatSession, res assistant.Result) ([]models.Message, error) {
	var msgs []store.NewMessage
	for _, n := range res.Notes {
		msgs = append(msgs, store.NewMessage{Role: models.MessageSystem, Content: n})
	}
	msgs = append(msgs, store.NewMessage{Role: models.MessageAssistant, Content: res.Reply})
	saved, err := s.store.AddChatMessages(ctx, chat.ConversationID, chat.LeadID, msgs)
	if err != nil {
		return nil, err
	}
	for _, f := range res.Flags {
		if err := s.store.FlagChat(ctx, chat.ConversationID, f); err != nil {
			return nil, err
		}
	}
	if res.HandedOff || res.Booking != nil {
		s.writeSummaryLater(chat)
	}

	// The buyer sees only their own words and the assistant's replies.
	shown := saved[:0]
	for _, m := range saved {
		if m.Role != models.MessageSystem {
			shown = append(shown, m)
		}
	}
	return shown, nil
}

func (s *Server) respondChat(w http.ResponseWriter, r *http.Request, chat store.ChatSession, msgs []models.Message, res assistant.Result) {
	reply := ChatReply{Messages: msgs, Slots: res.Slots, HandedOff: chat.HandedOff, ExecutiveName: shownExecutive(chat)}
	if reply.Slots == nil {
		reply.Slots = []assistant.Slot{}
	}
	if res.Booking != nil {
		d, err := s.store.VisitDetailByCode(r.Context(), s.now(), res.Booking.VisitCode)
		if err != nil {
			s.serverError(w, "chat visit", err)
			return
		}
		reply.Visit = &d
		reply.Slots = []assistant.Slot{} // booked: no more cards
	}
	writeJSON(w, http.StatusOK, reply)
}

// ---------------------------------------------------------------------------
// Slot cards and the "talk to a person" button: no model involved
// ---------------------------------------------------------------------------

type chatBookRequest struct {
	SlotID string `json:"slotId"`
}

func (s *Server) chatBook(w http.ResponseWriter, r *http.Request) {
	var req chatBookRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !reUUID.MatchString(req.SlotID) {
		writeError(w, http.StatusBadRequest, "Choose a time slot.")
		return
	}
	chat, ok := s.lockedChat(w, r)
	if !ok {
		return
	}
	defer s.chatLocks.unlock(chat.ConversationID)

	actions := &chatActions{store: s.store, chat: chat, now: s.now()}
	b, err := actions.Book(r.Context(), req.SlotID)
	switch {
	case errors.Is(err, assistant.ErrSlotTaken):
		writeError(w, http.StatusConflict, "That time was just taken. Please choose another slot.")
		return
	case errors.Is(err, assistant.ErrAlreadyBooked):
		writeError(w, http.StatusConflict, "You already have a visit booked. You can change it from your visit link.")
		return
	case errors.Is(err, assistant.ErrNeedsPerson):
		s.confirmByPhone(w, r, actions)
		return
	case err != nil:
		s.serverError(w, "chat book", err)
		return
	}

	when := assistant.WhenText(b.StartsAt)
	res := assistant.Result{
		Reply:   assistant.BookedReply(chat.Language, when, b.ExecutiveName),
		Booking: &b, HandedOff: true, ExecutiveName: b.ExecutiveName,
		Notes: []string{"Visit booked from a slot card for " + when + " with " + b.ExecutiveName + "."},
	}
	msgs, err := s.saveResult(r.Context(), chat, res)
	if err != nil {
		s.serverError(w, "chat save reply", err)
		return
	}
	s.respondChat(w, r, actions.chat, msgs, res)
}

// confirmByPhone hands a booking request to a person, for a phone number that
// belonged to a lead before this chat (see store/chat.go).
func (s *Server) confirmByPhone(w http.ResponseWriter, r *http.Request, actions *chatActions) {
	name, err := actions.Handoff(r.Context(), "Wants a site visit; confirm the time by phone.")
	if err != nil {
		s.serverError(w, "chat handoff", err)
		return
	}
	res := assistant.Result{
		Reply: assistant.ConfirmByPhoneReply(actions.chat.Language, name), HandedOff: true, ExecutiveName: name,
		Notes: []string{"Buyer picked a slot card; this number already had a lead, so the visit must be confirmed by phone."},
	}
	msgs, err := s.saveResult(r.Context(), actions.chat, res)
	if err != nil {
		s.serverError(w, "chat save reply", err)
		return
	}
	s.respondChat(w, r, actions.chat, msgs, res)
}

func (s *Server) chatHandoff(w http.ResponseWriter, r *http.Request) {
	chat, ok := s.lockedChat(w, r)
	if !ok {
		return
	}
	defer s.chatLocks.unlock(chat.ConversationID)

	actions := &chatActions{store: s.store, chat: chat, now: s.now()}
	name, err := actions.Handoff(r.Context(), "")
	if err != nil {
		s.serverError(w, "chat handoff", err)
		return
	}
	res := assistant.Result{
		Reply: assistant.HandoffReply(chat.Language, name), HandedOff: true, ExecutiveName: name,
		Notes: []string{"Buyer pressed \"talk to a person\"."},
	}
	msgs, err := s.saveResult(r.Context(), chat, res)
	if err != nil {
		s.serverError(w, "chat save reply", err)
		return
	}
	s.respondChat(w, r, actions.chat, msgs, res)
}

// ---------------------------------------------------------------------------
// Handoff summary, written in the background
// ---------------------------------------------------------------------------

// writeSummaryLater writes the salesperson's summary without making the buyer
// wait for a second model call. s.background lets shutdown (and tests) wait
// for it to finish.
func (s *Server) writeSummaryLater(chat store.ChatSession) {
	s.background.Add(1)
	go func() {
		defer s.background.Done()
		ctx, cancel := context.WithTimeout(context.Background(), maxSummaryRuntime)
		defer cancel()
		if err := s.writeSummary(ctx, chat.ConversationID, chat.LeadID); err != nil {
			slog.Error("chat summary failed", "conversation", chat.ConversationID, "error", err)
		}
	}()
}

func (s *Server) writeSummary(ctx context.Context, conversationID, leadID string) error {
	lead, err := s.store.ChatByID(ctx, conversationID)
	if err != nil {
		return err
	}
	visit, err := s.store.ChatVisit(ctx, conversationID)
	if err != nil {
		return err
	}
	facts := assistant.BuyerFacts{
		Name: lead.BuyerName, Project: lead.ProjectName,
		Wants: deref(lead.Wants), BudgetMinLakh: lead.BudgetMinLakh, BudgetMaxLakh: lead.BudgetMaxLakh,
		Timeline: deref(lead.Timeline),
	}
	if visit != nil {
		facts.Visit = "booked for " + assistant.WhenText(visit.StartsAt) + " with " + visit.ExecutiveName
	}
	msgs, err := s.store.ChatMessages(ctx, conversationID, 40, false)
	if err != nil {
		return err
	}
	return s.store.SetChatSummary(ctx, conversationID, assistant.Summary(ctx, s.llm, facts, toTurns(msgs)))
}

// ---------------------------------------------------------------------------
// Staff: the sales head's review queue of flagged chats
// ---------------------------------------------------------------------------

func (s *Server) listFlaggedChats(w http.ResponseWriter, r *http.Request) {
	chats, err := s.store.FlaggedChats(r.Context())
	if err != nil {
		s.serverError(w, "flagged chats", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"chats": chats})
}

func (s *Server) reviewFlaggedChat(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.UserFromContext(r.Context())
	id := chi.URLParam(r, "id")
	if !reUUID.MatchString(id) {
		writeError(w, http.StatusNotFound, "We could not find that chat.")
		return
	}
	ok, err := s.store.ReviewFlag(r.Context(), id, user.ID)
	if err != nil {
		s.serverError(w, "review flag", err)
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "That chat is not waiting for review.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func (s *Server) setChatCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     ChatCookie,
		Value:    token,
		Path:     "/api/chat", // sent only to the chat endpoints
		MaxAge:   int(chatCookieTTL.Seconds()),
		HttpOnly: true, // JavaScript cannot read it
		Secure:   s.cfg.SecureCookies(),
		SameSite: http.SameSiteLaxMode,
	})
}

// chatFromCookie finds the buyer's chat, or answers 404 if there is none.
func (s *Server) chatFromCookie(w http.ResponseWriter, r *http.Request) (store.ChatSession, bool) {
	c, err := r.Cookie(ChatCookie)
	if err != nil || c.Value == "" {
		writeError(w, http.StatusNotFound, "No chat yet. Start one first.")
		return store.ChatSession{}, false
	}
	chat, err := s.store.ChatByToken(r.Context(), auth.HashToken(c.Value))
	if errors.Is(err, store.ErrChatNotFound) {
		writeError(w, http.StatusNotFound, "No chat yet. Start one first.")
		return store.ChatSession{}, false
	}
	if err != nil {
		s.serverError(w, "chat lookup", err)
		return store.ChatSession{}, false
	}
	return chat, true
}

// lockedChat is chatFromCookie plus the one-message-at-a-time lock. The
// caller must unlock.
func (s *Server) lockedChat(w http.ResponseWriter, r *http.Request) (store.ChatSession, bool) {
	chat, ok := s.chatFromCookie(w, r)
	if !ok {
		return chat, false
	}
	if !s.chatLocks.tryLock(chat.ConversationID) {
		writeError(w, http.StatusConflict, "Still answering your last message. One moment.")
		return chat, false
	}
	return chat, true
}

// shownExecutive is the salesperson's name, once the buyer has been handed over.
func shownExecutive(chat store.ChatSession) string {
	if chat.HandedOff {
		return chat.ExecutiveName
	}
	return ""
}

func firstName(name string) string {
	if f := strings.Fields(name); len(f) > 0 {
		return f[0]
	}
	return name
}

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
