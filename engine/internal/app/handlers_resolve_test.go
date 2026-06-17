package app

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/loongxjin/forksync/engine/internal/agent"
)

// TestBuildResolveStreamWriter_NilSinkAndDiskFailure covers Critical bug #3:
// when the live sink is nil AND NewLogWriter fails, the returned cleanup must
// be safe to call (no nil-pointer dereference on the unopened LogWriter).
//
// /dev/null is a file, so MkdirAll(<file>/agent-logs/<repo>) must fail.
func TestBuildResolveStreamWriter_NilSinkAndDiskFailure(t *testing.T) {
	sw, cleanup := buildResolveStreamWriter("/dev/null", "repo-x", "sess-x", nil)
	// cleanup must not panic even though lw is nil.
	defer cleanup()

	// A nil sink + failed disk returns a no-op writer (not nil), so callers
	// don't have to nil-check.
	if sw == nil {
		t.Fatal("expected non-nil writer even when both sink and disk are unavailable")
	}
	if err := sw.WriteEvent(agent.StreamEvent{Type: agent.StreamEventStart}); err != nil {
		t.Fatalf("WriteEvent on nop writer: %v", err)
	}
}

// TestBuildResolveStreamWriter_SinkOnlyWhenDiskFails confirms the live sink
// still receives events when the disk writer cannot be opened (the bug used to
// drop them or panic on cleanup).
func TestBuildResolveStreamWriter_SinkOnlyWhenDiskFails(t *testing.T) {
	var got []agent.StreamEvent
	sink := func(ev agent.StreamEvent) { got = append(got, ev) }

	sw, cleanup := buildResolveStreamWriter("/dev/null", "repo-y", "sess-y", sink)
	defer cleanup()

	ev := agent.StreamEvent{Type: agent.StreamEventStdout, Data: "hello"}
	if err := sw.WriteEvent(ev); err != nil {
		t.Fatalf("WriteEvent: %v", err)
	}
	if len(got) != 1 || got[0].Type != agent.StreamEventStdout || got[0].Data != "hello" {
		t.Fatalf("sink did not receive the event: %+v", got)
	}
}

// TestSinkWriter_MalformedJSONForwardsAsStdout covers Important bug #4: a line
// that cannot be decoded as a StreamEvent must be forwarded as a raw stdout
// event, not silently dropped.
func TestSinkWriter_MalformedJSONForwardsAsStdout(t *testing.T) {
	var got []agent.StreamEvent
	sw := &sinkWriter{fn: func(ev agent.StreamEvent) { got = append(got, ev) }}

	// Valid StreamEvent JSON round-trips as the typed event.
	if _, err := sw.Write([]byte(`{"t":"start","ts":"2026-01-01T00:00:00Z"}`)); err != nil {
		t.Fatal(err)
	}
	// Malformed JSON is forwarded as stdout (trailing newline trimmed).
	if _, err := sw.Write([]byte("not-json-at-all\n")); err != nil {
		t.Fatal(err)
	}

	if len(got) != 2 {
		t.Fatalf("expected 2 forwarded events, got %d", len(got))
	}
	if got[0].Type != agent.StreamEventStart {
		t.Errorf("first event type = %q, want start", got[0].Type)
	}
	if got[1].Type != agent.StreamEventStdout || got[1].Data != "not-json-at-all" {
		t.Errorf("malformed line forwarded as %+v, want stdout/not-json-at-all", got[1])
	}
}

// TestLimitBody_RejectsOversizedBody confirms the global body cap is enforced:
// a handler wrapped by limitBody observes a *http.MaxBytesError when a client
// sends more than maxBytes.
func TestLimitBody_RejectsOversizedBody(t *testing.T) {
	var readErr error
	inner := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		// Drain the body; MaxBytesReader surfaces the cap as a Read error.
		_, readErr = io.Copy(io.Discard, r.Body)
	})
	h := limitBody(inner, 8)

	// Body well under the limit → handler reads it cleanly.
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("short"))
	h.ServeHTTP(httptest.NewRecorder(), req)
	if readErr != nil {
		t.Fatalf("small body: unexpected read error: %v", readErr)
	}

	// Body over the limit → Read returns *http.MaxBytesError.
	req = httptest.NewRequest(http.MethodPost, "/", strings.NewReader("this is way more than eight bytes"))
	h.ServeHTTP(httptest.NewRecorder(), req)
	if readErr == nil {
		t.Fatal("oversized body: expected a read error, got nil")
	}
	var mbe *http.MaxBytesError
	if !errors.As(readErr, &mbe) {
		t.Fatalf("oversized body: expected *http.MaxBytesError, got %T: %v", readErr, readErr)
	}
	if mbe.Limit != 8 {
		t.Errorf("MaxBytesError.Limit = %d, want 8", mbe.Limit)
	}
}
