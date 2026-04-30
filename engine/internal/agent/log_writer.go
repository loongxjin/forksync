package agent

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
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

// NewLogWriter creates a new LogWriter for the given repoID.
// The log file is created under <baseDir>/agent-logs/<repoID>/<YYYYMMDD-HHMMSS>.ndjson.
func NewLogWriter(baseDir, repoID string) (*LogWriter, error) {
	dir := filepath.Join(baseDir, agentLogDirName, sanitizeRepoID(repoID))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create agent log dir: %w", err)
	}

	ts := time.Now().Format("20060102-150405")
	path := filepath.Join(dir, ts+".ndjson")

	file, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("create log file: %w", err)
	}

	sw := NewStreamWriter(file)
	logger.Info("agent: created log writer", "repo", repoID, "path", path)
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

// LatestLogFile returns the path of the most recent log file for the given repoID.
func LatestLogFile(baseDir, repoID string) (string, error) {
	dir := filepath.Join(baseDir, agentLogDirName, sanitizeRepoID(repoID))
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("no logs found for repo %s", repoID)
		}
		return "", fmt.Errorf("read log dir: %w", err)
	}

	var files []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasSuffix(name, ".ndjson") {
			files = append(files, name)
		}
	}

	if len(files) == 0 {
		return "", fmt.Errorf("no log files found for repo %s", repoID)
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i] > files[j] // descending — newest first
	})

	return filepath.Join(dir, files[0]), nil
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
			logger.Debug("agent: skipping corrupted log line", "path", path, "error", err, "line", line)
			continue
		}
		events = append(events, ev)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read log file: %w", err)
	}

	return events, nil
}

// CleanupOldLogs removes log files older than maxAge for the given repoID.
func CleanupOldLogs(baseDir, repoID string, maxAge time.Duration) error {
	if maxAge <= 0 {
		maxAge = defaultLogRetention
	}

	dir := filepath.Join(baseDir, agentLogDirName, sanitizeRepoID(repoID))
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read log dir: %w", err)
	}

	cutoff := time.Now().Add(-maxAge)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			_ = os.Remove(filepath.Join(dir, e.Name()))
		}
	}

	return nil
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
