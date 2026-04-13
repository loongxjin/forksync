// Package logger provides file-based logging with daily log rotation.
// Logs are written to ~/.forksync/logs/sync-YYYY-MM-DD.log.
package logger

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Logger writes log entries to daily-rotated files.
type Logger struct {
	mu      sync.Mutex
	dir     string
	current string // current date string (YYYY-MM-DD)
	file    *os.File
}

// New creates a new Logger that writes to the given directory.
func New(dir string) (*Logger, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create log directory: %w", err)
	}
	return &Logger{dir: dir}, nil
}

// logFilePath returns the log file path for the given time.
func (l *Logger) logFilePath(t time.Time) string {
	return filepath.Join(l.dir, fmt.Sprintf("sync-%s.log", t.Format("2006-01-02")))
}

// writeTo writes a line to the log file, rotating if the date has changed.
func (l *Logger) writeTo(line string) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	today := now.Format("2006-01-02")

	// Rotate if date changed or file not open
	if l.current != today || l.file == nil {
		if l.file != nil {
			l.file.Close()
		}
		path := l.logFilePath(now)
		f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			return fmt.Errorf("open log file: %w", err)
		}
		l.file = f
		l.current = today
	}

	_, err := fmt.Fprintln(l.file, line)
	return err
}

// Info logs an informational message with timestamp.
func (l *Logger) Info(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	line := fmt.Sprintf("[%s] INFO  %s", time.Now().Format("15:04:05"), msg)
	_ = l.writeTo(line)
}

// Error logs an error message with timestamp.
func (l *Logger) Error(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	line := fmt.Sprintf("[%s] ERROR %s", time.Now().Format("15:04:05"), msg)
	_ = l.writeTo(line)
}

// Warn logs a warning message with timestamp.
func (l *Logger) Warn(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	line := fmt.Sprintf("[%s] WARN  %s", time.Now().Format("15:04:05"), msg)
	_ = l.writeTo(line)
}

// Close closes the current log file.
func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.file != nil {
		err := l.file.Close()
		l.file = nil
		l.current = ""
		return err
	}
	return nil
}

// Writer returns an io.Writer that writes to the log file.
// Each Write call produces one or more log lines.
func (l *Logger) Writer() io.Writer {
	return &logWriter{logger: l}
}

type logWriter struct {
	logger *Logger
}

func (w *logWriter) Write(p []byte) (n int, err error) {
	line := string(p)
	_ = w.logger.writeTo(line)
	return len(p), nil
}
