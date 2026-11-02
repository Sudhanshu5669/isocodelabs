package models

import "time"

// Who wrote a message (messages.role).
const (
	MessageUser      = "user"
	MessageAssistant = "assistant"
	MessageSystem    = "system"
)

// Message is a row of the `messages` table: one line of a conversation.
type Message struct {
	ID             string    `db:"id" json:"id"`
	ConversationID string    `db:"conversation_id" json:"conversationId"`
	Role           string    `db:"role" json:"role"`
	Content        string    `db:"content" json:"content"`
	CreatedAt      time.Time `db:"created_at" json:"createdAt"`
}
