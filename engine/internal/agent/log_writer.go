package agent

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/loongxjin/forksync/engine/internal/logger"
)

const (
	// agentLogDirName is the subdirectory under the user's config dir where agent logs are stored.
	agentLogDirName = "agent-logs"

	// defaultLogRetention is the default max age for old log files.
	defaultLogRetention = 7 * 24 * time.Hour
)

// LogWriter persists agent stream events to an NDJSON file on disk.
type LogWriter struct {
	file *os.File
	sw   *StreamWriter
	path string
}

// NewLogWriter creates a new LogWriter for the given repoID + resolve session.
// The log file is created under <baseDir>/agent-logs/<repoID>/<sessionID>.ndjson.
// Naming by session id (instead of a timestamp) means each resolve run gets a
// stable, unique file the frontend can locate precisely — replacing the old
// "newest file in the dir" lookup that suffered stale-read pollution across
// resolves.
func NewLogWriter(baseDir, repoID, sessionID string) (*LogWriter, error) {
	dir := filepath.Join(baseDir, agentLogDirName, sanitizeRepoID(repoID))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create agent log dir: %w", err)
	}

	path := filepath.Join(dir, sanitizeRepoID(sessionID)+".ndjson")

	file, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("create log file: %w", err)
	}

	sw := NewStreamWriter(file)
	logger.Debug("agent: created log writer", "repo", repoID, "session", sessionID, "path", path)
	return &LogWriter{file: file, sw: sw, path: path}, nil
}

// WriteEvent writes a stream event to the log file.
func (lw *LogWriter) WriteEvent(ev StreamEvent) error {
	if err := lw.sw.WriteEvent(ev); err != nil {
		logger.Warn("agent: failed to write log event", "path", lw.path, "type", ev.Type, "error", err)
		return err
	}
	return nil
}

// Close closes the underlying log file.
func (lw *LogWriter) Close() error {
	if lw.file != nil {
		logger.Debug("agent: closing log writer", "path", lw.path)
		return lw.file.Close()
	}
	return nil
}

// StreamWriter returns the underlying StreamWriter for direct use.
func (lw *LogWriter) StreamWriter() *StreamWriter {
	return lw.sw
}

// nopWriter is an io.Writer that discards everything — the fallback when a
// resolve run has neither a live sink nor a usable disk log.
type nopWriter struct{}

func (nopWriter) Write(p []byte) (int, error) { return len(p), nil }

// NewResolveLogWriter creates the disk-log arm of a resolve run's stream fan-out.
// It returns a StreamWriter (always non-nil) and a cleanup func (always safe to
// defer). On disk-open failure it logs a warning and returns a no-op writer so
// callers (the interactive resolve path in app, the auto-sync path in sync)
// never have to nil-check. This is the single shared log-writer setup; callers
// that also want a live sink wrap the result in a MultiStreamWriter themselves.
// sessionID names the log file so the frontend can locate it precisely.
func NewResolveLogWriter(baseDir, repoID, sessionID string) (*StreamWriter, func()) {
	lw, err := NewLogWriter(baseDir, repoID, sessionID)
	if err != nil {
		logger.Warn("agent: failed to create resolve log writer", "repo", repoID, "session", sessionID, "error", err)
		return NewStreamWriter(nopWriter{}), func() {}
	}
	return lw.StreamWriter(), func() { _ = lw.Close() }
}

// LogFile returns the path of the log file for a specific resolve session.
// This replaces the old LatestLogFile "newest file in the dir" lookup, which
// could return a stale log from a previous resolve. With session-named files,
// the exact file is addressed directly.
func LogFile(baseDir, repoID, sessionID string) (string, error) {
	dir := filepath.Join(baseDir, agentLogDirName, sanitizeRepoID(repoID))
	path := filepath.Join(dir, sanitizeRepoID(sessionID)+".ndjson")
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("no log found for repo %s session %s", repoID, sessionID)
		}
		return "", fmt.Errorf("stat log file: %w", err)
	}
	return path, nil
}

// ReadLogFile parses all StreamEvents from an NDJSON log file.
func ReadLogFile(path string) ([]StreamEvent, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open log file: %w", err)
	}
	defer file.Close()

	var events []StreamEvent
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		var ev StreamEvent
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			// Skip corrupted lines rather than failing entirely.
			logger.Warn("agent: skipping corrupted log line", "path", path, "error", err, "line", line)
			continue
		}
		events = append(events, ev)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read log file: %w", err)
	}

	return events, nil
}

// sanitizeRepoID makes a repoID safe for use as a directory name.
func sanitizeRepoID(repoID string) string {
	// Replace path separators and other risky characters.
	replacer := strings.NewReplacer(
		"/", "_",
		"\\", "_",
		":", "_",
		"..", "_",
	)
	return replacer.Replace(repoID)
}
