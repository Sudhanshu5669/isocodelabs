package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"leadflow/internal/leads"
	"leadflow/internal/models"
	"leadflow/internal/pii"
)

// =============================================================================
// THE BUYER CHAT
//
// Buyers do not log in. Starting a chat saves them as a lead (through the same
// IngestLead as every other source, so duplicates are caught) and opens a
// conversation that only the holder of a random cookie token can continue.
// We store only a hash of that token, and it expires on the server.
//
// A chat proves only that someone TYPED a phone number, not that they own it.
// So a chat sees and changes only what it created itself:
//   - the buyer's name is the one typed in this chat, never the stored lead's;
//   - the only visit it shows is one booked in this chat (conversations.visit_id);
//   - if the phone already belonged to a lead (new_lead = false), the chat may
//     not book for that lead or overwrite its details: a person confirms by
//     phone. A one-time code sent to the phone would remove this limit; it
//     needs a real SMS or WhatsApp sender, which this build mocks.
//
// Files: chat.go (this file: the buyer side) · chat_staff.go (what staff see).
// =============================================================================

// ErrChatNotFound means the chat token is unknown or expired.
var ErrChatNotFound = errors.New("store: chat not found")

// ChatTokenTTL is how long a buyer's chat link stays valid, on the server.
const ChatTokenTTL = 30 * 24 * time.Hour

// ChatSession is everything the assistant needs to know about one chat.
type ChatSession struct {
	ConversationID string
	LeadID         string
	BuyerName      string // as typed in THIS chat
	NewLead        bool   // this chat created the lead (see the note at the top)
	Language       string
	ProjectID      string
	ProjectName    string
	ReraNumber     string

	HandedOff     bool
	ExecutiveName string // the lead's salesperson, "" if none yet
	HasBooking    bool   // a visit booked in THIS chat is still upcoming

	Wants         *string
	BudgetMinLakh *int
	BudgetMaxLakh *int
	Timeline      *string

	UserMessages int // how many messages the buyer has sent
}

const chatSessionQuery = `
	SELECT c.id::text, l.id::text, COALESCE(c.buyer_name, l.name), c.new_lead, c.language, p.id, p.name, p.rera_number,
	       c.handed_off_at IS NOT NULL, u.name,
	       EXISTS (SELECT 1 FROM visits v WHERE v.id = c.visit_id AND v.status = 'booked'),
	       l.wants, l.budget_min_lakh, l.budget_max_lakh, l.timeline,
	       (SELECT count(*) FROM messages m WHERE m.conversation_id = c.id AND m.role = 'user')
	FROM conversations c
	JOIN leads l ON l.id = c.lead_id
	JOIN projects p ON p.id = c.project_id
	LEFT JOIN users u ON u.id = l.assigned_to
	`

func scanChatSession(row pgx.Row) (ChatSession, error) {
	var c ChatSession
	// Both names are encrypted (pii.Name). COALESCE picks the chat's own name
	// or the lead's while still encrypted; NULL executive name reads as "".
	err := row.Scan(&c.ConversationID, &c.LeadID, pii.Open(pii.Name, &c.BuyerName), &c.NewLead, &c.Language, &c.ProjectID,
		&c.ProjectName, &c.ReraNumber, &c.HandedOff, pii.Open(pii.Name, &c.ExecutiveName), &c.HasBooking,
		&c.Wants, &c.BudgetMinLakh, &c.BudgetMaxLakh, &c.Timeline, &c.UserMessages)
	if errors.Is(err, pgx.ErrNoRows) {
		return ChatSession{}, ErrChatNotFound
	}
	return c, err
}

// StartChat saves the buyer as a lead and opens a conversation for them.
// in must already have passed Input.Clean, with a project and consent.
//
// Errors: *leads.ValidationError (unknown project).
func (s *Store) StartChat(ctx context.Context, in leads.Input, tokenHash []byte) (ChatSession, error) {
	res, err := s.IngestLead(ctx, in, "")
	if err != nil {
		return ChatSession{}, err
	}
	var convID string
	err = s.pool.QueryRow(ctx, `
		INSERT INTO conversations (lead_id, project_id, language, token_hash, token_expires_at, buyer_name, new_lead)
		VALUES ($1::uuid, $2, $3, $4, $5, $6, $7)
		RETURNING id::text`, res.Lead.ID, in.ProjectID, in.Language, tokenHash,
		time.Now().Add(ChatTokenTTL), pii.Seal(pii.Name, in.Name), res.Outcome == OutcomeCreated).Scan(&convID)
	if err != nil {
		return ChatSession{}, err
	}
	return s.ChatByID(ctx, convID)
}

