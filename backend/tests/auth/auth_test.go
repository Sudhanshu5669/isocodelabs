package auth_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"leadflow/internal/auth"
)

// fakeStore is a stand-in for the database. It satisfies SessionStore because
// it has the UserBySession method.
type fakeStore struct {
	users map[string]auth.User // keyed by the raw token, to keep the test readable
}

func (f fakeStore) UserBySession(_ context.Context, hash []byte) (auth.User, error) {
	for token, u := range f.users {
		if string(auth.HashToken(token)) == string(hash) {
			return u, nil
		}
	}
	return auth.User{}, auth.ErrNoSession
}

func TestPasswordRoundTrip(t *testing.T) {
	hash, err := auth.HashPassword("correct horse")
	if err != nil {
		t.Fatal(err)
	}
	if hash == "correct horse" {
		t.Fatal("password must not be stored as plain text")
	}
	if !auth.CheckPassword(hash, "correct horse") {
		t.Error("right password should match")
	}
	if auth.CheckPassword(hash, "wrong") {
		t.Error("wrong password must not match")
	}
}

func TestSessionTokensAreUniqueAndHashed(t *testing.T) {
	a, ha, _ := auth.NewSessionToken()
	b, _, _ := auth.NewSessionToken()
	if a == b {
		t.Error("two tokens should never be equal")
	}
	if string(ha) != string(auth.HashToken(a)) {
		t.Error("the stored hash must equal the hash of the token")
	}
	if len(a) < 40 {
		t.Errorf("token looks too short to be safe: %d chars", len(a))
	}
}

// protected builds a route chain exactly as the real router does.
func protected(store auth.SessionStore, roles ...string) http.Handler {
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	var h http.Handler = ok
	if len(roles) > 0 {
		h = auth.RequireRole(roles...)(h)
	}
	return auth.Authenticate(store)(auth.RequireAuth(h))
}

func TestAccessControl(t *testing.T) {
	store := fakeStore{users: map[string]auth.User{
		"exec-token": {ID: "1", Role: auth.RoleExecutive},
		"head-token": {ID: "2", Role: auth.RoleHead},
	}}

	tests := []struct {
		name   string
		cookie string
		roles  []string
		want   int
	}{
		{"no cookie is rejected", "", nil, http.StatusUnauthorized},
		{"unknown token is rejected", "forged", nil, http.StatusUnauthorized},
		{"executive can use a signed-in route", "exec-token", nil, http.StatusOK},
		{"executive blocked from head-only route", "exec-token", []string{auth.RoleHead}, http.StatusForbidden},
		{"head can use a head-only route", "head-token", []string{auth.RoleHead}, http.StatusOK},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tc.cookie != "" {
				req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: tc.cookie})
			}
			rec := httptest.NewRecorder()
			protected(store, tc.roles...).ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Errorf("got status %d, want %d", rec.Code, tc.want)
			}
		})
	}
}
