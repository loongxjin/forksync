package cmd

import (
	"path/filepath"

	"github.com/loongxjin/forksync/engine/internal/agent"
	"github.com/loongxjin/forksync/engine/internal/agent/session"
	"github.com/loongxjin/forksync/engine/internal/config"
	"github.com/loongxjin/forksync/engine/internal/logger"
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
