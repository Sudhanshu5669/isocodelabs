package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSameOrigin(t *testing.T) {
	allowed := []string{"http://localhost:5173"}
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	handler := sameOrigin(allowed)(ok)

	tests := []struct {
		name   string
		method string
		origin string
		want   int
	}{
		{"GET is never blocked", http.MethodGet, "https://evil.example", http.StatusOK},
		{"POST without Origin (non-browser client)", http.MethodPost, "", http.StatusOK},
		{"POST from the same host", http.MethodPost, "http://app.example.com", http.StatusOK},
		{"POST from the allow-listed dev origin", http.MethodPost, "http://localhost:5173", http.StatusOK},
		{"POST from another site is blocked", http.MethodPost, "https://evil.example", http.StatusForbidden},
		{"POST with a malformed Origin is blocked", http.MethodPost, "::::", http.StatusForbidden},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, "http://app.example.com/api/x", nil)
			if tc.origin != "" {
				req.Header.Set("Origin", tc.origin)
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Errorf("got %d, want %d", rec.Code, tc.want)
			}
		})
	}
}

func TestLoginRequestValidation(t *testing.T) {
	tests := []struct {
		name string
		req  loginRequest
		ok   bool
	}{
		{"valid", loginRequest{Email: "a@b.co", Password: "secret"}, true},
		{"empty email", loginRequest{Email: "", Password: "secret"}, false},
		{"not an email", loginRequest{Email: "nope", Password: "secret"}, false},
		{"empty password", loginRequest{Email: "a@b.co", Password: ""}, false},
		{"password longer than bcrypt allows", loginRequest{Email: "a@b.co", Password: string(make([]byte, 73))}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.req.validate() == ""; got != tc.ok {
				t.Errorf("validate ok = %v, want %v", got, tc.ok)
			}
		})
	}
}
