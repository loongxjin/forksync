// Package logger provides file-based logging with daily log rotation using slog.
package logger

import (
	"fmt"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"
)

var (
	defaultLogger *slog.Logger
	rotateWriter  *dailyRotateWriter
)

// Init creates the default file logger in the given directory.
// Safe to call multiple times; subsequent calls replace the logger.
func Init(dir string) error {
	w, err := newDailyRotateWriter(dir)
	if err != nil {
		return err
	}
	rotateWriter = w

	handler := slog.NewTextHandler(w, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})
	defaultLogger = slog.New(handler)
	slog.SetDefault(defaultLogger)
	return nil
}

// Close flushes and closes the current log file.
func Close() error {
	if rotateWriter != nil {
		return rotateWriter.Close()
	}
	return nil
}

// Info logs an informational message.
func Info(msg string, args ...any) {
	if defaultLogger != nil {
		defaultLogger.Info(msg, args...)
	}
}

// Error logs an error message.
func Error(msg string, args ...any) {
	if defaultLogger != nil {
		defaultLogger.Error(msg, args...)
	}
}

// Warn logs a warning message.
func Warn(msg string, args ...any) {
	if defaultLogger != nil {
		defaultLogger.Warn(msg, args...)
	}
}

// Debug logs a debug message.
func Debug(msg string, args ...any) {
	if defaultLogger != nil {
		defaultLogger.Debug(msg, args...)
	}
}

// StdLogger returns a standard library *log.Logger backed by the same file writer.
func StdLogger() *log.Logger {
	if rotateWriter == nil {
		return log.Default()
	}
	return log.New(rotateWriter, "", 0)
}

// dailyRotateWriter writes to a log file that rotates daily.
type dailyRotateWriter struct {
	mu      sync.Mutex
	dir     string
	current string // YYYY-MM-DD
	file    *os.File
}

func newDailyRotateWriter(dir string) (*dailyRotateWriter, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create log directory: %w", err)
	}
	w := &dailyRotateWriter{dir: dir}
	if err := w.rotateIfNeeded(); err != nil {
		return nil, err
	}
	return w, nil
}

func (w *dailyRotateWriter) Write(p []byte) (n int, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if err := w.rotateIfNeeded(); err != nil {
		return 0, err
	}
	return w.file.Write(p)
}

func (w *dailyRotateWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.file != nil {
		err := w.file.Close()
		w.file = nil
		w.current = ""
		return err
	}
	return nil
}

func (w *dailyRotateWriter) rotateIfNeeded() error {
	today := time.Now().Format("2006-01-02")
	if w.current == today && w.file != nil {
		return nil
	}

	if w.file != nil {
		w.file.Close()
	}

	path := filepath.Join(w.dir, fmt.Sprintf("sync-%s.log", today))
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("open log file: %w", err)
	}
	w.file = f
	w.current = today
	return nil
}
