package store

import (
	"context"
	"time"
)

// What staff see of the buyer chats: the full history on a lead's page, and
// the sales head's daily queue of flagged chats.

// ChatTranscript is one conversation with all its messages, staff notes included.
type ChatTranscript struct {
	ID          string        `json:"id"`
	ProjectID   *string       `json:"projectId"`
	Language    string        `json:"language"`
	Summary     *string       `json:"summary"`
	HandedOffAt *time.Time    `json:"handedOffAt"`
	Flagged     bool          `json:"flagged"`
	FlagReason  *string       `json:"flagReason"`
	CreatedAt   time.Time     `json:"createdAt"`
	Messages    []ChatMessage `json:"messages"`
}

// ChatMessage is one line of a transcript.
type ChatMessage struct {
	Role      string    `json:"role"` // user, assistant or system (staff-only note)
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"createdAt"`
}

// maxTranscriptMessages bounds one transcript on the lead page.
const maxTranscriptMessages = 500

// LeadChats returns every conversation of a lead, newest first. The caller
// must already have checked that the viewer may see this lead.
func (s *Store) LeadChats(ctx context.Context, leadID string) ([]ChatTranscript, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id::text, project_id, language, summary, handed_off_at, flagged, flag_reason, created_at
		FROM conversations WHERE lead_id = $1::uuid ORDER BY created_at DESC`, leadID)
	if err != nil {
		return nil, err
	}
	chats := []ChatTranscript{}
	for rows.Next() {
		var c ChatTranscript
		if err := rows.Scan(&c.ID, &c.ProjectID, &c.Language, &c.Summary, &c.HandedOffAt, &c.Flagged, &c.FlagReason, &c.CreatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		chats = append(chats, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for i := range chats {
		msgs, err := s.ChatMessages(ctx, chats[i].ID, maxTranscriptMessages, true)
		if err != nil {
			return nil, err
		}
		chats[i].Messages = make([]ChatMessage, len(msgs))
		for j, m := range msgs {
			chats[i].Messages[j] = ChatMessage{Role: m.Role, Content: m.Content, CreatedAt: m.CreatedAt}
		}
	}
	return chats, nil
}

// FlaggedChat is one entry of the sales head's review queue.
type FlaggedChat struct {
	ID         string    `json:"id"`
	LeadID     string    `json:"leadId"`
	LeadCode   string    `json:"leadCode"`
	LeadName   string    `json:"leadName"`
	FlagReason string    `json:"flagReason"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

// FlaggedChats lists chats waiting for review, oldest first.
func (s *Store) FlaggedChats(ctx context.Context) ([]FlaggedChat, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT c.id::text, l.id::text, l.code, l.name, COALESCE(c.flag_reason, ''), c.updated_at
		FROM conversations c JOIN leads l ON l.id = c.lead_id
		WHERE c.flagged AND c.flag_reviewed_at IS NULL
		ORDER BY c.updated_at
		LIMIT 200`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []FlaggedChat{}
	for rows.Next() {
		var f FlaggedChat
		if err := rows.Scan(&f.ID, &f.LeadID, &f.LeadCode, &f.LeadName, &f.FlagReason, &f.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// ReviewFlag marks a flagged chat as reviewed by the sales head. It reports
// false if there was no such flagged chat waiting.
func (s *Store) ReviewFlag(ctx context.Context, conversationID, reviewerID string) (bool, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE conversations SET flag_reviewed_at = now(), flag_reviewed_by = $2::uuid
		WHERE id = $1::uuid AND flagged AND flag_reviewed_at IS NULL`, conversationID, reviewerID)
	return tag.RowsAffected() == 1, err
}
