package sync

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	stdsync "sync"
	"time"

	"github.com/loongxjin/forksync/engine/core/config"
	"github.com/loongxjin/forksync/engine/core/git"
	"github.com/loongxjin/forksync/engine/core/logger"
	"github.com/loongxjin/forksync/engine/core/repo"
	"github.com/loongxjin/forksync/engine/core/workflow"
	"github.com/loongxjin/forksync/engine/pkg/types"
)

// StatusRefresher handles concurrent status refresh for all repos.
type StatusRefresher struct {
	gitOps       git.OperationsProvider
	store        repo.Store
	cfg          *config.Config
	fetchTimeout time.Duration // sub-budget for the upstream fetch (default statusFetchTimeout)
}

// NewStatusRefresher creates a new StatusRefresher.
func NewStatusRefresher(
	gitOps git.OperationsProvider,
	store repo.Store,
	cfg *config.Config,
) *StatusRefresher {
	return &StatusRefresher{
		gitOps:       gitOps,
		store:        store,
		cfg:          cfg,
		fetchTimeout: statusFetchTimeout,
	}
}

// statusFetchTimeout caps the upstream fetch portion of a status check. It is
// a sub-budget of perRepoStatusTimeout so a slow/failed fetch does not exhaust
// the repo's whole budget — the local rev-list (Status) still runs afterwards
// against the previously-fetched refs, giving the user a usable result even
// when the network is slow.
const statusFetchTimeout = 10 * time.Second

// perRepoStatusTimeout is the timeout budget granted to each repo's status
// check (fetch + rev-list), independent of other repos. Previously all repos
// shared a single timeout: one slow upstream fetch consumed the whole budget
// and every other repo's git commands were killed with "context deadline
// exceeded". A per-repo budget isolates slow repos so they fail alone.
const perRepoStatusTimeout = 20 * time.Second

// RefreshAll refreshes the status of all repos concurrently.
// Handles: ahead/behind update, stale workflow cleanup,
// crash recovery, conflict state reconciliation.
//
// Each repo gets its own timeout budget (perRepoStatusTimeout) derived from
// ctx, so a slow upstream fetch on one repo cannot starve the others. The
// parent ctx still controls overall cancellation (app quit, request cancel).
func (sf *StatusRefresher) RefreshAll(
	ctx context.Context,
	repos []types.Repo,
	excludeNames []string,
) ([]types.Repo, error) {
	return sf.refreshAll(ctx, repos, excludeNames, perRepoStatusTimeout, statusFetchTimeout)
}

// RefreshAllWithPerRepoTimeout is like RefreshAll but with caller-supplied
// per-repo and fetch timeouts. Exposed for tests.
func (sf *StatusRefresher) RefreshAllWithPerRepoTimeout(
	ctx context.Context,
	repos []types.Repo,
	excludeNames []string,
	perRepo, fetch time.Duration,
) ([]types.Repo, error) {
	return sf.refreshAll(ctx, repos, excludeNames, perRepo, fetch)
}

func (sf *StatusRefresher) refreshAll(
	ctx context.Context,
	repos []types.Repo,
	excludeNames []string,
	perRepo, fetch time.Duration,
) ([]types.Repo, error) {
	// Build exclude set for quick lookup
	excludeSet := make(map[string]bool, len(excludeNames))
	for _, name := range excludeNames {
		excludeSet[name] = true
	}

	// Clean up stale workflows before refreshing
	workflow.CleanupStaleWorkflows(repos, sf.store)

	// Apply the caller-supplied fetch sub-budget for this batch.
	sf.fetchTimeout = fetch

	// Update ahead/behind for each repo concurrently and refresh stale conflict statuses.
	// Each repo runs under its own per-repo timeout so a slow fetch on one repo
	// cannot cancel the others' status checks.
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
			repoCtx, cancel := context.WithTimeout(ctx, perRepo)
			defer cancel()
			repos[idx] = sf.RefreshRepo(repoCtx, repos[idx])
		}(i)
	}
	wg.Wait()

	return repos, nil
}

