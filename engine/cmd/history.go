package cmd

import (
	"fmt"
	"time"

	"github.com/loongxjin/forksync/engine/internal/config"
	"github.com/loongxjin/forksync/engine/internal/history"
	"github.com/loongxjin/forksync/engine/internal/repo"
	"github.com/loongxjin/forksync/engine/pkg/types"
	"github.com/spf13/cobra"
)

var (
	historyLimit    int
	historyCleanup  bool
	historyKeepDays int
)

var historyCmd = &cobra.Command{
	Use:   "history [repo-name]",
	Short: "Show sync history",
	Long:  `Show recent sync history for all repos or a specific repo. Use --cleanup to clear history.`,
	RunE:  runHistory,
}

func init() {
	historyCmd.Flags().IntVar(&historyLimit, "limit", 20, "number of records to show")
	historyCmd.Flags().BoolVar(&historyCleanup, "cleanup", false, "clean up sync history")
	historyCmd.Flags().IntVar(&historyKeepDays, "keep-days", 0, "keep records from last N days (0 = clear all)")
	rootCmd.AddCommand(historyCmd)
}

func runHistory(cmd *cobra.Command, args []string) error {
	cfgMgr := config.NewManager()
	cfgMgr.Load() // ignore error — history works without config

	store, err := history.NewStore(cfgMgr.ConfigDir())
	if err != nil {
		return fmt.Errorf("open history store: %w", err)
	}
	defer store.Close()

	// Handle cleanup mode
	if historyCleanup {
		return runHistoryCleanup(cmd, args, store, cfgMgr)
	}

	var records []history.Record

	if len(args) > 0 {
		// Look up repo by name to get ID
		repoStore := repo.NewJSONStore(cfgMgr.ConfigDir())
		if loadErr := repoStore.Load(); loadErr != nil {
			return fmt.Errorf("load repo store: %w", loadErr)
		}
		r, ok := repoStore.GetByName(args[0])
		if !ok {
			return fmt.Errorf("repository %q not found", args[0])
		}
		records, err = store.ByRepo(r.ID, historyLimit)
	} else {
		records, err = store.Recent(historyLimit)
	}
	if err != nil {
		return fmt.Errorf("query history: %w", err)
	}

	if isJSON() {
		result := make([]types.SyncHistoryRecord, 0, len(records))
		for _, r := range records {
			result = append(result, types.SyncHistoryRecord{
				ID:             r.ID,
				RepoID:         r.RepoID,
				RepoName:       r.RepoName,
				Status:         r.Status,
				CommitsPulled:  r.CommitsPulled,
				ConflictFiles:  r.ConflictFiles,
				AgentUsed:      r.AgentUsed,
				ConflictsFound: r.ConflictsFound,
				AutoResolved:   r.AutoResolved,
				ErrorMessage:   r.ErrorMessage,
				CreatedAt:      r.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
			})
		}
		outputJSON(types.HistoryData{Records: result}, nil)
	} else {
		if len(records) == 0 {
			outputText("No sync history found.")
			return nil
		}
		for _, r := range records {
			icon := "✅"
			switch r.Status {
			case "conflict":
				icon = "⚠️"
			case "error":
				icon = "❌"
			case "up_to_date":
				icon = "—"
			}
			outputText("%s %s  %s  (%d commits)", icon, r.CreatedAt.Format("2006-01-02 15:04"), r.RepoName, r.CommitsPulled)
			if r.ErrorMessage != "" {
				outputText("   Error: %s", r.ErrorMessage)
			}
			if len(r.ConflictFiles) > 0 {
				outputText("   Conflicts: %d files", len(r.ConflictFiles))
			}
			if r.AgentUsed != "" {
				outputText("   Agent: %s (resolved %d)", r.AgentUsed, r.AutoResolved)
			}
		}
	}

	return nil
}

func runHistoryCleanup(cmd *cobra.Command, args []string, store *history.Store, cfgMgr *config.Manager) error {
	var err error
	var msg string
	var n int64

	if len(args) > 0 {
		// Clean up specific repo
		repoStore := repo.NewJSONStore(cfgMgr.ConfigDir())
		if loadErr := repoStore.Load(); loadErr != nil {
			return fmt.Errorf("load repo store: %w", loadErr)
		}
		r, ok := repoStore.GetByName(args[0])
		if !ok {
			return fmt.Errorf("repository %q not found", args[0])
		}
		n, err = store.ClearByRepo(r.ID)
		msg = fmt.Sprintf("Cleared %d history record(s) for repository %q", n, args[0])
	} else if historyKeepDays > 0 {
		// Clean up by date
		before := time.Now().AddDate(0, 0, -historyKeepDays)
		n, err = store.ClearBefore(before)
		msg = fmt.Sprintf("Cleared %d history record(s) older than %d days", n, historyKeepDays)
	} else {
		// Clean up all
		n, err = store.ClearAll()
		msg = fmt.Sprintf("Cleared %d history record(s)", n)
	}

	if err != nil {
		return fmt.Errorf("cleanup failed: %w", err)
	}

	if isJSON() {
		outputJSON(map[string]string{"message": msg}, nil)
	} else {
		outputText(msg)
	}
	return nil
}
