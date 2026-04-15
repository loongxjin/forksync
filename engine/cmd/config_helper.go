package cmd

import (
	"github.com/loongxjin/forksync/engine/internal/config"
	"github.com/loongxjin/forksync/engine/internal/logger"
)

var (
	sharedCfg    *config.Config
	sharedCfgMgr *config.Manager
)

// getSharedConfig returns the lazily-initialized shared config and config manager.
// This avoids creating a new config.Manager in every subcommand.
func getSharedConfig() (*config.Config, *config.Manager) {
	if sharedCfgMgr == nil {
		sharedCfgMgr = config.NewManager()
		var err error
		sharedCfg, err = sharedCfgMgr.Load()
		if err != nil {
			logger.Debug("config load skipped", "error", err)
		}
	}
	return sharedCfg, sharedCfgMgr
}
