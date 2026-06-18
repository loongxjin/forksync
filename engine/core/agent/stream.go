package agent

import (
	"bufio"
	"encoding/json"
	"io"
	"sync"
	"time"

	"github.com/loongxjin/forksync/engine/core/logger"
)

// StreamEventType identifies the kind of agent stream event.
type StreamEventType string

const (
	StreamEventStart          StreamEventType = "start"
	StreamEventStdout         StreamEventType = "stdout"
	StreamEventStderr         StreamEventType = "stderr"
	StreamEventTool           StreamEventType = "tool"
	StreamEventDone           StreamEventType = "done"
	StreamEventError          StreamEventType = "error"
	StreamEventStatePersisted StreamEventType = "state_persisted"
)

// StreamEvent is a single line of NDJSON output emitted during agent resolution.
type StreamEvent struct {
	// Type is the event type: start, stdout, stderr, tool, done, error.
	Type StreamEventType `json:"t"`

	// Data is the raw text for stdout/stderr/error events.
	Data string `json:"d,omitempty"`

	// Agent is the provider name (present in start events).
	Agent string `json:"agent,omitempty"`

	// Files is the list of conflict files (present in start events).
	Files []string `json:"files,omitempty"`

	// Timestamp is the ISO-8601 timestamp of the event.
	Timestamp time.Time `json:"ts"`

	// Success indicates whether resolution succeeded (present in done/error).
	Success bool `json:"success,omitempty"`

	// Summary is a truncated summary (present in done events).
	Summary string `json:"summary,omitempty"`

	// SessionID is the session identifier (present in done events).
	SessionID string `json:"session_id,omitempty"`

	// ResolvedFiles lists file paths the agent modified (present in done events).
	ResolvedFiles []string `json:"resolvedFiles,omitempty"`

	// Diff is the staged diff produced by the agent (present in done events).
	Diff string `json:"diff,omitempty"`

	// AgentName is the provider name (present in done events, complements the
	// per-event Agent field which only appears in start events).
	AgentName string `json:"agentName,omitempty"`

	// ToolName is the name of the tool call (present in tool events).
	ToolName string `json:"name,omitempty"`

	// ToolPath is the path argument of the tool call (present in tool events).
	ToolPath string `json:"path,omitempty"`
}

// StreamWriter writes StreamEvents as NDJSON lines to an io.Writer.
// Each WriteEvent immediately flushes so downstream consumers (e.g. Electron
// readline) see the event without delay.
type StreamWriter struct {
	mu    sync.Mutex
	w     *bufio.Writer
	enc   *json.Encoder
	multi *MultiStreamWriter // if set, delegates WriteEvent here instead
}

// NewStreamWriter creates a new StreamWriter that writes to w.
// It wraps w in a bufio.Writer and flushes after every event.
func NewStreamWriter(w io.Writer) *StreamWriter {
	bw := bufio.NewWriter(w)
	return &StreamWriter{
		w:   bw,
		enc: json.NewEncoder(bw),
	}
}

// WriteEvent encodes ev as a single NDJSON line and flushes immediately.
func (sw *StreamWriter) WriteEvent(ev StreamEvent) error {
	// Delegate to MultiStreamWriter if configured
	if sw.multi != nil {
		return sw.multi.WriteEvent(ev)
	}
	sw.mu.Lock()
	defer sw.mu.Unlock()
	if err := sw.enc.Encode(ev); err != nil {
		logger.Warn("stream: failed to encode event", "type", ev.Type, "error", err)
		return err
	}
	if err := sw.w.Flush(); err != nil {
		logger.Warn("stream: failed to flush", "type", ev.Type, "error", err)
		return err
	}
	return nil
}

// MultiStreamWriter fans out every WriteEvent to multiple StreamWriters.
// Used when --stream mode needs to write to both os.Stdout (for real-time
// Electron consumption) and a disk log file (for later replay).
type MultiStreamWriter struct {
	writers []*StreamWriter
}

// NewMultiStreamWriter creates a MultiStreamWriter that duplicates events
// to all given writers.
func NewMultiStreamWriter(writers ...*StreamWriter) *MultiStreamWriter {
	return &MultiStreamWriter{writers: writers}
}

// WriteEvent writes the event to every underlying writer.
// Errors from individual writers are logged but do not stop the others.
func (msw *MultiStreamWriter) WriteEvent(ev StreamEvent) error {
	var firstErr error
	for _, w := range msw.writers {
		if err := w.WriteEvent(ev); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// StreamWriter returns a *StreamWriter that delegates WriteEvent calls to
// this MultiStreamWriter. This allows MultiStreamWriter to be used anywhere
// a *StreamWriter is expected (e.g. as a parameter to ResolveConflicts).
func (msw *MultiStreamWriter) StreamWriter() *StreamWriter {
	return &StreamWriter{multi: msw}
}

// DoneEventFromResult builds the authoritative terminal "done" StreamEvent
// from an AgentResult, copying all enriched fields (ResolvedFiles, Diff,
// AgentName, Summary, SessionID). Both the manual resolve handler and the
// sync auto-resolve path call this so their disk logs end with a proper
// terminal frame — ensuring readAgentLog reports isRunning=false.
func DoneEventFromResult(r *AgentResult) StreamEvent {
	ev := StreamEvent{
		Type:      StreamEventDone,
		Success:   true,
		Timestamp: time.Now().UTC(),
	}
	if r != nil {
		ev.Summary = r.Summary
		ev.SessionID = r.SessionID
		ev.ResolvedFiles = r.ResolvedFiles
		ev.Diff = r.Diff
		ev.AgentName = r.AgentName
	}
	return ev
}
