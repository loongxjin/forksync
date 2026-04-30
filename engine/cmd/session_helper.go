package cmd

import (
	"path/filepath"

	"github.com/loongxjin/forksync/engine/internal/agent"
	"github.com/loongxjin/forksync/engine/internal/agent/session"
	"github.com/loongxjin/forksync/engine/internal/config"
	"github.com/loongxjin/forksync/engine/internal/history"
	"github.com/loongxjin/forksync/engine/internal/logger"
	"github.com/loongxjin/forksync/engine/internal/notify"
	"github.com/loongxjin/forksync/engine/internal/repo"
	syncpkg "github.com/loongxjin/forksync/engine/internal/sync"
	"github.com/loongxjin/forksync/engine/pkg/types"
)

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

	sessionsDir := filepath.Join(cfgMgr.ConfigDir(), "sessions")
	sessionStore := session.NewSessionStore(sessionsDir)
	if initErr := sessionStore.Init(); initErr != nil {
		logger.Warn("sync: failed to init session store", "error", initErr)
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

	return syncer, histCleanup
}

// setupSyncerWithNotifier creates a fully configured Syncer with all dependencies
// including the notifier.
func setupSyncerWithNotifier(cfg *config.Config, cfgMgr *config.Manager, store repo.Store) (*syncpkg.Syncer, func()) {
	syncer, cleanup := setupSyncer(cfg, cfgMgr, store)

	// Set up notifier if enabled in config
	if cfg != nil && cfg.Notification.Enabled {
		syncer.SetNotifier(notify.New())
	}

	return syncer, cleanup
}
