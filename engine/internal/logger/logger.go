// Package logger provides file-based logging with daily log rotation using slog.
package logger

import (
	"fmt"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var (
	defaultLogger *slog.Logger
	rotateWriter  *dailyRotateWriter
	mu            sync.RWMutex
)

// Init creates the default file logger in the given directory.
// Safe to call multiple times; subsequent calls replace the logger.
func Init(dir string) error {
	w, err := newDailyRotateWriter(dir)
	if err != nil {
		return err
	}

	mu.Lock()
	defer mu.Unlock()
	if rotateWriter != nil {
		_ = rotateWriter.Close()
	}
	rotateWriter = w

	level := slog.LevelDebug
	if env := os.Getenv("FORKSYNC_LOG_LEVEL"); env != "" {
		switch strings.ToLower(env) {
		case "info":
			level = slog.LevelInfo
		case "warn", "warning":
			level = slog.LevelWarn
		case "error":
			level = slog.LevelError
		}
	}

	handler := slog.NewTextHandler(w, &slog.HandlerOptions{
		Level: level,
	})
	defaultLogger = slog.New(handler)
	slog.SetDefault(defaultLogger)
	return nil
}

// Close flushes and closes the current log file.
func Close() error {
	mu.Lock()
	defer mu.Unlock()
	if rotateWriter != nil {
		err := rotateWriter.Close()
		rotateWriter = nil
		defaultLogger = nil
		return err
	}
	return nil
}

// Info logs an informational message.
func Info(msg string, args ...any) {
	mu.RLock()
	lg := defaultLogger
	mu.RUnlock()
	if lg != nil {
		lg.Info(msg, args...)
	}
}

// Error logs an error message.
func Error(msg string, args ...any) {
	mu.RLock()
	lg := defaultLogger
	mu.RUnlock()
	if lg != nil {
		lg.Error(msg, args...)
	}
}

// Warn logs a warning message.
func Warn(msg string, args ...any) {
	mu.RLock()
	lg := defaultLogger
	mu.RUnlock()
	if lg != nil {
		lg.Warn(msg, args...)
	}
}

// Debug logs a debug message.
func Debug(msg string, args ...any) {
	mu.RLock()
	lg := defaultLogger
	mu.RUnlock()
	if lg != nil {
		lg.Debug(msg, args...)
	}
}

// StdLogger returns a standard library *log.Logger backed by the same file writer.
func StdLogger() *log.Logger {
	mu.RLock()
	rw := rotateWriter
	mu.RUnlock()
	if rw == nil {
		return log.Default()
	}
	return log.New(rw, "", 0)
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
		if err := w.file.Close(); err != nil {
			return fmt.Errorf("close old log file: %w", err)
		}
	}

	path := filepath.Join(w.dir, fmt.Sprintf("sync-%s.log", today))
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("open log file: %w", err)
	}
	w.file = f
	w.current = today
	return nil
}
