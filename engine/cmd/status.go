package cmd

import (
	"context"
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
			refreshRepoStatus(cmd.Context(), repos, idx, gitOps, store)
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
		printStatusText(repos, agents, preferredAgent)
	}

	return nil
}

// refreshRepoStatus refreshes a single repo's ahead/behind counts and reconciles
// stale conflict states (e.g. user resolved externally).
func refreshRepoStatus(ctx context.Context, repos []types.Repo, idx int, gitOps *git.Operations, store repo.Store) {
	r := repos[idx]

	// For repos in conflict/resolving/resolved state, re-check the actual
	// git merge state. If the user has manually resolved and committed,
	// the stored status is stale and should be corrected.
	if isConflictState(r.Status) {
		reconcileConflictStatus(ctx, repos, idx, gitOps, store)
	}

	// Fetch latest refs before calculating ahead/behind
	if fetchErr := gitOps.Fetch(ctx, r); fetchErr != nil {
		logger.Error("status: fetch failed", "repo", r.Name, "error", fetchErr)
	}

	statusResult, err := gitOps.Status(ctx, r)
	if err != nil {
		logger.Error("status: status check failed", "repo", r.Name, "error", err)
		return
	}
	if statusResult == nil {
		return
	}

	repos[idx].AheadBy = statusResult.AheadBy
	repos[idx].BehindBy = statusResult.BehindBy

	// Transition unconfigured repos to up_to_date
	if repos[idx].Status == types.RepoStatusUnconfigured {
		repos[idx].Status = types.RepoStatusUpToDate
		safeUpdateRepo(store, repos[idx], r.Name)
	}

	// Detect sync_needed: upstream has new commits
	if isSyncNeeded(repos[idx]) {
		repos[idx].Status = types.RepoStatusSyncNeeded
		repos[idx].ErrorMessage = ""
		safeUpdateRepo(store, repos[idx], r.Name)
	}
}

// isConflictState returns true if the repo is in a conflict-related state
// that needs reconciliation with the actual git merge state.
func isConflictState(status types.RepoStatus) bool {
	return status == types.RepoStatusConflict ||
		status == types.RepoStatusResolving ||
		status == types.RepoStatusResolved
}

// reconcileConflictStatus checks the actual git merge state for a repo in conflict
// and corrects the stored status if the user has resolved externally.
func reconcileConflictStatus(ctx context.Context, repos []types.Repo, idx int, gitOps *git.Operations, store repo.Store) {
	r := repos[idx]
	isMerging, unmergedFiles, err := gitOps.IsMergingState(ctx, r.Path)
	if err != nil {
		return
	}

	// No merge in progress — conflicts were resolved externally
	if !isMerging {
		repos[idx].Status = types.RepoStatusUpToDate
		repos[idx].ErrorMessage = ""
		safeUpdateRepo(store, repos[idx], r.Name)
		return
	}

	// MERGE_HEAD exists but no unmerged files — user staged all resolutions
	if len(unmergedFiles) == 0 {
		repos[idx].Status = types.RepoStatusResolved
		safeUpdateRepo(store, repos[idx], r.Name)
		return
	}

	// Still unmerged files + resolving state → agent exited unexpectedly, roll back
	if r.Status == types.RepoStatusResolving {
		repos[idx].Status = types.RepoStatusConflict
		repos[idx].ErrorMessage = "agent exited unexpectedly, conflict resolution incomplete"
		safeUpdateRepo(store, repos[idx], r.Name)
	}
}

// isSyncNeeded returns true if upstream has new commits and the repo is in a
// state that allows transitioning to sync_needed.
func isSyncNeeded(r types.Repo) bool {
	if r.BehindBy == 0 {
		return false
	}
	switch r.Status {
	case types.RepoStatusSyncing, types.RepoStatusError,
		types.RepoStatusConflict, types.RepoStatusResolving,
		types.RepoStatusResolved:
		return false
	}
	return true
}

// safeUpdateRepo updates the repo in the store and logs any error.
func safeUpdateRepo(store repo.Store, r types.Repo, repoName string) {
	if updateErr := store.Update(r); updateErr != nil {
		logger.Error("status: failed to update repo", "repo", repoName, "error", updateErr)
	}
}

// printStatusText renders the status output in human-readable text format.
func printStatusText(repos []types.Repo, agents []types.AgentInfo, preferredAgent string) {
	if len(repos) == 0 {
		outputText("No repositories managed. Use 'forksync add <path>' to add one.")
	} else {
		outputText("Managed Repositories (%d):", len(repos))
		outputText("")
		for _, r := range repos {
			statusIcon := "⚪"
			switch r.Status {
			case types.RepoStatusUpToDate:
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
