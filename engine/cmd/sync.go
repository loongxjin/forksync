package cmd

import (
	"fmt"

	"github.com/loongxjin/forksync/engine/internal/history"
	"github.com/loongxjin/forksync/engine/internal/logger"
	"github.com/loongxjin/forksync/engine/internal/notify"
	"github.com/loongxjin/forksync/engine/internal/repo"
	syncpkg "github.com/loongxjin/forksync/engine/internal/sync"
	"github.com/loongxjin/forksync/engine/pkg/types"
	"github.com/spf13/cobra"
)

var syncAll bool

var syncCmd = &cobra.Command{
	Use:   "sync [repo-name]",
	Short: "Sync fork repositories with their upstream",
	RunE:  runSync,
}

func init() {
	syncCmd.Flags().BoolVar(&syncAll, "all", false, "sync all managed repositories")
	rootCmd.AddCommand(syncCmd)
}

func runSync(cmd *cobra.Command, args []string) error {
	cfg, cfgMgr := getSharedConfig()

	store := repo.NewJSONStore(cfgMgr.ConfigDir())
	if err := store.Load(); err != nil {
		return fmt.Errorf("load repo store: %w", err)
	}

	syncer := syncpkg.NewSyncerFromConfig(cfg, store)

	// Set up notifier if enabled in config
	if cfg != nil && cfg.Notification.Enabled {
		syncer.SetNotifier(notify.NewNotifier(true))
	}

	// Set up history store
	histStore, err := history.NewStore(cfgMgr.ConfigDir())
	if err == nil {
		syncer.SetHistoryStore(histStore)
		defer histStore.Close()
	}

	// Set up agent session manager for auto conflict resolution
	if mgr := newSessionManager(cfg, cfgMgr); mgr != nil {
		syncer.SetSessionManager(mgr)
	}

	defer logger.Close()

	syncResults := make([]types.SyncResult, 0)

	if syncAll {
		results := syncer.SyncAll(cmd.Context())
		for _, r := range results {
			syncResults = append(syncResults, r.ToSyncResult())
		}
	} else {
		if len(args) == 0 {
			return fmt.Errorf("specify a repo name or use --all")
		}

		r, ok := store.GetByName(args[0])
		if !ok {
			return fmt.Errorf("repository %q not found", args[0])
		}

		result := syncer.SyncRepo(cmd.Context(), r)
		syncResults = append(syncResults, result.ToSyncResult())
	}

	if isJSON() {
		outputJSON(types.SyncData{Results: syncResults}, nil)
	} else {
		for _, r := range syncResults {
			switch r.Status {
			case types.RepoStatusUpToDate:
				if r.CommitsPulled > 0 {
					outputText("✅ %s: synced (%d commits pulled)", r.RepoName, r.CommitsPulled)
				} else {
					outputText("✅ %s: already up to date", r.RepoName)
				}
			case types.RepoStatusConflict:
				outputText("⚠️  %s: conflicts in %d files", r.RepoName, len(r.ConflictFiles))
				for _, f := range r.ConflictFiles {
					outputText("   - %s", f)
				}
			case types.RepoStatusError:
				outputText("❌ %s: %s", r.RepoName, r.ErrorMessage)
			case types.RepoStatusResolved:
				agent := r.AgentUsed
				if r.AgentResult != nil && r.AgentResult.AgentName != "" {
					agent = r.AgentResult.AgentName
				}
				outputText("🔄 %s: conflicts resolved by %s, awaiting confirmation", r.RepoName, agent)
				if r.AgentResult != nil && r.AgentResult.Summary != "" {
					outputText("   Summary: %s", r.AgentResult.Summary)
				}
				for _, f := range r.PendingConfirm {
					outputText("   - %s", f)
				}
			}
		}
	}

	return nil
}
