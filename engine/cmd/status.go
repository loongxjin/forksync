package cmd

import (
	"fmt"

	"github.com/loongxjin/forksync/engine/internal/agent"
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

	// Update ahead/behind for each repo and refresh stale conflict statuses
	for i, r := range repos {
		if r.Status != types.RepoStatusSyncing {
			// For repos in conflict/resolving/resolved state, re-check the actual
			// git merge state. If the user has manually resolved and committed,
			// the stored status is stale and should be corrected.
			if r.Status == types.RepoStatusConflict || r.Status == types.RepoStatusResolving || r.Status == types.RepoStatusResolved {
				isMerging, unmergedFiles, err := gitOps.IsMergingState(cmd.Context(), r.Path)
				if err == nil && !isMerging {
					// No merge in progress — conflicts were resolved externally
					repos[i].Status = types.RepoStatusSynced
					repos[i].ErrorMessage = ""
					_ = store.Update(repos[i])
				} else if err == nil && isMerging && len(unmergedFiles) == 0 {
					// MERGE_HEAD exists but no unmerged files — user staged all
					// resolutions but hasn't committed yet. Still in conflict state
					// but update the status to reflect this transitional state.
					repos[i].Status = types.RepoStatusResolved
					_ = store.Update(repos[i])
				}
			}

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

	// Detect installed agents
	registry := agent.NewRegistry("")
	agents := registry.Discover()

	// Determine preferred agent
	preferredAgent := ""
	if len(agents) > 0 {
		preferredAgent = agents[0].Name
	}

	if isJSON() {
		outputJSON(types.StatusData{
			Repos:          repos,
			Agents:         agents,
			PreferredAgent: preferredAgent,
		}, nil)
	} else {
		if len(repos) == 0 {
			outputText("No repositories managed. Use 'forksync add <path>' to add one.")
		} else {
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

		// Show agent detection
		if len(agents) > 0 {
			outputText("")
			outputText("Agents detected: %s", preferredAgent)
		} else {
			outputText("")
			outputText("No AI agents detected. Install Claude Code, OpenCode, Droid, or Codex for auto-conflict resolution.")
		}
	}

	return nil
}
