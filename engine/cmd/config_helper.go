package cmd

import (
	"path/filepath"
	"sync"

	"github.com/loongxjin/forksync/engine/internal/config"
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

// getSharedConfig returns the lazily-initialized shared config and config manager.
// This avoids creating a new config.Manager in every subcommand.
func getSharedConfig() (*config.Config, *config.Manager) {
	configOnce.Do(func() {
		sharedCfgMgr = config.NewManager()
		sharedCfg, configErr = sharedCfgMgr.Load()
		if configErr != nil {
			logger.Debug("config load skipped", "error", configErr)
		}
	})
	return sharedCfg, sharedCfgMgr
}
