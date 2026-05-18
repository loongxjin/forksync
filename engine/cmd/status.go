package cmd

import (
	"context"
	"fmt"
	stdsync "sync"
	"time"

	"github.com/loongxjin/forksync/engine/internal/agent"
	"github.com/loongxjin/forksync/engine/internal/git"
	"github.com/loongxjin/forksync/engine/internal/logger"
	"github.com/loongxjin/forksync/engine/internal/repo"
	"github.com/loongxjin/forksync/engine/internal/workflow"
	"github.com/loongxjin/forksync/engine/pkg/types"
	"github.com/spf13/cobra"
)

var statusExclude []string

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show status of all managed repositories",
	RunE:  runStatus,
}

func init() {
	rootCmd.AddCommand(statusCmd)
	statusCmd.Flags().StringSliceVar(&statusExclude, "exclude", nil, "comma-separated repo names to skip (e.g. --exclude repo1,repo2)")
}

// statusTimeout is the per-repo timeout for status operations (fetch + rev-list).
// 30 seconds is generous for a single repo; the overall command may take longer
// when there are many repos, but each individual operation is bounded.
const statusTimeout = 30 * time.Second

// staleWorkflowThreshold is the age after which an active workflow is considered stale.
const staleWorkflowThreshold = 30 * time.Minute

// resolvingStaleThreshold is the age after which a "resolving" status is considered
// stale and safe to recover. Must be longer than the max agent resolve timeout.
const resolvingStaleThreshold = 10 * time.Minute

// actionStatusUpdate is the log action label for repo status refresh updates.
const actionStatusUpdate = "status-update"

