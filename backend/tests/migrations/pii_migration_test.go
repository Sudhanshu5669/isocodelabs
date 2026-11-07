package migrations_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"leadflow/internal/pii"
	"leadflow/internal/store"
	"leadflow/tests/testdb"
)

// The DPDP migrations (00005 to 00007) on data that already exists: plaintext
// rows must come out encrypted and still usable, and going back down must give
// the exact plaintext (and the old rules) back. This runs in its own
// throw-away schema because it changes the schema back and forth.

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	pool, cleanup, err := testdb.Open(context.Background(), "../../.env")
	if err != nil {
		fmt.Println("integration tests disabled:", err)
		cleanup()
		if testdb.Required() {
			os.Exit(1)
		}
		os.Exit(m.Run())
	}
	testPool = pool
	code := m.Run()
	cleanup()
	os.Exit(code)
}

const lastPlaintextVersion = 4 // the schema before personal data was encrypted

func TestEncryptMigrationRoundTrip(t *testing.T) {
	if testPool == nil {
		t.Skip("no DATABASE_URL: skipping database integration test")
	}
	ctx := context.Background()
	db := stdlib.OpenDBFromPool(testPool)
	defer db.Close()
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.ExecContext(ctx, q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	one := func(q string, args ...any) string {
		t.Helper()
		var v string
		if err := db.QueryRowContext(ctx, q, args...).Scan(&v); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
		return v
	}

	// 1. Back to the plaintext schema, and add rows the old way.
	if err := goose.DownTo(db, ".", lastPlaintextVersion); err != nil {
		t.Fatal(err)
	}
	userID := one(`INSERT INTO users (email, name, role, password_hash)
		VALUES ('Old.User@Test.Local', 'Old User', 'executive', 'x') RETURNING id::text`)
	leadID := one(`INSERT INTO leads (name, phone, source, project_id, consent)
		VALUES ('Kiran Desai', '+919000110777', 'website', 'project-1', true) RETURNING id::text`)
	exec(`INSERT INTO lead_touches (lead_id, source, note) VALUES ($1::uuid, 'website', 'First enquiry')`, leadID)
	exec(`INSERT INTO lead_touches (lead_id, source, note) VALUES ($1::uuid, 'manual', NULL)`, leadID)
	convID := one(`INSERT INTO conversations (lead_id, buyer_name, summary) VALUES ($1::uuid, 'Kiran', NULL) RETURNING id::text`, leadID)
	exec(`INSERT INTO messages (conversation_id, role, content) VALUES ($1::uuid, 'user', 'Is there parking?')`, convID)

	// 2. Encrypt.
	if err := goose.Up(db, "."); err != nil {
		t.Fatal(err)
	}
	raw := func(q string, args ...any) []byte {
		t.Helper()
		var b []byte
		if err := db.QueryRowContext(ctx, q, args...).Scan(&b); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
		return b
	}
	check := func(what string, stored []byte, kind pii.Kind, plain string) {
		t.Helper()
		if bytes.Contains(stored, []byte(plain)) {
			t.Errorf("%s still readable in the database", what)
		}
		if got, err := pii.Decrypt(kind, stored); err != nil || got != plain {
			t.Errorf("%s decrypts to %q, %v", what, got, err)
		}
	}
	check("users.email", raw(`SELECT email FROM users WHERE id = $1::uuid`, userID), pii.Email, "Old.User@Test.Local")
	check("users.name", raw(`SELECT name FROM users WHERE id = $1::uuid`, userID), pii.Name, "Old User")
	check("leads.name", raw(`SELECT name FROM leads WHERE id = $1::uuid`, leadID), pii.Name, "Kiran Desai")
	check("leads.phone", raw(`SELECT phone FROM leads WHERE id = $1::uuid`, leadID), pii.Phone, "+919000110777")
	check("conversations.buyer_name", raw(`SELECT buyer_name FROM conversations WHERE id = $1::uuid`, convID), pii.Name, "Kiran")
	check("messages.content", raw(`SELECT content FROM messages WHERE conversation_id = $1::uuid`, convID), pii.Text, "Is there parking?")
	if n := one(`SELECT count(*)::text FROM lead_touches WHERE lead_id = $1::uuid AND note IS NULL`, leadID); n != "1" {
		t.Errorf("a NULL note must stay NULL, found %s", n)
	}
	if !bytes.Equal(raw(`SELECT phone_bidx FROM leads WHERE id = $1::uuid`, leadID), pii.PhoneIndex("+919000110777")) {
		t.Error("phone blind index not computed")
	}

	// The app finds the migrated rows by their blind indexes.
	st := store.New(testPool)
	u, err := st.UserByEmail(ctx, "old.user@test.local")
	if err != nil || u.ID != userID || u.Name != "Old User" {
		t.Errorf("sign-in lookup after migration: %+v, %v", u, err)
	}
	page, err := st.ListLeads(ctx, store.Viewer{IsHead: true}, store.LeadFilter{Query: "desai kiran"})
	if err != nil || page.Total != 1 || page.Leads[0].ID != leadID || page.Leads[0].Phone != "+919000110777" {
		t.Errorf("name search after migration: %+v, %v", page, err)
	}

	// 3. And back: the plaintext and the old uniqueness rules return.
	if err := goose.DownTo(db, ".", lastPlaintextVersion); err != nil {
		t.Fatal(err)
	}
	if got := one(`SELECT email || '|' || name FROM users WHERE id = $1::uuid`, userID); got != "Old.User@Test.Local|Old User" {
		t.Errorf("users after down: %q", got)
	}
	if got := one(`SELECT name || '|' || phone FROM leads WHERE id = $1::uuid`, leadID); got != "Kiran Desai|+919000110777" {
		t.Errorf("leads after down: %q", got)
	}
	// A different SQL text on purpose: pgx caches prepared statements by text, and
	// Postgres refuses to reuse one whose result type changed (bytea -> text).
	if got := one(`SELECT content AS plain_content FROM messages WHERE conversation_id = $1::uuid`, convID); got != "Is there parking?" {
		t.Errorf("messages after down: %q", got)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO leads (name, phone, source, consent)
		VALUES ('Copy', '+919000110777', 'website', true)`); err == nil {
		t.Error("after down, the phone number must be unique again")
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO users (email, name, role, password_hash)
		VALUES ('old.user@test.local', 'Copy', 'executive', 'x')`); err == nil {
		t.Error("after down, emails must be unique ignoring case again")
	}

	// Leave the schema fully migrated.
	if err := goose.Up(db, "."); err != nil {
		t.Fatal(err)
	}
}