// RefreshRepo refreshes a single repo's status.
func (sf *StatusRefresher) RefreshRepo(ctx context.Context, r types.Repo) types.Repo {
	// For repos already in conflict/resolving/resolved/waiting state, re-check the actual
	// git merge state. If the user has manually resolved and committed,
	// the stored status is stale and should be corrected.
	if workflow.IsConflictState(r.Status) {
		return sf.reconcileConflictStatus(ctx, r)
	}

	// Rebuild workflow for repos that were syncing or in error before restart (spec §7.2).
	if r.Status == types.RepoStatusSyncing || r.Status == types.RepoStatusError {
		updated, action := workflow.RecoverStaleState(ctx, r, sf.gitOps)
		if action != "" {
			r = updated
		}
		if workflow.IsConflictState(r.Status) || r.Status == types.RepoStatusError {
			return r
		}
	}

	// Proactively detect merge conflicts on disk regardless of stored status.
	if !workflow.IsConflictState(r.Status) && (r.Workflow == nil || r.Workflow.Status == "") {
		isMerging, unmergedFiles, err := sf.gitOps.IsMergingState(ctx, r.Path)
		if err == nil && isMerging {
			if len(unmergedFiles) > 0 {
				r.Status = types.RepoStatusConflict
				r.ErrorMessage = "repository has unresolved merge conflicts"
				r.Workflow = workflow.RebuildWorkflow(r,
					workflow.RebuildFromConflict,
					fmt.Sprintf("%d files have conflicts", len(unmergedFiles)),
				)
				sf.updateRepo(r)
				return r
			}
			// MERGE_HEAD exists but all files resolved → resolved state
			r.Status = types.RepoStatusResolved
			r.Workflow = workflow.RebuildWorkflow(r, workflow.RebuildFromAcceptChanges)
			sf.updateRepo(r)
			return r
		}
	}

	// Fetch latest refs before calculating ahead/behind. Fetch runs under a
	// sub-timeout so a slow/failed network fetch does not exhaust the repo's
	// whole ctx budget — the local rev-list below still runs against the
	// previously-fetched refs, giving a usable result even when the network
	// is slow or the proxy fails.
	fetchCtx, fetchCancel := context.WithTimeout(ctx, sf.fetchTimeout)
	if fetchErr := sf.gitOps.Fetch(fetchCtx, r); fetchErr != nil {
		logger.Warn("status: fetch failed", "repo", r.Name, "error", fetchErr)
	}
	fetchCancel()

	statusResult, err := sf.gitOps.Status(ctx, r)
	if err != nil {
		logger.Error("status: status check failed", "repo", r.Name, "error", err)
		return r
	}
	if statusResult == nil {
		return r
	}

	r.AheadBy = statusResult.AheadBy
	r.BehindBy = statusResult.BehindBy

	// Transition unconfigured repos to up_to_date
	if r.Status == types.RepoStatusUnconfigured {
		r.Status = types.RepoStatusUpToDate
		sf.updateRepo(r)
	}

	// Detect sync_needed: upstream has new commits
	if workflow.IsSyncNeeded(r) {
		r.Status = types.RepoStatusSyncNeeded
		r.ErrorMessage = ""
		sf.updateRepo(r)
	} else if r.BehindBy == 0 && r.Status == types.RepoStatusSyncNeeded {
		// Previously sync_needed but now up-to-date (e.g. user synced externally)
		r.Status = types.RepoStatusUpToDate
		r.ErrorMessage = ""
		sf.updateRepo(r)
	} else if r.BehindBy == 0 && r.Status == types.RepoStatusUpToDate && r.ErrorMessage != "" {
		// Stale error message from a previous post-sync failure — clear it.
		r.ErrorMessage = ""
		sf.updateRepo(r)
	}

	// Clean up stale workflows for repos in a settled state (up_to_date, sync_needed, unconfigured).
	// This handles the case where the user manually resolved a conflict outside the app,
	// the sync command updated status to up_to_date but left a stale workflow behind.
	if r.Workflow != nil && r.Workflow.Status != "" {
		switch r.Status {
		case types.RepoStatusUpToDate, types.RepoStatusSyncNeeded, types.RepoStatusUnconfigured:
			logger.Info("status: clearing stale workflow for settled repo",
				"repo", r.Name, "repo_status", string(r.Status),
				"workflow_status", string(r.Workflow.Status))
			r.Workflow = nil
			sf.updateRepo(r)
		}
	}

	return r
}

