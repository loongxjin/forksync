package cmd

import (
	"sync"

	"github.com/loongxjin/forksync/engine/internal/config"
	"github.com/loongxjin/forksync/engine/internal/logger"
)

var (
	sharedCfg    *config.Config
	sharedCfgMgr *config.Manager
	configOnce   sync.Once
	configErr    error
)

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
