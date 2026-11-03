package httpapi

import (
	"crypto/subtle"
	"errors"
	"net/http"

	"leadflow/internal/leads"
	"leadflow/internal/reminders"
	"leadflow/internal/store"
)

// optOutByCode lets a buyer stop all messages from their visit page.
// POST /api/visits/{code}/opt-out
func (s *Server) optOutByCode(w http.ResponseWriter, r *http.Request) {
	detail, ok := s.visitByCode(w, r)
	if !ok {
		return
	}
	err := s.store.OptOutByVisitCode(r.Context(), detail.Code)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "We could not find that visit.")
		return
	}
	if err != nil {
		s.serverError(w, "opt out", err)
		return
	}
	s.respondWithVisit(w, r, detail.Code)
}

type inboundMessage struct {
	From string `json:"from"` // the buyer's phone number
	Text string `json:"text"`
}

// inboundMessage is called by the messaging provider when a buyer replies to
// a reminder. (In this build the provider is mocked; the endpoint has the
// shape a WhatsApp or SMS webhook would use.) A reply such as "STOP" opts the
// buyer out of all messages, as the DPDP Act requires.
//
// The provider proves it is the caller with a shared secret in the
// X-Webhook-Secret header. Without INBOUND_WEBHOOK_SECRET configured the
// endpoint does not exist (404).
//
// POST /api/webhooks/inbound {"from": "+919876543210", "text": "STOP"}
func (s *Server) inboundMessage(w http.ResponseWriter, r *http.Request) {
	secret := s.cfg.InboundWebhookSecret
	if secret == "" {
		http.NotFound(w, r)
		return
	}
	// ConstantTimeCompare takes the same time however much of the secret
	// matches, so the secret cannot be guessed one character at a time.
	if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Webhook-Secret")), []byte(secret)) != 1 {
		writeError(w, http.StatusUnauthorized, "Unknown caller.")
		return
	}
	var msg inboundMessage
	if err := decodeJSON(w, r, &msg); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !reminders.IsStop(msg.Text) {
		// Other replies would go to the chat assistant once WhatsApp is live.
		writeJSON(w, http.StatusOK, map[string]any{"optedOut": false})
		return
	}
	phone, err := leads.NormalizePhone(msg.From)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"optedOut": false})
		return
	}
	found, err := s.store.OptOutByPhone(r.Context(), phone)
	if err != nil {
		s.serverError(w, "inbound opt out", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"optedOut": found})
}
