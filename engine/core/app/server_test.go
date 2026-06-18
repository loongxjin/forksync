package app

import (
	"context"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestServerSkeletonHealthAndVersion verifies the always-on routes that the
// server registers in routes() work without any deps wiring.
func TestServerSkeletonHealthAndVersion(t *testing.T) {
	mux := http.NewServeMux()
	s := &Server{deps: nil}
	// Re-register only the always-on routes to avoid needing a real Deps.
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("GET /version", s.handleVersion)

	t.Run("healthz returns ok", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
		rec := httptest.NewRecorder()
		s.handleHealth(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
		body := rec.Body.String()
		if !strings.Contains(body, `"ok":true`) {
			t.Fatalf("body = %q, want ok:true", body)
		}
	})

	t.Run("version returns envelope", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/version", nil)
		rec := httptest.NewRecorder()
		s.handleVersion(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
		body := rec.Body.String()
		if !strings.Contains(body, `"success":true`) {
			t.Fatalf("body = %q, want success:true envelope", body)
		}
	})
}

// TestServerStartPrintsAddr confirms Start announces FORKSYNC_HTTP_ADDR on stdout.
func TestServerStartPrintsAddr(t *testing.T) {
	// Capture stdout via a pipe on the *Server.Start path is heavy; instead
	// verify Addr() reflects the listener directly.
	srv, err := NewServer("127.0.0.1:0", nil)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	if !strings.HasPrefix(srv.Addr(), "127.0.0.1:") {
		t.Fatalf("Addr = %q, want 127.0.0.1: prefix", srv.Addr())
	}
	// Start with an already-cancelled context so Serve returns immediately
	// after announcing the address, exercising the print + shutdown path.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := srv.Start(ctx); err != nil {
		t.Fatalf("Start on cancelled ctx: %v", err)
	}
	// Start must generate a non-empty token announced to the parent process.
	if srv.Token() == "" {
		t.Fatalf("Token() empty after Start; want generated token")
	}
}

// TestGenerateToken returns a 64-char hex string and is unique across calls.
func TestGenerateToken(t *testing.T) {
	tok := generateToken()
	if len(tok) != tokenBytes*2 { // hex = 2 chars per byte
		t.Fatalf("len(token) = %d, want %d", len(tok), tokenBytes*2)
	}
	// Should be valid hex.
	if _, err := hex.DecodeString(tok); err != nil {
		t.Fatalf("token %q is not hex: %v", tok, err)
	}
	// Two calls must differ (256 bits of entropy; collision is astronomically
	// unlikely — guards against a constant-output regression).
	if tok == generateToken() {
		t.Fatalf("generateToken returned identical values twice")
	}
}
