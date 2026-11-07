package store_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"leadflow/internal/auth"
	"leadflow/internal/models"
	"leadflow/internal/pii"
	"leadflow/internal/store"
)

// DPDP: personal data must be unreadable in the database itself, while the app
// still finds leads by phone, users by email, and leads by name.

// rawBytes reads one column exactly as stored, without decrypting.
func rawBytes(t *testing.T, sql string, args ...any) []byte {
	t.Helper()
	var b []byte
	if err := testPool.QueryRow(context.Background(), sql, args...).Scan(&b); err != nil {
		t.Fatal(err)
	}
	return b
}

func assertSealed(t *testing.T, what string, stored []byte, kind pii.Kind, plain string) {
	t.Helper()
	if bytes.Contains(stored, []byte(plain)) {
		t.Errorf("%s: plaintext %q is visible in the database", what, plain)
	}
	if got, err := pii.Decrypt(kind, stored); err != nil || got != plain {
		t.Errorf("%s: decrypts to %q, %v; want %q", what, got, err, plain)
	}
}

func TestPersonalDataIsStoredEncrypted(t *testing.T) {
	requireDB(t)
	ctx := context.Background()
	in := baseInput()
	in.Note = "Wants a call after 6 pm"
	res := ingest(t, in, "")
	id := res.Lead.ID
	phone := res.Lead.Phone // normalised by Clean

	assertSealed(t, "leads.name", rawBytes(t, `SELECT name FROM leads WHERE id = $1::uuid`, id), pii.Name, in.Name)
	assertSealed(t, "leads.phone", rawBytes(t, `SELECT phone FROM leads WHERE id = $1::uuid`, id), pii.Phone, phone)
	assertSealed(t, "lead_touches.note", rawBytes(t, `SELECT note FROM lead_touches WHERE lead_id = $1::uuid`, id), pii.Text, in.Note)
	if got := rawBytes(t, `SELECT phone_bidx FROM leads WHERE id = $1::uuid`, id); !bytes.Equal(got, pii.PhoneIndex(phone)) {
		t.Error("phone_bidx is not the HMAC of the normalised phone")
	}

	// The store still hands back plain values.
	if res.Lead.Name != in.Name {
		t.Errorf("store returned name %q, want %q", res.Lead.Name, in.Name)
	}

	// Chat: the buyer's name and every message.
	chatIn := baseInput()
	clean, err := chatIn.Clean()
	if err != nil {
		t.Fatal(err)
	}
	chat, err := testStore.StartChat(ctx, clean, []byte(randHex(16)))
	if err != nil {
		t.Fatal(err)
	}
	const said = "My number is 98765 43210, call me"
	if _, err := testStore.AddChatMessages(ctx, chat.ConversationID, chat.LeadID,
		[]store.NewMessage{{Role: models.MessageUser, Content: said}}); err != nil {
		t.Fatal(err)
	}
	assertSealed(t, "messages.content",
		rawBytes(t, `SELECT content FROM messages WHERE conversation_id = $1::uuid`, chat.ConversationID), pii.Text, said)
	assertSealed(t, "conversations.buyer_name",
		rawBytes(t, `SELECT buyer_name FROM conversations WHERE id = $1::uuid`, chat.ConversationID), pii.Name, clean.Name)
	msgs, err := testStore.ChatMessages(ctx, chat.ConversationID, 10, false)
	if err != nil || len(msgs) != 1 || msgs[0].Content != said {
		t.Errorf("ChatMessages = %+v, %v", msgs, err)
	}
	if err := testStore.SetChatSummary(ctx, chat.ConversationID, "Wants 3 BHK, budget 1 Cr"); err != nil {
		t.Fatal(err)
	}
	assertSealed(t, "conversations.summary",
		rawBytes(t, `SELECT summary FROM conversations WHERE id = $1::uuid`, chat.ConversationID), pii.Text, "Wants 3 BHK, budget 1 Cr")
}

func TestLeadSearchUsesBlindIndexes(t *testing.T) {
	requireDB(t)
	ctx := context.Background()
	first, last := "zq"+randHex(3), "xk"+randHex(3)
	in := baseInput()
	in.Name = strings.ToUpper(first[:1]) + first[1:] + " " + last
	res := ingest(t, in, "")
	ten := strings.TrimPrefix(res.Lead.Phone, "+91")

	search := func(q string) bool {
		t.Helper()
		page, err := testStore.ListLeads(ctx, headViewer(), store.LeadFilter{Query: q, Limit: 100})
		if err != nil {
			t.Fatal(err)
		}
		for _, l := range page.Leads {
			if l.ID == res.Lead.ID {
				return true
			}
		}
		return false
	}

	for _, q := range []string{first, last, strings.ToUpper(last), last + " " + first, res.Lead.Phone, ten, "0" + ten, res.Lead.Code} {
		if !search(q) {
			t.Errorf("search %q did not find the lead", q)
		}
	}
	// Encrypted values cannot be partly matched: these must NOT find it.
	for _, q := range []string{first[:3], first + " nomatch", ten[:6]} {
		if search(q) {
			t.Errorf("search %q found the lead, but partial names and phones cannot match", q)
		}
	}
}

func TestUserEmailLookupIgnoresCaseAndStaysUnique(t *testing.T) {
	requireDB(t)
	ctx := context.Background()
	email := "Case." + randHex(4) + "@Test.Local"
	u, err := testStore.CreateUser(ctx, email, "Nisha Rao", auth.RoleExecutive, "x")
	if err != nil {
		t.Fatal(err)
	}
	assertSealed(t, "users.email", rawBytes(t, `SELECT email FROM users WHERE id = $1::uuid`, u.ID), pii.Email, email)
	assertSealed(t, "users.name", rawBytes(t, `SELECT name FROM users WHERE id = $1::uuid`, u.ID), pii.Name, "Nisha Rao")

	got, err := testStore.UserByEmail(ctx, strings.ToLower(email))
	if err != nil || got.ID != u.ID || got.Email != email || got.Name != "Nisha Rao" {
		t.Fatalf("UserByEmail = %+v, %v", got, err)
	}
	if _, err := testStore.CreateUser(ctx, strings.ToUpper(email), "Someone Else", auth.RoleExecutive, "x"); !errors.Is(err, store.ErrEmailTaken) {
		t.Errorf("same email in another case: err = %v, want ErrEmailTaken", err)
	}
}