// reconcileConflictStatus checks the actual git merge state for a repo in conflict
// and corrects the stored status if the user has resolved externally.
func (sf *StatusRefresher) reconcileConflictStatus(ctx context.Context, r types.Repo) types.Repo {
	resolvingStaleThreshold := 10 * time.Minute

	// If the agent is actively resolving, do not interfere at all.
	// IsMergingState below auto-stages files (git add), which would corrupt
	// the agent's working state. Only recover if the resolving state is stale
	// (agent process likely crashed).
	if r.Status == types.RepoStatusResolving {
		if r.Workflow != nil && time.Since(r.Workflow.StartedAt) < resolvingStaleThreshold {
			return r
		}
	}

	isMerging, unmergedFiles, err := sf.gitOps.IsMergingState(ctx, r.Path)
	if err != nil {
		logger.Warn("status: reconcileConflictStatus IsMergingState failed, checking MERGE_HEAD directly",
			"repo", r.Name, "error", err)
		// Fallback: directly check if MERGE_HEAD exists on disk.
		// If MERGE_HEAD is gone, the merge was completed externally.
		mergeHead := filepath.Join(r.Path, ".git", "MERGE_HEAD")
		if _, statErr := os.Stat(mergeHead); statErr != nil && os.IsNotExist(statErr) {
			logger.Info("status: MERGE_HEAD gone after IsMergingState error, clearing conflict state", "repo", r.Name)
			r.Status = types.RepoStatusUpToDate
			r.ErrorMessage = ""
			r.Workflow = nil
			sf.updateRepo(r)
			return r
		}
		// MERGE_HEAD still exists or can't tell — keep current state
		return r
	}

	// No merge in progress — conflicts were resolved externally
	if !isMerging {
		r.Status = types.RepoStatusUpToDate
		r.ErrorMessage = ""
		r.Workflow = nil
		sf.updateRepo(r)
		return r
	}

	// MERGE_HEAD exists but no unmerged files — user staged all resolutions
	// MERGE_HEAD exists but no unmerged files — user/agent staged all resolutions.
	// Before rebuilding the workflow, check if the existing one is already a valid
	// resolved-state workflow (has agent_resolve completed or waiting). If so,
	// preserve it to avoid losing the resolveSessionId stamped on the step.
	if r.Workflow != nil && r.Workflow.Status == types.WorkflowWaiting {
		step := workflow.FindStep(r.Workflow, types.StepAgentResolve)
		if step != nil && (step.Status == types.StepStatusSuccess || step.Status == types.StepStatusWaiting) {
			// Workflow is already in the correct resolved/waiting state with
			// the resolveSessionId preserved. Don't rebuild.
			return r
		}
	}
	if len(unmergedFiles) == 0 {
		r.Status = types.RepoStatusResolved
		r.Workflow = workflow.RebuildWorkflow(r, workflow.RebuildFromAcceptChanges)
		sf.updateRepo(r)
		return r
	}

	// Still unmerged files + resolving state → agent exited unexpectedly, roll back
	if r.Status == types.RepoStatusResolving {
		r.Status = types.RepoStatusConflict
		r.ErrorMessage = "agent exited unexpectedly, conflict resolution incomplete"
		r.Workflow = workflow.RebuildWorkflow(r, workflow.RebuildFromAgentResolve)
		sf.updateRepo(r)
		return r
	}

	// Still have unmerged files — conflict state.
	// If the workflow already has a definitive failure (e.g. agent_resolve failed),
	// preserve it rather than blindly rebuilding.
	if r.Workflow != nil && r.Workflow.Status == types.WorkflowFailed {
		return r
	}

	// If the existing workflow has an agent_resolve step that is currently
	// running (auto-resolve within sync), preserve it. Rebuilding here would
	// lose the resolveSessionId stamped by the syncer, and the follow-up
	// store.Update would trigger an eventsStore→refreshSilent→status→here
	// feedback loop via the /stream/events push channel.
	if r.Workflow != nil {
		step := workflow.FindStep(r.Workflow, types.StepAgentResolve)
		if step != nil && step.Status == types.StepStatusRunning {
			return r
		}
	}

	// Still have unmerged files — conflict state
	r.Workflow = workflow.RebuildWorkflow(r,
		workflow.RebuildFromConflict,
		fmt.Sprintf("%d files have conflicts", len(unmergedFiles)),
	)
	sf.updateRepo(r)
	return r
}

func (sf *StatusRefresher) updateRepo(r types.Repo) {
	if err := sf.store.Update(r); err != nil {
		logger.Error("status: failed to update repo", "repo", r.Name, "error", err)
	}
}
