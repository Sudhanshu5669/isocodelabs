package httpapi_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"leadflow/internal/ai"
	"leadflow/internal/auth"
	"leadflow/internal/config"
	"leadflow/internal/httpapi"
	"leadflow/internal/pii"
	"leadflow/tests/testdb"
)

// Integration tests for the HTTP layer, run against a throw-away schema in a
// real Postgres (see package testdb). Without DATABASE_URL they skip themselves.

var (
	testPool *pgxpool.Pool
	handler  http.Handler

	model      = &fakeModel{} // the chat's language model, scripted per test
	background sync.WaitGroup // background work (handoff summaries) to wait for
)

// fakeModel stands in for the real AI provider. Each test sets fn to script the replies.
type fakeModel struct {
	mu sync.Mutex
	fn func(req ai.Request) (ai.Turn, error)
}

func (f *fakeModel) Provider() string { return "fake" }
func (f *fakeModel) Model() string    { return "test" }

func (f *fakeModel) Complete(_ context.Context, req ai.Request) (ai.Turn, error) {
	f.mu.Lock()
	fn := f.fn
	f.mu.Unlock()
	if fn == nil {
		return ai.Turn{}, errors.New("fake model: no script set")
	}
	return fn(req)
}

func (f *fakeModel) script(fn func(req ai.Request) (ai.Turn, error)) {
	f.mu.Lock()
	f.fn = fn
	f.mu.Unlock()
}

func TestMain(m *testing.M) {
	os.Exit(runTests(m))
}

func runTests(m *testing.M) int {
	flag.Parse()
	if testing.Short() {
		return m.Run()
	}
	pool, cleanup, err := testdb.Open(context.Background(), "../../.env")
	defer cleanup()
	defer background.Wait() // runs first: let summaries finish before the schema is dropped
	if err != nil {
		fmt.Println("integration tests disabled:", err)
		if testdb.Required() {
			return 1
		}
		return m.Run()
	}
	if pool != nil {
		testPool = pool
		handler = httpapi.NewRouter(config.Config{AppEnv: "development"}, pool, model, &background)
	}
	return m.Run()
}

// ---- helpers ---------------------------------------------------------------

func requireDB(t *testing.T) {
	t.Helper()
	if handler == nil {
		t.Skip("no DATABASE_URL: skipping HTTP integration test")
	}
}

func randHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

const testPassword = "test-password-1"

// newUser inserts a staff user and returns their id and email.
func newUser(t *testing.T, role string) (id, email string) {
	t.Helper()
	hash, err := auth.HashPassword(testPassword)
	if err != nil {
		t.Fatal(err)
	}
	email = randHex(6) + "@test.local"
	err = testPool.QueryRow(context.Background(), `
		INSERT INTO users (email, email_bidx, name, role, password_hash) VALUES ($1, $2, $3, $4, $5) RETURNING id::text`,
		pii.Seal(pii.Email, email), pii.EmailIndex(email), pii.Seal(pii.Name, "Test User"), role, hash).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	return id, email
}

// loginAs signs a user in through the real login endpoint and returns the cookie.
func loginAs(t *testing.T, email string) *http.Cookie {
	t.Helper()
	rec := call(t, http.MethodPost, "/api/auth/login", nil, map[string]string{"email": email, "password": testPassword})
	if rec.Code != http.StatusOK {
		t.Fatalf("login failed: %d %s", rec.Code, rec.Body.String())
	}
	for _, c := range rec.Result().Cookies() {
		if c.Name == auth.CookieName {
			return c
		}
	}
	t.Fatal("login set no session cookie")
	return nil
}

// newStaff creates a user and signs them in.
func newStaff(t *testing.T, role string) (id string, cookie *http.Cookie) {
	t.Helper()
	id, email := newUser(t, role)
	return id, loginAs(t, email)
}

func newLead(t *testing.T, assignedTo string) (id, phone string) {
	t.Helper()
	// Always a valid Indian mobile (starts with 9), so code that checks numbers accepts it.
	phone = "+919" + fmt.Sprintf("%09d", time.Now().UnixNano()%1000000000)
	var assigned any
	if assignedTo != "" {
		assigned = assignedTo
	}
	err := testPool.QueryRow(context.Background(), `
		INSERT INTO leads (name, phone, phone_bidx, name_words, source, project_id, consent, assigned_to)
		VALUES ($1, $2, $3, $4, 'website', 'project-1', true, $5::uuid) RETURNING id::text`,
		pii.Seal(pii.Name, "Asha Sharma"), pii.Seal(pii.Phone, phone), pii.PhoneIndex(phone),
		pii.NameWordIndexes("Asha Sharma"), assigned).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	return id, phone
}

var slotCounter = 0

// newSlotAt creates an open one-hour slot for an executive at a given start.
func newSlotAt(t *testing.T, execID string, start time.Time) string {
	t.Helper()
	var id string
	err := testPool.QueryRow(context.Background(), `
		INSERT INTO visit_slots (project_id, executive_id, starts_at, ends_at)
		VALUES ('project-1', $1::uuid, $2, $3) RETURNING id::text`, execID, start, start.Add(time.Hour)).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// newSlot creates an open slot for an executive, days ahead, and returns its id.
func newSlot(t *testing.T, execID string) string {
	t.Helper()
	slotCounter++
	start := time.Now().Add(time.Duration(3+slotCounter) * 24 * time.Hour).Truncate(time.Hour)
	var id string
	err := testPool.QueryRow(context.Background(), `
		INSERT INTO visit_slots (project_id, executive_id, starts_at, ends_at)
		VALUES ('project-1', $1::uuid, $2, $3) RETURNING id::text`, execID, start, start.Add(time.Hour)).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// call sends a request through the real router.
func call(t *testing.T, method, path string, cookie *http.Cookie, body any) *httptest.ResponseRecorder {
	t.Helper()
	return serve(newJSONRequest(t, method, path, cookie, body))
}

func serve(req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func newJSONRequest(t *testing.T, method, path string, cookie *http.Cookie, body any) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	req.RemoteAddr = "10.0.0." + fmt.Sprint(time.Now().UnixNano()%250+1) + ":1234" // spread over IPs so rate limits do not interfere
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	return req
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("bad JSON %q: %v", rec.Body.String(), err)
	}
	return v
}

func wantStatus(t *testing.T, rec *httptest.ResponseRecorder, want int) {
	t.Helper()
	if rec.Code != want {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, want, rec.Body.String())
	}
}
