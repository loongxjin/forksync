package app

import (
	"net/http"
	"strings"
)

// authToken is HTTP middleware that enforces the per-session bearer token on
// every request except the always-on readiness probes (/healthz, /version),
// which the Electron parent polls before it has read the token off stdout.
//
// Browsers cannot set headers on a WebSocket handshake, so for WS upgrade
// routes (any path under /stream/) the token is accepted via a ?token= query
// parameter instead of the Authorization header. All other requests must carry
// `Authorization: Bearer <token>`.
//
// If token is empty (engine started without one — should not happen in normal
// operation), the middleware allows everything through rather than lock out
// the local user.
func authToken(h http.Handler, token string) http.Handler {
	if token == "" {
		return h
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isPublicPath(r) {
			h.ServeHTTP(w, r)
			return
		}
		if !tokenValid(r, token) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"success":false,"error":"unauthorized"}`))
			return
		}
		h.ServeHTTP(w, r)
	})
}

// isPublicPath reports whether r is an always-on readiness probe that must be
// reachable before the parent has exchanged the token (Electron's /healthz
// poll at startup, and /version for the boot handshake).
func isPublicPath(r *http.Request) bool {
	if r.Method != http.MethodGet {
		return false
	}
	return r.URL.Path == "/healthz" || r.URL.Path == "/version"
}

// tokenValid checks the Authorization: Bearer header for normal requests, or
// the ?token= query param for WebSocket upgrade routes (which can't set
// headers during the browser handshake).
func tokenValid(r *http.Request, expected string) bool {
	// WebSocket upgrades: browsers cannot attach headers to the WS handshake,
	// so accept the token from the query string for any /stream/ route.
	if strings.HasPrefix(r.URL.Path, "/stream/") {
		if got := r.URL.Query().Get("token"); got != "" && constantTimeEquals(got, expected) {
			return true
		}
		return false
	}
	// Normal HTTP: require `Authorization: Bearer <token>`.
	auth := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if !strings.HasPrefix(auth, prefix) {
		return false
	}
	return constantTimeEquals(strings.TrimPrefix(auth, prefix), expected)
}

// constantTimeEquals compares two strings without short-circuiting, to avoid
// timing side-channels on token comparison. The local-server threat model is
// modest, but this is cheap and correct.
func constantTimeEquals(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	var diff byte
	for i := 0; i < len(a); i++ {
		diff |= a[i] ^ b[i]
	}
	return diff == 0
}
