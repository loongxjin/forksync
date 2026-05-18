package cmd

import (
	"fmt"
	"path/filepath"
	"sync"

	"github.com/loongxjin/forksync/engine/internal/config"
	"github.com/loongxjin/forksync/engine/internal/git"
	"github.com/loongxjin/forksync/engine/internal/logger"
	"github.com/loongxjin/forksync/engine/internal/repo"
	"github.com/loongxjin/forksync/engine/pkg/types"
)

var (
	sharedCfg    *config.Config
	sharedCfgMgr *config.Manager
	configOnce   sync.Once
	configErr    error
)

// sessionsDir returns the path to the sessions directory within the config dir.
func sessionsDir(cfgMgr *config.Manager) string {
	return filepath.Join(cfgMgr.ConfigDir(), "sessions")
}

// updateRepoWithLog updates the repo in the store and logs any error.
func updateRepoWithLog(r types.Repo, store repo.Store, action string) {
	if updateErr := store.Update(r); updateErr != nil {
		logger.Error("failed to update repo", "repo", r.Name, "action", action, "error", updateErr)
	}
}

// newGitOps creates a git.Operations instance with proxy support if configured.
func newGitOps(cfg *config.Config) git.OperationsProvider {
	if cfg != nil && cfg.Proxy.Enabled && cfg.Proxy.URL != "" {
		return git.NewOperationsWithProxy(cfg.Proxy.URL)
	}
	return git.NewOperations()
}

// getSharedConfig returns the lazily-initialized shared config and config manager.
// This avoids creating a new config.Manager in every subcommand.
func getSharedConfig() (*config.Config, *config.Manager) {
	configOnce.Do(func() {
		sharedCfgMgr = config.NewManager()
		sharedCfg, configErr = sharedCfgMgr.Load()
		if configErr != nil {
			logger.Warn("config load skipped", "error", configErr)
		}
	})
	return sharedCfg, sharedCfgMgr
}

func loadRepoStore() (repo.Store, error) {
	_, cfgMgr := getSharedConfig()
	store := repo.NewJSONStore(cfgMgr.ConfigDir())
	if err := store.Load(); err != nil {
		return nil, fmt.Errorf("load repo store: %w", err)
	}
	return store, nil
}
