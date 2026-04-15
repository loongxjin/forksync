package cmd

import (
	"fmt"
	"sync"

	"github.com/loongxjin/forksync/engine/internal/agent"
	"github.com/loongxjin/forksync/engine/internal/git"
	"github.com/loongxjin/forksync/engine/internal/logger"
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
	cfg, cfgMgr := getSharedConfig()

	store := repo.NewJSONStore(cfgMgr.ConfigDir())
	if err := store.Load(); err != nil {
		return fmt.Errorf("load repo store: %w", err)
	}

	repos, err := store.List()
	if err != nil {
		return fmt.Errorf("list repos: %w", err)
	}

	var gitOps *git.Operations
	if cfg != nil && cfg.Proxy.Enabled && cfg.Proxy.URL != "" {
		gitOps = git.NewOperationsWithProxy(cfg.Proxy.URL)
	} else {
		gitOps = git.NewOperations()
	}

	// Update ahead/behind for each repo concurrently and refresh stale conflict statuses
	var wg sync.WaitGroup
	sem := make(chan struct{}, 8)
	for i := range repos {
		if repos[i].Status == types.RepoStatusSyncing {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int) {
			defer wg.Done()
			defer func() { <-sem }()
			r := repos[idx]

			// For repos in conflict/resolving/resolved state, re-check the actual
			// git merge state. If the user has manually resolved and committed,
			// the stored status is stale and should be corrected.
			if r.Status == types.RepoStatusConflict || r.Status == types.RepoStatusResolving || r.Status == types.RepoStatusResolved {
				isMerging, unmergedFiles, err := gitOps.IsMergingState(cmd.Context(), r.Path)
				if err == nil && !isMerging {
					// No merge in progress — conflicts were resolved externally
					repos[idx].Status = types.RepoStatusSynced
					repos[idx].ErrorMessage = ""
					if updateErr := store.Update(repos[idx]); updateErr != nil {
						logger.Error("status: failed to update repo", "repo", r.Name, "error", updateErr)
					}
				} else if err == nil && isMerging && len(unmergedFiles) == 0 {
					// MERGE_HEAD exists but no unmerged files — user staged all
					// resolutions but hasn't committed yet. Still in conflict state
					// but update the status to reflect this transitional state.
					repos[idx].Status = types.RepoStatusResolved
					if updateErr := store.Update(repos[idx]); updateErr != nil {
						logger.Error("status: failed to update repo", "repo", r.Name, "error", updateErr)
					}
				} else if err == nil && isMerging && len(unmergedFiles) > 0 && r.Status == types.RepoStatusResolving {
					// MERGE_HEAD exists and there are still unmerged files, but
					// the repo is in "resolving" state. This means the agent has
					// exited (crashed, timed out, etc.) without finishing.
					// Roll back to "conflict" so the user can retry.
					repos[idx].Status = types.RepoStatusConflict
					repos[idx].ErrorMessage = "agent exited unexpectedly, conflict resolution incomplete"
					if updateErr := store.Update(repos[idx]); updateErr != nil {
						logger.Error("status: failed to update repo", "repo", r.Name, "error", updateErr)
					}
				}
			}

			// Fetch latest refs before calculating ahead/behind
			if fetchErr := gitOps.Fetch(cmd.Context(), r); fetchErr != nil {
				logger.Error("status: fetch failed", "repo", r.Name, "error", fetchErr)
			}
			statusResult, err := gitOps.Status(cmd.Context(), r)
			if err == nil && statusResult != nil {
				repos[idx].AheadBy = statusResult.AheadBy
				repos[idx].BehindBy = statusResult.BehindBy
				if repos[idx].Status == types.RepoStatusUnconfigured && r.Upstream != "" {
					repos[idx].Status = types.RepoStatusSynced
				}
			} else if err != nil {
				logger.Error("status: status check failed", "repo", r.Name, "error", err)
			}
		}(i)
	}
	wg.Wait()

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
