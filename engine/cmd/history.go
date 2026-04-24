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
	_, cfgMgr := getSharedConfig()

	store, err := history.NewStore(cfgMgr.ConfigDir())
	if err != nil {
		return fmt.Errorf("open history store: %w", err)
	}
	defer store.Close()

	// Handle cleanup mode
	if historyCleanup {
		return runHistoryCleanup(cmd, args, store, cfgMgr)
	}

	var dbRecords []history.Record

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
		dbRecords, err = store.ByRepo(r.ID, historyLimit)
	} else {
		dbRecords, err = store.Recent(historyLimit)
	}
	if err != nil {
		return fmt.Errorf("query history: %w", err)
	}

	if isJSON() {
		records := make([]types.SyncHistoryRecord, 0, len(dbRecords))
		for _, r := range dbRecords {
			records = append(records, types.SyncHistoryRecord{
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
				Summary:        r.Summary,
				SummaryStatus:  r.SummaryStatus,
				CreatedAt:      r.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
			})
		}
		outputJSON(types.HistoryData{Records: records}, nil)
	} else {
		if len(dbRecords) == 0 {
			outputText("No sync history found.")
			return nil
		}
		for _, rec := range dbRecords {
			icon := "✅"
			switch rec.Status {
			case string(types.RepoStatusConflict):
				icon = "⚠️"
			case string(types.RepoStatusError):
				icon = "❌"
			case string(types.RepoStatusUpToDate):
				icon = "—"
			}
			outputText("%s %s  %s  (%d commits)", icon, rec.CreatedAt.Format("2006-01-02 15:04"), rec.RepoName, rec.CommitsPulled)
			if rec.ErrorMessage != "" {
				outputText("   Error: %s", rec.ErrorMessage)
			}
			if len(rec.ConflictFiles) > 0 {
				outputText("   Conflicts: %d files", len(rec.ConflictFiles))
			}
			if rec.AgentUsed != "" {
				outputText("   Agent: %s (resolved %d)", rec.AgentUsed, rec.AutoResolved)
			}
			if rec.Summary != "" {
				outputText("   📝 %s", rec.Summary)
			} else if rec.SummaryStatus == string(types.SummaryStatusGenerating) {
				outputText("   🤖 AI summary generating...")
			} else if rec.SummaryStatus == string(types.SummaryStatusFailed) {
				outputText("   ❌ AI summary generation failed")
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
