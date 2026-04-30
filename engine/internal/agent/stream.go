package agent

import (
	"encoding/json"
	"io"
	"sync"
	"time"

	"github.com/loongxjin/forksync/engine/internal/logger"
)

// StreamEventType identifies the kind of agent stream event.
type StreamEventType string

const (
	StreamEventStart  StreamEventType = "start"
	StreamEventStdout StreamEventType = "stdout"
	StreamEventStderr StreamEventType = "stderr"
	StreamEventTool   StreamEventType = "tool"
	StreamEventDone   StreamEventType = "done"
	StreamEventError  StreamEventType = "error"
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

	// ToolName is the name of the tool call (present in tool events).
	ToolName string `json:"name,omitempty"`

	// ToolPath is the path argument of the tool call (present in tool events).
	ToolPath string `json:"path,omitempty"`
}

// StreamWriter writes StreamEvents as NDJSON lines to an io.Writer.
type StreamWriter struct {
	mu  sync.Mutex
	enc *json.Encoder
}

// NewStreamWriter creates a new StreamWriter that writes to w.
func NewStreamWriter(w io.Writer) *StreamWriter {
	return &StreamWriter{enc: json.NewEncoder(w)}
}

// WriteEvent encodes ev as a single NDJSON line.
func (sw *StreamWriter) WriteEvent(ev StreamEvent) error {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	if err := sw.enc.Encode(ev); err != nil {
		logger.Warn("stream: failed to encode event", "type", ev.Type, "error", err)
		return err
	}
	logger.Debug("stream: wrote event", "type", ev.Type)
	return nil
}
