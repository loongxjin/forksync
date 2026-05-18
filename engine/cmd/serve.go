package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	syncpkg "github.com/loongxjin/forksync/engine/internal/sync"

	"github.com/loongxjin/forksync/engine/internal/agent"
	"github.com/loongxjin/forksync/engine/internal/agent/session"
	"github.com/loongxjin/forksync/engine/internal/config"
	"github.com/loongxjin/forksync/engine/internal/history"
	"github.com/loongxjin/forksync/engine/internal/logger"
	"github.com/loongxjin/forksync/engine/internal/notify"
	"github.com/loongxjin/forksync/engine/internal/repo"
	sched "github.com/loongxjin/forksync/engine/internal/scheduler"
	"github.com/loongxjin/forksync/engine/pkg/types"
	"github.com/spf13/cobra"
)

var serveInterval string

const defaultServeInterval = "30m"

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the ForkSync background service (scheduler)",
	Long: `Start the ForkSync background service that periodically syncs all managed repositories.
This is designed to be spawned by the Electron UI.

The service runs until interrupted (SIGINT/SIGTERM).`,
	RunE: runServe,
}

func init() {
	serveCmd.Flags().StringVar(&serveInterval, "interval", "", "sync interval (overrides config, e.g. 15m, 1h)")
	rootCmd.AddCommand(serveCmd)
}

// ServeStatus is the JSON status output for the serve command.
type ServeStatus struct {
	Running  bool   `json:"running"`
	Interval string `json:"interval"`
	Message  string `json:"message"`
}

func runServe(cmd *cobra.Command, args []string) error {
	cfg, cfgMgr := getSharedConfig()

	// Override interval from flag if provided
	// Make a copy to avoid mutating the shared config pointer.
	if serveInterval != "" && cfg != nil {
		cfgCopy := *cfg
		cfgCopy.Sync.DefaultInterval = serveInterval
		cfg = &cfgCopy
	}

	store, err := loadRepoStore()
	if err != nil {
		return err
	}

	// Create syncer with all dependencies
	syncer, cleanup := setupSyncer(cfg, cfgMgr, store)
	defer cleanup()

	// Create and start scheduler (nil notifier — notifications handled by Electron layer)
	scheduler := sched.NewScheduler(syncer, nil, cfg)

	// Ensure logger is closed on exit
	defer logger.Close()

	// Set up signal handling for graceful shutdown
	ctx, cancel := context.WithCancel(cmd.Context())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	// Determine interval string for output
	intervalStr := defaultServeInterval
	if cfg != nil && cfg.Sync.DefaultInterval != "" {
		intervalStr = cfg.Sync.DefaultInterval
	}

	// Output startup status
	if isJSON() {
		status := ServeStatus{
			Running:  true,
			Interval: intervalStr,
			Message:  "ForkSync service started",
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(status); err != nil {
			fmt.Fprintf(os.Stderr, "error encoding status: %v\n", err)
		}
	} else {
		outputText("🚀 ForkSync service started (interval: %s)", intervalStr)
		outputText("Press Ctrl+C to stop")
	}

	// Start scheduler (runs SyncAll immediately, then on interval)
	scheduler.Start(ctx)

	// Wait for signal
	<-sigCh
	cancel()

	outputText("Stopping ForkSync service...")
	scheduler.Stop()
	outputText("ForkSync service stopped.")

	return nil
}

// newSessionManager creates a session.Manager if agent auto-resolve is
// configured and an agent CLI is available. Returns nil when auto-resolve
// should not be attempted (no agent, disabled, etc.).
func newSessionManager(cfg *config.Config, cfgMgr *config.Manager) *session.Manager {
	if cfg == nil {
		return nil
	}

	// Only create session manager when conflict strategy is agent_resolve
	if cfg.Agent.ConflictStrategy != types.StrategyAgentResolve {
		return nil
	}

	preferred := cfg.Agent.Preferred
	reg := agent.NewRegistry(preferred)
	provider, err := reg.GetPreferred()
	if err != nil {
		logger.Debug("sync: no agent available for auto-resolve", "error", err)
		return nil
	}

	sessionsDir := sessionsDir(cfgMgr)
	sessionStore := session.NewSessionStore(sessionsDir)
	if initErr := sessionStore.Init(); initErr != nil {
		logger.Error("sync: failed to init session store", "error", initErr)
		return nil
	}

	return session.NewManager(sessionStore, provider)
}

// setupSyncer creates a fully configured Syncer with history store and session manager.
// It accepts an already-loaded store so the caller and syncer share the same instance.
// Returns the syncer, store, and a cleanup function that must be deferred.
func setupSyncer(cfg *config.Config, cfgMgr *config.Manager, store repo.Store) (*syncpkg.Syncer, func()) {
	syncer := syncpkg.NewSyncerFromConfig(cfg, store, cfgMgr.ConfigDir())

	// Set up history store
	var histCleanup func()
	histStore, err := history.NewStore(cfgMgr.ConfigDir())
	if err == nil {
		syncer.SetHistoryStore(histStore)
		histCleanup = func() { histStore.Close() }
	} else {
		histCleanup = func() {}
	}

	// Set up agent session manager for auto conflict resolution
	if mgr := newSessionManager(cfg, cfgMgr); mgr != nil {
		syncer.SetSessionManager(mgr)
	}

	// Set up notifier for background syncs (no frontend watching)
	if cfg != nil && cfg.Notification.Enabled {
		syncer.SetNotifier(notify.New())
	}

	return syncer, histCleanup
}
