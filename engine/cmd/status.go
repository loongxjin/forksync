package cmd

import (
	"fmt"

	"github.com/loongxjin/forksync/engine/internal/config"
	"github.com/loongxjin/forksync/engine/internal/git"
	"github.com/loongxjin/forksync/engine/internal/repo"
	"github.com/loongxjin/forksync/engine/pkg/types"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show status of all managed repositories",
	RunE:  runStatus,
}

func init() {
	rootCmd.AddCommand(statusCmd)
}

func runStatus(cmd *cobra.Command, args []string) error {
	cfgMgr := config.NewManager()
	if _, err := cfgMgr.Load(); err != nil {
		// Non-fatal
	}

	store := repo.NewJSONStore(cfgMgr.ConfigDir())
	if err := store.Load(); err != nil {
		return fmt.Errorf("load repo store: %w", err)
	}

	repos, err := store.List()
	if err != nil {
		return fmt.Errorf("list repos: %w", err)
	}

	gitOps := git.NewOperations()

	// Update ahead/behind for each repo
	for i, r := range repos {
		if r.Status != types.RepoStatusSyncing {
			statusResult, err := gitOps.Status(cmd.Context(), r)
			if err == nil && statusResult != nil {
				repos[i].AheadBy = statusResult.AheadBy
				repos[i].BehindBy = statusResult.BehindBy
				if repos[i].Status == types.RepoStatusUnconfigured && r.Upstream != "" {
					repos[i].Status = types.RepoStatusSynced
				}
			}
		}
	}

	if isJSON() {
		outputJSON(types.StatusData{Repos: repos}, nil)
	} else {
		if len(repos) == 0 {
			outputText("No repositories managed. Use 'forksync add <path>' to add one.")
			return nil
		}

		outputText("Managed Repositories (%d):", len(repos))
		outputText("")
		for _, r := range repos {
			statusIcon := "⚪"
			switch r.Status {
			case types.RepoStatusSynced:
				statusIcon = "🟢"
			case types.RepoStatusSyncing:
				statusIcon = "🟡"
			case types.RepoStatusConflict:
				statusIcon = "🔴"
			case types.RepoStatusError:
				statusIcon = "❌"
			}

			outputText("  %s %s", statusIcon, r.Name)
			if r.Upstream != "" {
				outputText("     Upstream: %s", r.Upstream)
			}
			if r.BehindBy > 0 {
				outputText("     Behind by %d commits", r.BehindBy)
			}
			if r.AheadBy > 0 {
				outputText("     Ahead by %d commits", r.AheadBy)
			}
			if r.ErrorMessage != "" {
				outputText("     Error: %s", r.ErrorMessage)
			}
		}
	}

	return nil
}
