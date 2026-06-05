package sync

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	stdsync "sync"
	"time"

	"github.com/loongxjin/forksync/engine/internal/config"
	"github.com/loongxjin/forksync/engine/internal/git"
	"github.com/loongxjin/forksync/engine/internal/logger"
	"github.com/loongxjin/forksync/engine/internal/repo"
	"github.com/loongxjin/forksync/engine/internal/workflow"
	"github.com/loongxjin/forksync/engine/pkg/types"
)

// StatusRefresher handles concurrent status refresh for all repos.
type StatusRefresher struct {
	gitOps git.OperationsProvider
	store  repo.Store
	cfg    *config.Config
}

// NewStatusRefresher creates a new StatusRefresher.
func NewStatusRefresher(
	gitOps git.OperationsProvider,
	store repo.Store,
	cfg *config.Config,
) *StatusRefresher {
	return &StatusRefresher{
		gitOps: gitOps,
		store:  store,
		cfg:    cfg,
	}
}

// RefreshAll refreshes the status of all repos concurrently.
// Handles: ahead/behind update, stale workflow cleanup,
// crash recovery, conflict state reconciliation.
func (sf *StatusRefresher) RefreshAll(
	ctx context.Context,
	repos []types.Repo,
	excludeNames []string,
) ([]types.Repo, error) {
	// Build exclude set for quick lookup
	excludeSet := make(map[string]bool, len(excludeNames))
	for _, name := range excludeNames {
		excludeSet[name] = true
	}

	// Clean up stale workflows before refreshing
	workflow.CleanupStaleWorkflows(repos, sf.store)

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
			repos[idx] = sf.RefreshRepo(ctx, repos[idx])
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

	// Fetch latest refs before calculating ahead/behind
	if fetchErr := sf.gitOps.Fetch(ctx, r); fetchErr != nil {
		logger.Warn("status: fetch failed", "repo", r.Name, "error", fetchErr)
	}

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
