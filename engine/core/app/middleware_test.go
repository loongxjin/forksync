package app

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// okHandler is a no-op 200 handler used to verify the middleware passes
// requests through when auth succeeds.
var okHandler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
})

func TestAuthTokenEmptyTokenIsPassThrough(t *testing.T) {
	h := authToken(okHandler, "")
	req := httptest.NewRequest(http.MethodPost, "/sync/all", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200 (empty token = pass-through)", rec.Code)
	}
}

func TestAuthTokenHealthzExempt(t *testing.T) {
	h := authToken(okHandler, "secret")
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /healthz code = %d, want 200 (exempt)", rec.Code)
	}
}

func TestAuthTokenVersionExempt(t *testing.T) {
	h := authToken(okHandler, "secret")
	req := httptest.NewRequest(http.MethodGet, "/version", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /version code = %d, want 200 (exempt)", rec.Code)
	}
}

func TestAuthTokenRejectsMissingHeader(t *testing.T) {
	h := authToken(okHandler, "secret")
	req := httptest.NewRequest(http.MethodPost, "/sync/all", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("code = %d, want 401", rec.Code)
	}
}

func TestAuthTokenRejectsWrongToken(t *testing.T) {
	h := authToken(okHandler, "secret")
	req := httptest.NewRequest(http.MethodPost, "/sync/all", nil)
	req.Header.Set("Authorization", "Bearer wrong")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("code = %d, want 401", rec.Code)
	}
}

func TestAuthTokenAcceptsCorrectBearer(t *testing.T) {
	h := authToken(okHandler, "secret")
	req := httptest.NewRequest(http.MethodPost, "/sync/all", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
}

func TestAuthTokenWSRouteAcceptsQueryToken(t *testing.T) {
	h := authToken(okHandler, "secret")
	// Browsers cannot set WS headers; the token must come via ?token=.
	req := httptest.NewRequest(http.MethodGet, "/stream/resolve/repo1?token=secret", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("WS route code = %d, want 200", rec.Code)
	}
}

func TestAuthTokenWSRouteRejectsMissingQueryToken(t *testing.T) {
	h := authToken(okHandler, "secret")
	// A WS route with no ?token= (or a wrong one) must be rejected even if a
	// bearer header is present — the header is stripped by the browser on WS.
	req := httptest.NewRequest(http.MethodGet, "/stream/resolve/repo1", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("WS route without ?token= code = %d, want 401", rec.Code)
	}
}

func TestConstantTimeEquals(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"abc", "abc", true},
		{"abc", "abd", false},
		{"abc", "abcd", false},
		{"", "", true},
		{"abc", "", false},
	}
	for _, c := range cases {
		if got := constantTimeEquals(c.a, c.b); got != c.want {
			t.Errorf("constantTimeEquals(%q,%q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}
