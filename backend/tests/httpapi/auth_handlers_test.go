package httpapi_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"leadflow/internal/auth"
	"leadflow/internal/config"
	"leadflow/internal/httpapi"
	"leadflow/internal/models"
)

// ---- unit tests (no database) ----------------------------------------------

func TestClientIPIgnoresForwardingHeadersByDefault(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/api/auth/login", nil)
	r.RemoteAddr = "203.0.113.7:5555"
	r.Header.Set("X-Forwarded-For", "1.2.3.4")
	r.Header.Set("X-Real-IP", "5.6.7.8")
	r.Header.Set("True-Client-IP", "9.9.9.9")

	if got := httpapi.ClientIP(r, ""); got != "203.0.113.7" {
		t.Errorf("with no trusted proxy, headers must be ignored: got %s", got)
	}
}

func TestClientIPTrustsOnlyTheNamedHeader(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/", nil)
	r.RemoteAddr = "10.0.0.1:5555" // the proxy
	r.Header.Set("Fly-Client-IP", "198.51.100.4")
	r.Header.Set("X-Forwarded-For", "1.2.3.4, 198.51.100.9")

	if got := httpapi.ClientIP(r, "Fly-Client-IP"); got != "198.51.100.4" {
		t.Errorf("named header: got %s", got)
	}
	// For X-Forwarded-For only the entry the proxy itself added (the last) counts.
	if got := httpapi.ClientIP(r, "X-Forwarded-For"); got != "198.51.100.9" {
		t.Errorf("X-Forwarded-For: got %s, want the last entry", got)
	}
	r.Header.Set("Fly-Client-IP", "not-an-ip")
	if got := httpapi.ClientIP(r, "Fly-Client-IP"); got != "10.0.0.1" {
		t.Errorf("a garbage header must fall back to the connection address, got %s", got)
	}
}

func TestLimiterCannotBeResetByFlooding(t *testing.T) {
	l := httpapi.NewIPLimiter(1, time.Hour)
	if !l.Allow("victim-of-guessing") || l.Allow("victim-of-guessing") {
		t.Fatal("bucket of 1 should allow exactly one")
	}
	for i := 0; i < httpapi.MaxTrackedKeys+50; i++ {
		l.Allow(fmt.Sprintf("spoofed-%d", i))
	}
	if l.Allow("victim-of-guessing") {
		t.Fatal("flooding the limiter with new keys reset an exhausted key")
	}
}

func TestCookiesAreSecureUnlessDevelopment(t *testing.T) {
	if !(config.Config{}).SecureCookies() || !(config.Config{AppEnv: "production"}).SecureCookies() {
		t.Error("cookies must be Secure by default and in production")
	}
	if (config.Config{AppEnv: "development"}).SecureCookies() {
		t.Error("local development uses plain http")
	}
}

// ---- integration tests -----------------------------------------------------

// loginWith posts a login from a fixed client address.
func loginWith(t *testing.T, email, password, remote string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := newJSONRequest(t, http.MethodPost, "/api/auth/login", nil, map[string]string{"email": email, "password": password})
	req.RemoteAddr = remote
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return serve(req)
}

func TestLoginFailuresAllLookTheSame(t *testing.T) {
	requireDB(t)
	_, email := newUser(t, models.RoleExecutive)
	_, disabled := newUser(t, models.RoleExecutive)
	if _, err := testPool.Exec(context.Background(), `UPDATE users SET active = false WHERE email = $1`, disabled); err != nil {
		t.Fatal(err)
	}

	cases := map[string]*httptest.ResponseRecorder{
		"unknown email":            call(t, http.MethodPost, "/api/auth/login", nil, map[string]string{"email": "nobody-" + randHex(4) + "@test.local", "password": testPassword}),
		"wrong password":           call(t, http.MethodPost, "/api/auth/login", nil, map[string]string{"email": email, "password": "wrong-password"}),
		"disabled, right password": call(t, http.MethodPost, "/api/auth/login", nil, map[string]string{"email": disabled, "password": testPassword}),
	}
	for name, rec := range cases {
		if rec.Code != http.StatusUnauthorized || !strings.Contains(rec.Body.String(), "Incorrect email or password.") {
			t.Errorf("%s: got %d %s", name, rec.Code, rec.Body.String())
		}
	}
}

