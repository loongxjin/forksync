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

func runStatus(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithTimeout(cmd.Context(), statusTimeout)
	defer cancel()

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

	// Build exclude set for quick lookup
	excludeSet := make(map[string]bool, len(statusExclude))
	for _, name := range statusExclude {
		excludeSet[name] = true
	}

	// Clean up stale workflows before refreshing
	cleanupStaleWorkflows(repos, store)

	// Update ahead/behind for each repo concurrently and refresh stale conflict statuses
	var wg stdsync.WaitGroup
	sem := make(chan struct{}, types.DefaultMaxConcurrency)
	for i := range repos {
		if repos[i].Status == types.RepoStatusSyncing {
			continue
		}
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
func refreshRepoStatus(ctx context.Context, repos []types.Repo, idx int, gitOps *git.Operations, store repo.Store) {
	r := repos[idx]

	// For repos already in conflict/resolving/resolved/waiting state, re-check the actual
	// git merge state. If the user has manually resolved and committed,
	// the stored status is stale and should be corrected.
	if isConflictState(r.Status) {
		reconcileConflictStatus(ctx, repos, idx, gitOps, store)
		return
	}

	// Rebuild workflow for repos that were syncing or in error before restart (spec §7.2).
	if r.Status == types.RepoStatusSyncing || r.Status == types.RepoStatusError {
		rebuildWorkflowForSyncingOrError(ctx, repos, idx, gitOps, store)
		if isConflictState(repos[idx].Status) || repos[idx].Status == types.RepoStatusError {
			return
		}
	}

	// Proactively detect merge conflicts on disk regardless of stored status.
	// A repo may have MERGE_HEAD from external operations (e.g. manual git merge)
	// that were never tracked by forksync.
	if !isConflictState(repos[idx].Status) {
		isMerging, unmergedFiles, err := gitOps.IsMergingState(ctx, r.Path)
		if err == nil && isMerging {
			if len(unmergedFiles) > 0 {
				repos[idx].Status = types.RepoStatusConflict
				repos[idx].ErrorMessage = "repository has unresolved merge conflicts"
				repos[idx].Workflow = rebuildWorkflow(r,
					workflowRebuildFromConflict,
					fmt.Sprintf("%d files have conflicts", len(unmergedFiles)),
				)
				safeUpdateRepo(store, repos[idx], r.Name)
				return
			}
			// MERGE_HEAD exists but all files resolved → resolved state
			repos[idx].Status = types.RepoStatusResolved
			repos[idx].Workflow = rebuildWorkflow(r,
				workflowRebuildFromAcceptChanges,
			)
			safeUpdateRepo(store, repos[idx], r.Name)
			return
		}
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
		status == types.RepoStatusResolved ||
		status == types.RepoStatusWaiting
}

// cleanupStaleWorkflows removes successfully completed workflows.
// Success: cleared immediately on refresh.
// Failed/Waiting: retained until the user explicitly handles them.
func cleanupStaleWorkflows(repos []types.Repo, store repo.Store) {
	for i := range repos {
		wf := repos[i].Workflow
		if wf == nil {
			continue
		}
		if wf.Status == types.WorkflowSuccess {
			repos[i].Workflow = nil
			if updateErr := store.Update(repos[i]); updateErr != nil {
				logger.Error("status: failed to clear stale workflow", "repo", repos[i].Name, "error", updateErr)
			}
		}
	}
}

// reconcileConflictStatus checks the actual git merge state for a repo in conflict
// and corrects the stored status if the user has resolved externally.
// It also rebuilds a lightweight workflow for repos that were in an active state
// before app restart (see spec §7.2).
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
		repos[idx].Workflow = nil
		safeUpdateRepo(store, repos[idx], r.Name)
		return
	}

	// MERGE_HEAD exists but no unmerged files — user staged all resolutions
	if len(unmergedFiles) == 0 {
		repos[idx].Status = types.RepoStatusResolved
		// Rebuild workflow: fetch→merge→check_conflicts→resolve_strategy→agent_resolve(success)→accept_changes(waiting)
		repos[idx].Workflow = rebuildWorkflow(r,
			workflowRebuildFromAcceptChanges,
		)
		safeUpdateRepo(store, repos[idx], r.Name)
		return
	}

	// Still unmerged files + resolving state → agent exited unexpectedly, roll back
	if r.Status == types.RepoStatusResolving {
		repos[idx].Status = types.RepoStatusConflict
		repos[idx].ErrorMessage = "agent exited unexpectedly, conflict resolution incomplete"
		// Rebuild workflow: fetch→merge→check_conflicts(success)→resolve_strategy(success)→agent_resolve(running)
		repos[idx].Workflow = rebuildWorkflow(r,
			workflowRebuildFromAgentResolve,
		)
		safeUpdateRepo(store, repos[idx], r.Name)
		return
	}

	// Still have unmerged files — conflict state
	// Rebuild workflow: fetch→merge→check_conflicts(success with msg)→resolve_strategy(waiting)
	repos[idx].Workflow = rebuildWorkflow(r,
		workflowRebuildFromConflict,
		fmt.Sprintf("%d files have conflicts", len(unmergedFiles)),
	)
	safeUpdateRepo(store, repos[idx], r.Name)
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
		types.RepoStatusResolved, types.RepoStatusWaiting:
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

// ---------------------------------------------------------------------------
// Workflow rebuild helpers (spec §7.2)
// ---------------------------------------------------------------------------

type workflowRebuildPoint int

const (
	workflowRebuildFromFetch         workflowRebuildPoint = iota // syncing, no MERGE_HEAD
	workflowRebuildFromMerge                                       // syncing, MERGE_HEAD exists
	workflowRebuildFromConflict                                    // conflict: check_conflicts success, resolve_strategy waiting
	workflowRebuildFromAgentResolve                                // resolving: agent_resolve running
	workflowRebuildFromAcceptChanges                               // resolved: accept_changes waiting
	workflowRebuildFromCommitFailed                                // error: commit failed
)

// rebuildWorkflow creates a lightweight workflow for a repo that was in an active
// state before app restart. The rebuildPoint determines which step the workflow
// resumes from. All preceding steps are marked as success.
func rebuildWorkflow(r types.Repo, point workflowRebuildPoint, extraMsg ...string) *types.SyncWorkflow {
	msg := ""
	if len(extraMsg) > 0 {
		msg = extraMsg[0]
	}

	wf := &types.SyncWorkflow{
		RunID:     r.ID,
		Status:    types.WorkflowRunning,
		StartedAt: time.Now().Add(-time.Minute), // approximate
		Steps: []types.WorkflowStepRecord{
			{Step: types.StepFetch, Status: types.StepStatusPending},
			{Step: types.StepMerge, Status: types.StepStatusPending},
			{Step: types.StepCheckConflicts, Status: types.StepStatusPending},
			{Step: types.StepResolveStrategy, Status: types.StepStatusPending},
			{Step: types.StepAgentResolve, Status: types.StepStatusPending},
			{Step: types.StepAcceptChanges, Status: types.StepStatusPending},
			{Step: types.StepCommit, Status: types.StepStatusPending},
		},
	}

	switch point {
	case workflowRebuildFromFetch:
		// Syncing, no MERGE_HEAD → likely interrupted during fetch
		wf.Steps[0].Status = types.StepStatusRunning // fetch running
		wf.Status = types.WorkflowRunning

	case workflowRebuildFromMerge:
		// Syncing, MERGE_HEAD exists → interrupted during merge
		wf.Steps[0].Status = types.StepStatusSuccess // fetch done
		wf.Steps[1].Status = types.StepStatusRunning // merge running
		wf.Status = types.WorkflowRunning

	case workflowRebuildFromConflict:
		// Conflict state → check_conflicts found issues, waiting at resolve_strategy
		wf.Steps[0].Status = types.StepStatusSuccess
		wf.Steps[1].Status = types.StepStatusSuccess
		wf.Steps[2].Status = types.StepStatusSuccess
		wf.Steps[2].Message = msg
		wf.Steps[3].Status = types.StepStatusWaiting // resolve_strategy waiting
		wf.Steps[4].Status = types.StepStatusSkipped // agent_resolve
		wf.Steps[5].Status = types.StepStatusSkipped // accept_changes
		wf.Status = types.WorkflowWaiting

	case workflowRebuildFromAgentResolve:
		// Resolving → agent was running
		wf.Steps[0].Status = types.StepStatusSuccess
		wf.Steps[1].Status = types.StepStatusSuccess
		wf.Steps[2].Status = types.StepStatusSuccess
		wf.Steps[3].Status = types.StepStatusSuccess
		wf.Steps[4].Status = types.StepStatusRunning // agent_resolve running
		wf.Status = types.WorkflowRunning

	case workflowRebuildFromAcceptChanges:
		// Resolved → agent done, waiting for user to accept
		wf.Steps[0].Status = types.StepStatusSuccess
		wf.Steps[1].Status = types.StepStatusSuccess
		wf.Steps[2].Status = types.StepStatusSuccess
		wf.Steps[3].Status = types.StepStatusSuccess
		wf.Steps[4].Status = types.StepStatusSuccess
		wf.Steps[4].Message = "resolved"
		wf.Steps[5].Status = types.StepStatusWaiting // accept_changes waiting
		wf.Status = types.WorkflowWaiting

	case workflowRebuildFromCommitFailed:
		// Error → commit failed
		wf.Steps[0].Status = types.StepStatusSuccess
		wf.Steps[1].Status = types.StepStatusSuccess
		wf.Steps[2].Status = types.StepStatusSuccess
		wf.Steps[3].Status = types.StepStatusSkipped
		wf.Steps[4].Status = types.StepStatusSkipped
		wf.Steps[5].Status = types.StepStatusSkipped
		wf.Steps[6].Status = types.StepStatusFailed // commit failed
		wf.Steps[6].Error = r.ErrorMessage
		wf.Status = types.WorkflowFailed
	}

	return wf
}

// rebuildWorkflowForSyncingOrError rebuilds a workflow for repos that were in
// syncing or error state before app restart.
func rebuildWorkflowForSyncingOrError(ctx context.Context, repos []types.Repo, idx int, gitOps *git.Operations, store repo.Store) {
	r := repos[idx]

	isMerging, unmergedFiles, err := gitOps.IsMergingState(ctx, r.Path)
	if err != nil {
		return
	}

	if r.Status == types.RepoStatusSyncing {
		if !isMerging {
			// No MERGE_HEAD → interrupted during fetch
			repos[idx].Workflow = rebuildWorkflow(r, workflowRebuildFromFetch)
			repos[idx].Status = types.RepoStatusSyncNeeded
			repos[idx].ErrorMessage = ""
		} else if len(unmergedFiles) > 0 {
			// MERGE_HEAD + conflicts → interrupted during merge, conflicts found
			repos[idx].Workflow = rebuildWorkflow(r,
				workflowRebuildFromConflict,
				fmt.Sprintf("%d files have conflicts", len(unmergedFiles)),
			)
			repos[idx].Status = types.RepoStatusConflict
			repos[idx].ErrorMessage = "repository has unresolved merge conflicts"
		} else {
			// MERGE_HEAD exists, no conflicts → interrupted during merge/commit
			repos[idx].Workflow = rebuildWorkflow(r, workflowRebuildFromMerge)
			repos[idx].Status = types.RepoStatusSyncing
		}
		safeUpdateRepo(store, repos[idx], r.Name)
		return
	}

	if r.Status == types.RepoStatusError {
		if isMerging {
			// MERGE_HEAD exists → commit likely failed
			repos[idx].Workflow = rebuildWorkflow(r, workflowRebuildFromCommitFailed)
		} else {
			// No MERGE_HEAD → fetch or merge failed, clear workflow
			repos[idx].Workflow = nil
			repos[idx].Status = types.RepoStatusSyncNeeded
			repos[idx].ErrorMessage = ""
		}
		safeUpdateRepo(store, repos[idx], r.Name)
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
		outputText("No AI agents detected. Install Claude Code, OpenCode, Droid, or Codex for auto-conflict resolution.")
	}
}
