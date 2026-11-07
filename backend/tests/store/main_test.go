package store_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"leadflow/internal/pii"
	"leadflow/internal/store"
	"leadflow/tests/testdb"
)

// Integration tests run against a real Postgres in a throw-away schema. See
// package testdb. Without DATABASE_URL they skip themselves.

var (
	testPool  *pgxpool.Pool
	testStore *store.Store
)

func TestMain(m *testing.M) {
	os.Exit(runTests(m))
}

func runTests(m *testing.M) int {
	flag.Parse() // testing.Short() needs flags parsed first
	if testing.Short() {
		return m.Run()
	}

	pool, cleanup, err := testdb.Open(context.Background(), "../../.env")
	defer cleanup()
	if err != nil {
		fmt.Println("integration tests disabled:", err)
		if testdb.Required() {
			return 1
		}
		return m.Run()
	}
	if pool != nil {
		testPool, testStore = pool, store.New(pool)
	}
	return m.Run()
}

// ---- helpers ---------------------------------------------------------------

func requireDB(t *testing.T) {
	t.Helper()
	if testStore == nil {
		t.Skip("no DATABASE_URL: skipping database integration test")
	}
}

func randHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// decrypted runs a query that returns one encrypted column and decrypts it.
func decrypted(t *testing.T, kind pii.Kind, sql string, args ...any) string {
	t.Helper()
	var v string
	if err := testPool.QueryRow(context.Background(), sql, args...).Scan(pii.Open(kind, &v)); err != nil {
		t.Fatal(err)
	}
	return v
}

// insertUserSQL adds a staff user directly. Email and name are encrypted and
// the email gets its blind index, exactly as the store writes them.
const insertUserSQL = `INSERT INTO users (email, email_bidx, name, role, password_hash)
	VALUES ($1, $2, $3, $4, 'x') RETURNING id::text`

func userArgs(email, name, role string) []any {
	return []any{pii.Seal(pii.Email, email), pii.EmailIndex(email), pii.Seal(pii.Name, name), role}
}

// newExecutive creates a sales executive and returns their id.
func newExecutive(t *testing.T) string {
	t.Helper()
	var id string
	err := testPool.QueryRow(context.Background(), insertUserSQL, userArgs(randHex(6)+"@test.local", "Test Exec", "executive")...).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// newLead creates a lead who has (or has not) consented to be contacted.
func newLead(t *testing.T, consent bool) string {
	t.Helper()
	var id string
	phone := "+91" + fmt.Sprintf("%010d", randInt())
	err := testPool.QueryRow(context.Background(), `
		INSERT INTO leads (name, phone, phone_bidx, name_words, source, project_id, consent)
		VALUES ($1, $2, $3, $4, 'website', 'project-1', $5) RETURNING id::text`,
		pii.Seal(pii.Name, "Test Buyer"), pii.Seal(pii.Phone, phone), pii.PhoneIndex(phone),
		pii.NameWordIndexes("Test Buyer"), consent).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func randInt() int {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return int(b[0])<<24 | int(b[1])<<16 | int(b[2])<<8 | int(b[3])
}

// futureStart returns a slot start comfortably beyond the minimum notice.
func futureStart(days int) time.Time {
	return time.Now().Add(time.Duration(days) * 24 * time.Hour).Truncate(time.Hour)
}

// newSlot creates an open one-hour slot for an executive and returns its id.
func newSlot(t *testing.T, execID string, start time.Time) string {
	t.Helper()
	slot, err := testStore.CreateSlot(context.Background(), "project-1", execID, start, start.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	return slot.ID
}

func scalar[T any](t *testing.T, sql string, args ...any) T {
	t.Helper()
	var v T
	if err := testPool.QueryRow(context.Background(), sql, args...).Scan(&v); err != nil {
		t.Fatal(err)
	}
	return v
}
