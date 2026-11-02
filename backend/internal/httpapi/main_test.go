package httpapi

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"leadflow/internal/auth"
	"leadflow/internal/config"
	"leadflow/internal/testdb"
)

// Integration tests for the HTTP layer, run against a throw-away schema in a
// real Postgres (see package testdb). Without DATABASE_URL they skip themselves.

var (
	testPool *pgxpool.Pool
	handler  http.Handler
)

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
	if err != nil {
		fmt.Println("integration tests disabled:", err)
		return m.Run()
	}
	if pool != nil {
		testPool = pool
		handler = NewRouter(config.Config{AppEnv: "development"}, pool)
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
		INSERT INTO users (email, name, role, password_hash) VALUES ($1, 'Test User', $2, $3) RETURNING id::text`,
		email, role, hash).Scan(&id)
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
	phone = "+91" + fmt.Sprintf("%010d", time.Now().UnixNano()%10000000000)
	var assigned any
	if assignedTo != "" {
		assigned = assignedTo
	}
	err := testPool.QueryRow(context.Background(), `
		INSERT INTO leads (name, phone, source, project_id, consent, assigned_to)
		VALUES ('Asha Sharma', $1, 'website', 'project-1', true, $2::uuid) RETURNING id::text`, phone, assigned).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	return id, phone
}

var slotCounter = 0

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
