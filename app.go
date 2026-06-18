package main

import (
	"context"
	"time"

	"github.com/loongxjin/forksync/engine/core/app"
	"github.com/loongxjin/forksync/engine/core/config"
	"github.com/loongxjin/forksync/engine/core/logger"
	syncpkg "github.com/loongxjin/forksync/engine/core/sync"
	"github.com/loongxjin/forksync/engine/pkg/types"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App is the Wails application struct. It owns the engine dependencies that
// were previously wired in engine/core/app/server.go and exposes them as
// bound methods to the React frontend.
type App struct {
	ctx    context.Context
	deps   *app.Deps
	cfgMgr *config.Manager
}

// NewApp creates an App. Deps are built lazily in startup().
func NewApp() *App {
	return &App{}
}

// startup is called once at Wails startup.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	// Init engine logger. The log directory is resolved relative to the
	// config dir (~/.forksync/logs), matching the old HTTP server behavior.
	cfgMgr := config.NewManager()
	a.cfgMgr = cfgMgr
	logDir := resolveLogDir(cfgMgr)
	if err := logger.Init(logDir); err != nil {
		logger.Warn("app: logger init failed", "error", err)
	}

	deps, err := app.BuildDeps()
	if err != nil {
		logger.Error("app: failed to build dependencies", "error", err)
		return
	}
	a.deps = deps

	// Start background scheduler if sync_on_startup is configured.
	if deps.Cfg != nil && deps.Cfg.Sync.SyncOnStartup {
		deps.StartScheduler(ctx)
	}

	// Bridge engine eventbus to Wails Events for push-based state updates.
	if deps.Bus != nil {
		ch, subCancel := deps.Bus.Subscribe()
		go func() {
			defer subCancel()
			for ev := range ch {
					runtime.EventsEmit(ctx, "engine:event", string(ev.Type))
			}
		}()
	}

	logger.Info("app: wails started")
}

// shutdown is called on application exit.
func (a *App) shutdown(_ context.Context) {
	if a.deps != nil {
		a.deps.Close()
	}
	logger.Close()
}

// resolveLogDir returns the log directory under the config dir.
func resolveLogDir(cfgMgr *config.Manager) string {
	return cfgMgr.ConfigDir() + "/logs"
}

// --------------------------------------------------------------------
// Bound methods — one per engine capability, replacing HTTP handlers.
// Each returns (result, error); Wails serializes to JSON automatically.
// --------------------------------------------------------------------

const statusTimeout = 30 * time.Second

// Status returns the current engine status (repos, sync state, agents).
// Mirrors the old GET /status handler.
func (a *App) Status(exclude []string) (types.StatusData, error) {
	if a.deps == nil {
		return types.StatusData{}, nil
	}

	ctx, cancel := context.WithTimeout(a.ctx, statusTimeout)
	defer cancel()

	repos, err := a.deps.Store.List()
	if err != nil {
		return types.StatusData{}, err
	}

	refresher := syncpkg.NewStatusRefresher(a.deps.GitOps, a.deps.RawStore(), a.deps.Cfg)
	repos, err = refresher.RefreshAll(ctx, repos, exclude)
	if err != nil {
		return types.StatusData{}, err
	}

	agents := a.deps.AgentRegistry.Discover()
	preferredAgent := ""
	if len(agents) > 0 {
		preferredAgent = agents[0].Name
	}

	return types.StatusData{
		Repos:          repos,
		Agents:         agents,
		PreferredAgent: preferredAgent,
	}, nil
}

// Greet exists for smoke-testing the Wails binding.
func (a *App) Greet(name string) string {
	if a.deps == nil {
		return "engine not ready"
	}
	return "ForkSync engine ready, " + name
}
