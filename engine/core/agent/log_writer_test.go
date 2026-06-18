package agent

import (
	"testing"
)

// TestNewResolveLogWriter_Success covers the happy path: a usable baseDir
// yields a non-nil StreamWriter whose events reach disk, plus a safe cleanup.
func TestNewResolveLogWriter_Success(t *testing.T) {
	sw, cleanup := NewResolveLogWriter(t.TempDir(), "repo-1", "sess-1")
	defer cleanup()

	if sw == nil {
		t.Fatal("expected non-nil StreamWriter")
	}
	// WriteEvent must not error on a freshly opened log.
	if err := sw.WriteEvent(StreamEvent{Type: StreamEventStart, Data: "go"}); err != nil {
		t.Fatalf("WriteEvent: %v", err)
	}
}

// TestNewResolveLogWriter_DiskFailureFallsBackToNop confirms the helper's
// contract shared by the interactive resolve path and the auto-sync path:
// when the disk log cannot be opened, callers still get a non-nil writer and
// a cleanup that is safe to call. This is what lets both paths avoid nil-checks.
func TestNewResolveLogWriter_DiskFailureFallsBackToNop(t *testing.T) {
	// /dev/null is a file, so MkdirAll(<file>/agent-logs/<repo>) must fail.
	sw, cleanup := NewResolveLogWriter("/dev/null", "repo-x", "sess-x")
	defer cleanup() // must not panic

	if sw == nil {
		t.Fatal("expected non-nil (nop) StreamWriter on disk failure")
	}
	if err := sw.WriteEvent(StreamEvent{Type: StreamEventStart}); err != nil {
		t.Fatalf("WriteEvent on nop writer: %v", err)
	}
}