// ChatByToken finds the chat a cookie token belongs to. An expired token
// finds nothing, whatever the browser still holds.
//
// Errors: ErrChatNotFound.
func (s *Store) ChatByToken(ctx context.Context, tokenHash []byte) (ChatSession, error) {
	return scanChatSession(s.pool.QueryRow(ctx,
		chatSessionQuery+`WHERE c.token_hash = $1 AND c.token_expires_at > now()`, tokenHash))
}

// SetChatVisit records that a visit was booked in this chat, which is what
// allows the chat to show it.
func (s *Store) SetChatVisit(ctx context.Context, conversationID, visitID string) error {
	_, err := s.pool.Exec(ctx, `UPDATE conversations SET visit_id = $2::uuid WHERE id = $1::uuid`, conversationID, visitID)
	return err
}

// ChatVisit returns the visit booked in this chat if it is still upcoming
// (nil otherwise). A chat never sees visits it did not book.
func (s *Store) ChatVisit(ctx context.Context, conversationID string) (*VisitDetail, error) {
	var code string
	err := s.pool.QueryRow(ctx, `
		SELECT v.code FROM conversations c JOIN visits v ON v.id = c.visit_id
		WHERE c.id = $1::uuid AND v.status = 'booked'`, conversationID).Scan(&code)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	d, err := s.VisitDetailByCode(ctx, time.Now(), code)
	if err != nil {
		return nil, err
	}
	return &d, nil
}

// ChatByID loads a chat by its conversation id (used for background work).
//
// Errors: ErrChatNotFound.
func (s *Store) ChatByID(ctx context.Context, conversationID string) (ChatSession, error) {
	return scanChatSession(s.pool.QueryRow(ctx, chatSessionQuery+`WHERE c.id = $1::uuid`, conversationID))
}

// ChatMessages returns the last `limit` messages of a conversation, oldest
// first. withSystem includes the staff-only notes.
func (s *Store) ChatMessages(ctx context.Context, conversationID string, limit int, withSystem bool) ([]models.Message, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT * FROM (
			SELECT id::text, conversation_id::text, role, content, created_at FROM messages
			WHERE conversation_id = $1::uuid AND ($3 OR role <> 'system')
			ORDER BY created_at DESC, id DESC LIMIT $2
		) recent ORDER BY created_at, id`, conversationID, limit, withSystem)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.Message{}
	for rows.Next() {
		var m models.Message
		if err := rows.Scan(&m.ID, &m.ConversationID, &m.Role, pii.Open(pii.Text, &m.Content), &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// NewMessage is a message about to be saved.
type NewMessage struct {
	Role    string // models.MessageUser, MessageAssistant or MessageSystem
	Content string
}

// AddChatMessages saves messages in order, in one transaction. It also keeps
// the lead's timeline honest: the first assistant reply sets
// first_response_at, and a buyer who writes moves a new lead to "engaged".
func (s *Store) AddChatMessages(ctx context.Context, conversationID, leadID string, msgs []NewMessage) ([]models.Message, error) {
	var saved []models.Message
	err := s.withTx(ctx, func(tx pgx.Tx) error {
		saved = saved[:0]
		// Timestamps one microsecond apart keep the order exact, even though
		// the whole batch is saved within the same instant.
		base := time.Now()
		for i, m := range msgs {
			var out models.Message
			if err := tx.QueryRow(ctx, `
				INSERT INTO messages (conversation_id, role, content, created_at)
				VALUES ($1::uuid, $2, $3, $4)
				RETURNING id::text, conversation_id::text, role, content, created_at`,
				conversationID, m.Role, pii.Seal(pii.Text, m.Content), base.Add(time.Duration(i)*time.Microsecond),
			).Scan(&out.ID, &out.ConversationID, &out.Role, pii.Open(pii.Text, &out.Content), &out.CreatedAt); err != nil {
				return err
			}
			saved = append(saved, out)

			switch m.Role {
			case models.MessageAssistant:
				if _, err := tx.Exec(ctx, `UPDATE leads SET first_response_at = $2
					WHERE id = $1::uuid AND first_response_at IS NULL`, leadID, out.CreatedAt); err != nil {
					return err
				}
			case models.MessageUser:
				if _, err := tx.Exec(ctx, `UPDATE leads SET status = 'engaged', updated_at = now()
					WHERE id = $1::uuid AND status = 'new'`, leadID); err != nil {
					return err
				}
			}
		}
		_, err := tx.Exec(ctx, `UPDATE conversations SET updated_at = now() WHERE id = $1::uuid`, conversationID)
		return err
	})
	return saved, err
}

// SaveChatProfile records what the buyer wants. Empty values are ignored, so
// a later message never wipes an earlier answer. A chat that did not create
// the lead (overwrite = false) may only fill details the lead does not have
// yet; it can never change what is already recorded.
func (s *Store) SaveChatProfile(ctx context.Context, leadID string, overwrite bool, wants string, budgetMin, budgetMax *int, timeline string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE leads SET
			wants           = CASE WHEN $6::boolean THEN COALESCE($2::text, wants) ELSE COALESCE(wants, $2::text) END,
			budget_min_lakh = CASE WHEN $6::boolean THEN COALESCE($3::int, budget_min_lakh) ELSE COALESCE(budget_min_lakh, $3::int) END,
			budget_max_lakh = CASE WHEN $6::boolean THEN COALESCE($4::int, budget_max_lakh) ELSE COALESCE(budget_max_lakh, $4::int) END,
			timeline        = CASE WHEN $6::boolean THEN COALESCE($5::text, timeline) ELSE COALESCE(timeline, $5::text) END,
			updated_at      = now()
		WHERE id = $1::uuid`, leadID, nullable(wants), budgetMin, budgetMax, nullable(timeline), overwrite)
	return err
}