func TestSpoofedForwardedForDoesNotBypassLoginLimit(t *testing.T) {
	requireDB(t)
	remote := "192.0.2.77:4000"
	var last *httptest.ResponseRecorder
	for i := 0; i < 11; i++ {
		last = loginWith(t, "nobody-"+randHex(4)+"@test.local", "x-password", remote,
			map[string]string{"X-Forwarded-For": fmt.Sprintf("10.9.%d.%d", i, i), "X-Real-IP": fmt.Sprintf("10.8.0.%d", i)})
	}
	if last.Code != http.StatusTooManyRequests || !strings.Contains(last.Body.String(), "wait a minute") {
		t.Fatalf("11th attempt from one address should be limited whatever the headers say: %d %s", last.Code, last.Body.String())
	}
}

func TestAccountIsThrottledAcrossIPs(t *testing.T) {
	requireDB(t)
	_, email := newUser(t, models.RoleExecutive)
	for i := 0; i < 5; i++ {
		rec := loginWith(t, email, "wrong-password", fmt.Sprintf("198.18.%d.1:4000", i), nil)
		wantStatus(t, rec, http.StatusUnauthorized)
	}
	// From yet another address, even the RIGHT password waits now.
	rec := loginWith(t, email, testPassword, "198.18.99.1:4000", nil)
	if rec.Code != http.StatusTooManyRequests || !strings.Contains(rec.Body.String(), "this account") {
		t.Fatalf("6th attempt on the account should be throttled: %d %s", rec.Code, rec.Body.String())
	}
}

func TestLogoutEndsTheSessionOnTheServer(t *testing.T) {
	requireDB(t)
	_, cookie := newStaff(t, models.RoleExecutive)
	wantStatus(t, call(t, http.MethodGet, "/api/auth/me", cookie, nil), http.StatusOK)

	wantStatus(t, call(t, http.MethodPost, "/api/auth/logout", cookie, nil), http.StatusNoContent)
	// A copy of the old cookie no longer works.
	wantStatus(t, call(t, http.MethodGet, "/api/auth/me", cookie, nil), http.StatusUnauthorized)
}

func TestExpiredSessionIsRejected(t *testing.T) {
	requireDB(t)
	_, cookie := newStaff(t, models.RoleExecutive)
	if _, err := testPool.Exec(context.Background(), `UPDATE sessions SET expires_at = now() - interval '1 minute'
		WHERE token_hash = $1`, auth.HashToken(cookie.Value)); err != nil {
		t.Fatal(err)
	}
	wantStatus(t, call(t, http.MethodGet, "/api/auth/me", cookie, nil), http.StatusUnauthorized)
}

func TestDisabledUserLosesSessionAtOnce(t *testing.T) {
	requireDB(t)
	id, cookie := newStaff(t, models.RoleExecutive)
	if _, err := testPool.Exec(context.Background(), `UPDATE users SET active = false WHERE id = $1::uuid`, id); err != nil {
		t.Fatal(err)
	}
	wantStatus(t, call(t, http.MethodGet, "/api/auth/me", cookie, nil), http.StatusUnauthorized)
}

func TestSessionCookieFlags(t *testing.T) {
	requireDB(t)
	_, email := newUser(t, models.RoleExecutive)

	check := func(h http.Handler, wantSecure bool) {
		req := newJSONRequest(t, http.MethodPost, "/api/auth/login", nil, map[string]string{"email": email, "password": testPassword})
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		wantStatus(t, rec, http.StatusOK)
		for _, c := range rec.Result().Cookies() {
			if c.Name == auth.CookieName {
				if !c.HttpOnly || c.SameSite != http.SameSiteLaxMode || c.Secure != wantSecure {
					t.Errorf("cookie flags HttpOnly=%v SameSite=%v Secure=%v (want Secure=%v)", c.HttpOnly, c.SameSite, c.Secure, wantSecure)
				}
				return
			}
		}
		t.Error("no session cookie")
	}
	check(handler, false) // the test router runs as development
	check(httpapi.NewRouter(config.Config{AppEnv: "production"}, testPool, model, &background), true)
}

func TestAPIResponsesAreNotCached(t *testing.T) {
	requireDB(t)
	_, cookie := newStaff(t, models.RoleHead)
	rec := call(t, http.MethodGet, "/api/auth/me", cookie, nil)
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", got)
	}
}
