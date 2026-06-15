package app

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/loongxjin/forksync/engine/internal/agent"
	"github.com/loongxjin/forksync/engine/internal/agent/session"
	"github.com/loongxjin/forksync/engine/internal/config"
	"github.com/loongxjin/forksync/engine/internal/git"
	"github.com/loongxjin/forksync/engine/internal/history"
	"github.com/loongxjin/forksync/engine/internal/logger"
	"github.com/loongxjin/forksync/engine/internal/repo"
	respkg "github.com/loongxjin/forksync/engine/internal/resolve"
	sched "github.com/loongxjin/forksync/engine/internal/scheduler"
	syncpkg "github.com/loongxjin/forksync/engine/internal/sync"
	"github.com/loongxjin/forksync/engine/pkg/types"
)

// Deps holds the long-lived, shared dependencies constructed once at server
// startup. Every HTTP handler reads from a single *Deps instance — this is the
// HTTP-era replacement for cmd/config_helper.go's package-level singletons.
type Deps struct {
	Cfg     *config.Config
	CfgMgr  *config.Manager
	Store   repo.Store
	GitOps  git.OperationsProvider
	Syncer  *syncpkg.Syncer
	Resolve *respkg.Resolver

	// HistStore is the long-lived history DB handle. May be nil if init failed.
	HistStore *history.Store
	// AgentRegistry is configured with the user's preferred agent.
	AgentRegistry *agent.Registry
	// SessionMgr drives agent sessions; nil unless agent_resolve strategy is on.
	SessionMgr *session.Manager

	// configDir is cached to avoid repeated lookups on ConfigMgr.
	configDir string

	histCleanup func()
}

// BuildDeps constructs a Deps instance by loading config, stores, syncer and
// resolver exactly once. This is the single construction point that replaces
// the old cmd/getSharedConfig / loadRepoStore / setupSyncer / newGitOps
// helpers (which lived in cmd/config_helper.go and cmd/serve.go).
func BuildDeps() (*Deps, error) {
	cfgMgr := config.NewManager()
	cfg, err := cfgMgr.Load()
	if err != nil {
		// Mirror the old behavior: a missing/broken config is non-fatal; we run
		// with a nil cfg and the handlers degrade gracefully.
		logger.Warn("app: config load skipped", "error", err)
	}

	store := repo.NewJSONStore(cfgMgr.ConfigDir())
	if err := store.Load(); err != nil {
		return nil, fmt.Errorf("load repo store: %w", err)
	}

	deps := &Deps{
		Cfg:        cfg,
		CfgMgr:     cfgMgr,
		Store:      store,
		GitOps:     newGitOps(cfg),
		configDir:  cfgMgr.ConfigDir(),
		histCleanup: func() {},
	}

	// History store (SQLite). Optional — handlers tolerate nil.
	if histStore, err := history.NewStore(cfgMgr.ConfigDir()); err == nil {
		deps.HistStore = histStore
		deps.histCleanup = func() { histStore.Close() }
	} else {
		logger.Warn("app: history store init failed", "error", err)
	}

	// Agent registry (preferred agent from config, if any).
	preferred := ""
	if cfg != nil {
		preferred = cfg.Agent.Preferred
	}
	deps.AgentRegistry = agent.NewRegistry(preferred)

	// Session manager — only when agent_resolve is configured AND an agent is
	// available (mirrors cmd/serve.go newSessionManager).
	if cfg != nil && cfg.Agent.ConflictStrategy == types.StrategyAgentResolve {
		provider, perr := deps.AgentRegistry.GetPreferred()
		if perr == nil {
			sessionStore := session.NewSessionStore(deps.SessionsDir())
			if initErr := sessionStore.Init(); initErr != nil {
				logger.Error("app: failed to init session store", "error", initErr)
			} else {
				deps.SessionMgr = session.NewManager(sessionStore, provider)
			}
		} else {
			logger.Debug("app: no agent available for auto-resolve", "error", perr)
		}
	}

	// Syncer with the same option wiring as cmd/serve.go setupSyncer.
	var syncOpts []syncpkg.Option
	if deps.HistStore != nil {
		syncOpts = append(syncOpts, syncpkg.WithHistoryStore(deps.HistStore))
	}
	if deps.SessionMgr != nil {
		syncOpts = append(syncOpts, syncpkg.WithSessionManager(deps.SessionMgr))
	}
	if cfg != nil && cfg.Notification.Enabled {
		// notifications are surfaced by the Electron layer; background syncs
		// still emit desktop notifications when enabled in config.
		// (A nil notifier is passed for the scheduler path below.)
	}
	deps.Syncer = syncpkg.NewSyncerFromConfig(cfg, store, cfgMgr.ConfigDir(), syncOpts...)

	// Resolver reuses the same gitOps/store/cfg/sessionMgr.
	deps.Resolve = respkg.NewResolver(deps.GitOps, store, cfg, cfgMgr, deps.SessionMgr)

	return deps, nil
}

// Close releases all long-lived resources. Safe to call once at shutdown.
func (d *Deps) Close() {
	if d.histCleanup != nil {
		d.histCleanup()
	}
}

// ConfigDir returns the ForkSync config directory (~/.forksync).
func (d *Deps) ConfigDir() string { return d.configDir }

// SessionsDir returns the agent sessions directory under the config dir.
func (d *Deps) SessionsDir() string {
	return filepath.Join(d.configDir, "sessions")
}

// StartScheduler launches the background sync scheduler if sync_on_startup is
// enabled in config. Returns a stop function (no-op if not started).
func (d *Deps) StartScheduler(ctx context.Context) (stop func()) {
	if d.Cfg == nil || !d.Cfg.Sync.SyncOnStartup {
		return func() {}
	}
	scheduler := sched.NewScheduler(d.Syncer, nil, d.Cfg)
	scheduler.Start(ctx)
	logger.Info("app: scheduler started")
	return func() { scheduler.Stop() }
}

// newGitOps creates a git.Operations instance with proxy support if configured.
// Mirrors cmd/config_helper.go newGitOps.
func newGitOps(cfg *config.Config) git.OperationsProvider {
	if cfg != nil && cfg.Proxy.Enabled && cfg.Proxy.URL != "" {
		return git.NewOperationsWithProxy(cfg.Proxy.URL)
	}
	return git.NewOperations()
}

// updateRepoWithLog updates the repo in the store and logs any error.
// Mirrors cmd/config_helper.go updateRepoWithLog.
func updateRepoWithLog(r types.Repo, store repo.Store, action string) {
	if updateErr := store.Update(r); updateErr != nil {
		logger.Error("failed to update repo", "repo", r.Name, "action", action, "error", updateErr)
	}
}