// HandOffChat asks a salesperson to take over and returns their name. A lead
// that already has a salesperson keeps them. Otherwise the executive who shows
// this project and has the fewest open leads gets it, so work is shared fairly.
// It returns "" if there is no active executive at all.
func (s *Store) HandOffChat(ctx context.Context, conversationID, leadID, projectID string) (string, error) {
	var name string
	err := s.withTx(ctx, func(tx pgx.Tx) error {
		var assigned *string
		var assigneeActive bool
		if err := tx.QueryRow(ctx, `
			SELECT l.assigned_to::text, COALESCE(u.active, false)
			FROM leads l LEFT JOIN users u ON u.id = l.assigned_to
			WHERE l.id = $1::uuid FOR UPDATE OF l`, leadID).Scan(&assigned, &assigneeActive); err != nil {
			return err
		}
		// A salesperson who has left (switched off) cannot take the chat.
		if assigned != nil && !assigneeActive {
			assigned = nil
		}
		if assigned == nil {
			var id string
			err := tx.QueryRow(ctx, `
				SELECT u.id::text FROM users u
				WHERE u.role = 'executive' AND u.active
				ORDER BY
					-- prefer executives who show this project
					NOT EXISTS (SELECT 1 FROM visit_slots s WHERE s.executive_id = u.id AND s.project_id = $1),
					(SELECT count(*) FROM leads l WHERE l.assigned_to = u.id AND l.status IN ('new', 'engaged', 'visit_booked')),
					-- then the longest-serving (names are encrypted, so not alphabetical)
					u.created_at, u.id
				LIMIT 1`, projectID).Scan(&id)
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
			if id != "" {
				if _, err := tx.Exec(ctx, `UPDATE leads SET assigned_to = $2::uuid, updated_at = now() WHERE id = $1::uuid`, leadID, id); err != nil {
					return err
				}
				assigned = &id
			}
		}
		if _, err := tx.Exec(ctx, `UPDATE conversations SET handed_off_at = COALESCE(handed_off_at, now()), updated_at = now()
			WHERE id = $1::uuid`, conversationID); err != nil {
			return err
		}
		name = ""
		if assigned != nil {
			return tx.QueryRow(ctx, `SELECT name FROM users WHERE id = $1::uuid`, *assigned).Scan(pii.Open(pii.Name, &name))
		}
		return nil
	})
	return name, err
}

// SetChatSummary stores the handoff summary the salesperson reads.
func (s *Store) SetChatSummary(ctx context.Context, conversationID, summary string) error {
	_, err := s.pool.Exec(ctx, `UPDATE conversations SET summary = $2 WHERE id = $1::uuid`, conversationID, pii.Seal(pii.Text, summary))
	return err
}

// FlagChat puts a chat in the sales head's review queue. Reasons add up, and a
// chat that was already reviewed goes back into the queue.
func (s *Store) FlagChat(ctx context.Context, conversationID, reason string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE conversations SET
			flagged = true,
			flag_reason = CASE WHEN flag_reason IS NULL OR flag_reviewed_at IS NOT NULL THEN $2
			                   ELSE left(flag_reason || '; ' || $2, 1000) END,
			flag_reviewed_at = NULL, flag_reviewed_by = NULL
		WHERE id = $1::uuid`, conversationID, reason)
	return err
}

// ProjectFacts returns the fact sheet of one project, in a stable order.
func (s *Store) ProjectFacts(ctx context.Context, projectID string) ([]models.FactSheetEntry, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id::text, project_id, topic, label, value, updated_by::text, updated_at
		FROM fact_sheet_entries WHERE project_id = $1 ORDER BY topic`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.FactSheetEntry{}
	for rows.Next() {
		var f models.FactSheetEntry
		if err := rows.Scan(&f.ID, &f.ProjectID, &f.Topic, &f.Label, &f.Value, &f.UpdatedBy, &f.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}