func runStatus(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithTimeout(cmd.Context(), statusTimeout)
	defer cancel()

	cfg, _ := getSharedConfig()

	store, err := loadRepoStore()
	if err != nil {
		return err
	}

	repos, err := store.List()
	if err != nil {
		return fmt.Errorf("list repos: %w", err)
	}

	var gitOps git.OperationsProvider
	gitOps = newGitOps(cfg)

	// Build exclude set for quick lookup
	excludeSet := make(map[string]bool, len(statusExclude))
	for _, name := range statusExclude {
		excludeSet[name] = true
	}

	// Clean up stale workflows before refreshing
	workflow.CleanupStaleWorkflows(repos, store)

	// Update ahead/behind for each repo concurrently and refresh stale conflict statuses
	var wg stdsync.WaitGroup
	sem := make(chan struct{}, types.DefaultMaxConcurrency)
	for i := range repos {
		if excludeSet[repos[i].Name] {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int) {
			defer wg.Done()
			defer func() { <-sem }()
			refreshRepoStatus(ctx, repos, idx, gitOps, store)
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
func refreshRepoStatus(ctx context.Context, repos []types.Repo, idx int, gitOps git.OperationsProvider, store repo.Store) {
	r := repos[idx]

	// For repos already in conflict/resolving/resolved/waiting state, re-check the actual
	// git merge state. If the user has manually resolved and committed,
	// the stored status is stale and should be corrected.
	if workflow.IsConflictState(r.Status) {
		reconcileConflictStatus(ctx, repos, idx, gitOps, store)
		return
	}

	// Rebuild workflow for repos that were syncing or in error before restart (spec §7.2).
	if r.Status == types.RepoStatusSyncing || r.Status == types.RepoStatusError {
		rebuildWorkflowForSyncingOrError(ctx, repos, idx, gitOps, store)
		if workflow.IsConflictState(repos[idx].Status) || repos[idx].Status == types.RepoStatusError {
			return
		}
	}

	// Proactively detect merge conflicts on disk regardless of stored status.
	// A repo may have MERGE_HEAD from external operations (e.g. manual git merge)
	// that were never tracked by forksync.
	// However, if the repo already has an active workflow (running/waiting),
	// skip this detection — the workflow was created by a syncer/resolve process
	// and rebuilding would destroy the real workflow state.
	if !workflow.IsConflictState(repos[idx].Status) && (repos[idx].Workflow == nil || repos[idx].Workflow.Status == "") {
		isMerging, unmergedFiles, err := gitOps.IsMergingState(ctx, r.Path)
		if err == nil && isMerging {
			if len(unmergedFiles) > 0 {
				repos[idx].Status = types.RepoStatusConflict
				repos[idx].ErrorMessage = "repository has unresolved merge conflicts"
				repos[idx].Workflow = workflow.RebuildWorkflow(r,
					workflow.RebuildFromConflict,
					fmt.Sprintf("%d files have conflicts", len(unmergedFiles)),
				)
				updateRepoWithLog(repos[idx], store, actionStatusUpdate)
				return
			}
			// MERGE_HEAD exists but all files resolved → resolved state
			repos[idx].Status = types.RepoStatusResolved
			repos[idx].Workflow = workflow.RebuildWorkflow(r,
				workflow.RebuildFromAcceptChanges,
			)
			updateRepoWithLog(repos[idx], store, actionStatusUpdate)
			return
		}
	}

	// Fetch latest refs before calculating ahead/behind
	if fetchErr := gitOps.Fetch(ctx, r); fetchErr != nil {
		logger.Warn("status: fetch failed", "repo", r.Name, "error", fetchErr)
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
		updateRepoWithLog(repos[idx], store, actionStatusUpdate)
	}

	// Detect sync_needed: upstream has new commits
	if workflow.IsSyncNeeded(repos[idx]) {
		repos[idx].Status = types.RepoStatusSyncNeeded
		repos[idx].ErrorMessage = ""
		updateRepoWithLog(repos[idx], store, actionStatusUpdate)
	} else if repos[idx].BehindBy == 0 && repos[idx].Status == types.RepoStatusSyncNeeded {
		// Previously sync_needed but now up-to-date (e.g. user synced externally)
		repos[idx].Status = types.RepoStatusUpToDate
		repos[idx].ErrorMessage = ""
		updateRepoWithLog(repos[idx], store, actionStatusUpdate)
	} else if repos[idx].BehindBy == 0 && repos[idx].Status == types.RepoStatusUpToDate && repos[idx].ErrorMessage != "" {
		// Stale error message from a previous post-sync failure — clear it.
		repos[idx].ErrorMessage = ""
		updateRepoWithLog(repos[idx], store, actionStatusUpdate)
	}
}

// reconcileConflictStatus checks the actual git merge state for a repo in conflict
// and corrects the stored status if the user has resolved externally.
func reconcileConflictStatus(ctx context.Context, repos []types.Repo, idx int, gitOps git.OperationsProvider, store repo.Store) {
	r := repos[idx]

	// If the agent is actively resolving, do not interfere at all.
	// IsMergingState below auto-stages files (git add), which would corrupt
	// the agent's working state. Only recover if the resolving state is stale
	// (agent process likely crashed).
	if r.Status == types.RepoStatusResolving {
		if r.Workflow != nil && time.Since(r.Workflow.StartedAt) < resolvingStaleThreshold {
			return
		}
	}

	isMerging, unmergedFiles, err := gitOps.IsMergingState(ctx, r.Path)
	if err != nil {
		return
	}

	// No merge in progress — conflicts were resolved externally
	if !isMerging {
		repos[idx].Status = types.RepoStatusUpToDate
		repos[idx].ErrorMessage = ""
		repos[idx].Workflow = nil
		updateRepoWithLog(repos[idx], store, actionStatusUpdate)
		return
	}

	// MERGE_HEAD exists but no unmerged files — user staged all resolutions
	if len(unmergedFiles) == 0 {
		repos[idx].Status = types.RepoStatusResolved
		// Rebuild workflow: fetch→merge→check_conflicts→resolve_strategy→agent_resolve(success)→accept_changes(waiting)
		repos[idx].Workflow = workflow.RebuildWorkflow(r,
			workflow.RebuildFromAcceptChanges,
		)
		updateRepoWithLog(repos[idx], store, actionStatusUpdate)
		return
	}

	// Still unmerged files + resolving state → agent exited unexpectedly, roll back
	if r.Status == types.RepoStatusResolving {
		repos[idx].Status = types.RepoStatusConflict
		repos[idx].ErrorMessage = "agent exited unexpectedly, conflict resolution incomplete"
		// Rebuild workflow: fetch→merge→check_conflicts(success)→resolve_strategy(success)→agent_resolve(running)
		repos[idx].Workflow = workflow.RebuildWorkflow(r,
			workflow.RebuildFromAgentResolve,
		)
		updateRepoWithLog(repos[idx], store, actionStatusUpdate)
		return
	}

	// Still have unmerged files — conflict state.
	// If the workflow already has a definitive failure (e.g. agent_resolve failed),
	// preserve it rather than blindly rebuilding.
	if r.Workflow != nil && r.Workflow.Status == types.WorkflowFailed {
		return
	}

	// Still have unmerged files — conflict state
	// Rebuild workflow: fetch→merge→check_conflicts(success with msg)→resolve_strategy(waiting)
	repos[idx].Workflow = workflow.RebuildWorkflow(r,
		workflow.RebuildFromConflict,
		fmt.Sprintf("%d files have conflicts", len(unmergedFiles)),
	)
	updateRepoWithLog(repos[idx], store, actionStatusUpdate)
}

// ---------------------------------------------------------------------------
// Workflow rebuild (spec §7.2) — delegates to workflow package
// ---------------------------------------------------------------------------

// rebuildWorkflowForSyncingOrError rebuilds a workflow for repos that were in
// syncing or error state before app restart.
func rebuildWorkflowForSyncingOrError(ctx context.Context, repos []types.Repo, idx int, gitOps git.OperationsProvider, store repo.Store) {
	r := repos[idx]
	updated, action := workflow.RecoverStaleState(ctx, r, gitOps)
	if action != "" {
		repos[idx] = updated
		updateRepoWithLog(repos[idx], store, actionStatusUpdate)
	}
}

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
		outputText("No AI agents detected. Install Claude Code, OpenCode, or Codex for auto-conflict resolution.")
	}
}
