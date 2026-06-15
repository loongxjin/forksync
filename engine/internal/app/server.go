// Package app hosts the embedded HTTP server that replaces the old Cobra CLI.
//
// The Electron main process spawns a single long-lived `forksync` process which
// runs this server on 127.0.0.1:<random-port>. All engine capabilities that
// were previously Cobra subcommands are exposed as REST endpoints (plus one
// WebSocket endpoint for streaming agent resolve output).
package app

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/loongxjin/forksync/engine/internal/config"
	"github.com/loongxjin/forksync/engine/internal/logger"
	"github.com/loongxjin/forksync/engine/pkg/version"
)

// addrReadyPrefix is printed to stdout (single line) once the server is
// listening, so the Electron parent can discover the bound port:
//
//	FORKSYNC_HTTP_ADDR=127.0.0.1:54321
const addrReadyPrefix = "FORKSYNC_HTTP_ADDR="

// Server is the embedded HTTP server. It owns a *Deps and an *http.Server.
type Server struct {
	deps   *Deps
	server *http.Server
	ln     net.Listener

	stopScheduler func()
	mu            sync.Mutex
	started       bool
}

// NewServer constructs a Server bound to addr. If addr is empty or ":0", the
// server binds to a kernel-chosen port on 127.0.0.1.
func NewServer(addr string, deps *Deps) (*Server, error) {
	if addr == "" || addr == ":0" {
		addr = "127.0.0.1:0"
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("listen %s: %w", addr, err)
	}

	mux := http.NewServeMux()
	s := &Server{
		deps: deps,
		ln:   ln,
		server: &http.Server{
			Handler:           mux,
			ReadHeaderTimeout: 10 * time.Second,
		},
	}
	s.routes(mux)
	return s, nil
}

// Addr returns the actual address the server is listening on.
func (s *Server) Addr() string { return s.ln.Addr().String() }

// Start serves HTTP until ctx is done or Shutdown is called. It prints the
// FORKSYNC_HTTP_ADDR line to stdout exactly once so the Electron parent can
// discover the port.
func (s *Server) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return errors.New("server already started")
	}
	s.started = true
	if s.deps != nil {
		s.stopScheduler = s.deps.StartScheduler(ctx)
	}
	s.mu.Unlock()

	// Announce the address to the parent process.
	fmt.Fprintf(os.Stdout, "%s%s\n", addrReadyPrefix, s.Addr())

	serveErr := make(chan error, 1)
	go func() { serveErr <- s.server.Serve(s.ln) }()

	logger.Info("app: http server listening", "addr", s.Addr())

	select {
	case <-ctx.Done():
		return s.Shutdown(context.Background())
	case err := <-serveErr:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

// Shutdown gracefully stops the HTTP server and the background scheduler.
func (s *Server) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	if stop := s.stopScheduler; stop != nil {
		stop()
		s.stopScheduler = nil
	}
	s.mu.Unlock()
	return s.server.Shutdown(ctx)
}

// routes registers all HTTP routes on the given mux.
func (s *Server) routes(mux *http.ServeMux) {
	// Health & version (always available).
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("GET /version", s.handleVersion)
}

// handleHealth is the readiness probe used by the Electron parent at startup.
func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true}`))
}

// handleVersion mirrors `forksync version`.
func (s *Server) handleVersion(w http.ResponseWriter, _ *http.Request) {
	writeOK(w, map[string]string{
		"version":  version.Version,
		"commit":   version.Commit,
		"builtAt":  version.BuildDate,
	})
}

// Run builds deps, starts the server on addr, and blocks on SIGINT/SIGTERM.
// This is the entry point called by main.go. It initializes the logger and
// closes it on exit.
func Run(addr string) error {
	logDir, err := resolveLogDir()
	if err != nil {
		// non-fatal: logger.Init will fall back to stderr-ish behavior internally
		logger.Warn("app: could not resolve log dir", "error", err)
	} else if err := logger.Init(logDir); err != nil {
		logger.Warn("app: logger init failed", "error", err)
	}
	defer logger.Close()

	deps, err := BuildDeps()
	if err != nil {
		return fmt.Errorf("build deps: %w", err)
	}
	defer deps.Close()

	srv, err := NewServer(addr, deps)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		logger.Info("app: shutdown signal received")
		cancel()
	}()

	return srv.Start(ctx)
}

// resolveLogDir returns <configDir>/logs for the logger, mirroring the old
// rootCmd PersistentPreRunE behavior.
func resolveLogDir() (string, error) {
	mgr := config.NewManager()
	return mgr.ConfigDir() + "/logs", nil
}
