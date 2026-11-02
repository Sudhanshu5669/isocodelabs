package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// fakeStore is a stand-in for the database. It satisfies SessionStore because
// it has the UserBySession method.
type fakeStore struct {
	users map[string]User // keyed by the raw token, to keep the test readable
}

func (f fakeStore) UserBySession(_ context.Context, hash []byte) (User, error) {
	for token, u := range f.users {
		if string(HashToken(token)) == string(hash) {
			return u, nil
		}
	}
	return User{}, ErrNoSession
}

func TestPasswordRoundTrip(t *testing.T) {
	hash, err := HashPassword("correct horse")
	if err != nil {
		t.Fatal(err)
	}
	if hash == "correct horse" {
		t.Fatal("password must not be stored as plain text")
	}
	if !CheckPassword(hash, "correct horse") {
		t.Error("right password should match")
	}
	if CheckPassword(hash, "wrong") {
		t.Error("wrong password must not match")
	}
}

func TestSessionTokensAreUniqueAndHashed(t *testing.T) {
	a, ha, _ := NewSessionToken()
	b, _, _ := NewSessionToken()
	if a == b {
		t.Error("two tokens should never be equal")
	}
	if string(ha) != string(HashToken(a)) {
		t.Error("the stored hash must equal the hash of the token")
	}
	if len(a) < 40 {
		t.Errorf("token looks too short to be safe: %d chars", len(a))
	}
}

// protected builds a route chain exactly as the real router does.
func protected(store SessionStore, roles ...string) http.Handler {
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	var h http.Handler = ok
	if len(roles) > 0 {
		h = RequireRole(roles...)(h)
	}
	return Authenticate(store)(RequireAuth(h))
}

func TestAccessControl(t *testing.T) {
	store := fakeStore{users: map[string]User{
		"exec-token": {ID: "1", Role: RoleExecutive},
		"head-token": {ID: "2", Role: RoleHead},
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
		{"executive blocked from head-only route", "exec-token", []string{RoleHead}, http.StatusForbidden},
		{"head can use a head-only route", "head-token", []string{RoleHead}, http.StatusOK},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tc.cookie != "" {
				req.AddCookie(&http.Cookie{Name: CookieName, Value: tc.cookie})
			}
			rec := httptest.NewRecorder()
			protected(store, tc.roles...).ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Errorf("got status %d, want %d", rec.Code, tc.want)
			}
		})
	}
}
