package workflow

import (
	"context"
	"fmt"
	"time"

	"github.com/loongxjin/forksync/engine/internal/git"
	"github.com/loongxjin/forksync/engine/internal/logger"
	"github.com/loongxjin/forksync/engine/internal/repo"
	"github.com/loongxjin/forksync/engine/pkg/types"
)

// RebuildPoint represents where the workflow was interrupted.
type RebuildPoint int

const (
	RebuildFromFetch         RebuildPoint = iota // syncing, no MERGE_HEAD
	RebuildFromMerge                             // syncing, MERGE_HEAD exists
	RebuildFromConflict                          // conflict: check_conflicts success, resolve_strategy waiting
	RebuildFromAgentResolve                      // resolving: agent_resolve running
	RebuildFromAcceptChanges                     // resolved: accept_changes waiting
	RebuildFromCommitFailed                      // error: commit failed
)

// RebuildWorkflow creates a lightweight workflow for a repo that was in an active
// state before app restart. The RebuildPoint determines which step the workflow
// resumes from. All preceding steps are marked as success.
func RebuildWorkflow(r types.Repo, point RebuildPoint, extraMsg ...string) *types.SyncWorkflow {
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
	case RebuildFromFetch:
		// Syncing, no MERGE_HEAD → likely interrupted during fetch
		wf.Steps[0].Status = types.StepStatusRunning // fetch running
		wf.Status = types.WorkflowRunning

	case RebuildFromMerge:
		// Syncing, MERGE_HEAD exists → interrupted during merge
		wf.Steps[0].Status = types.StepStatusSuccess // fetch done
		wf.Steps[1].Status = types.StepStatusRunning // merge running
		wf.Status = types.WorkflowRunning

	case RebuildFromConflict:
		// Conflict state → check_conflicts found issues, waiting at resolve_strategy
		wf.Steps[0].Status = types.StepStatusSuccess
		wf.Steps[1].Status = types.StepStatusSuccess
		wf.Steps[2].Status = types.StepStatusSuccess
		wf.Steps[2].Message = msg
		wf.Steps[3].Status = types.StepStatusWaiting // resolve_strategy waiting
		wf.Steps[4].Status = types.StepStatusSkipped // agent_resolve
		wf.Steps[5].Status = types.StepStatusSkipped // accept_changes
		wf.Status = types.WorkflowWaiting

	case RebuildFromAgentResolve:
		// Resolving → agent was running
		wf.Steps[0].Status = types.StepStatusSuccess
		wf.Steps[1].Status = types.StepStatusSuccess
		wf.Steps[2].Status = types.StepStatusSuccess
		wf.Steps[3].Status = types.StepStatusSuccess
		wf.Steps[4].Status = types.StepStatusRunning // agent_resolve running
		wf.Status = types.WorkflowRunning

	case RebuildFromAcceptChanges:
		// Resolved → agent done, waiting for user to accept
		wf.Steps[0].Status = types.StepStatusSuccess
		wf.Steps[1].Status = types.StepStatusSuccess
		wf.Steps[2].Status = types.StepStatusSuccess
		wf.Steps[3].Status = types.StepStatusSuccess
		wf.Steps[4].Status = types.StepStatusSuccess
		wf.Steps[4].Message = "resolved"
		wf.Steps[5].Status = types.StepStatusWaiting // accept_changes waiting
		wf.Status = types.WorkflowWaiting

	case RebuildFromCommitFailed:
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

// staleWorkflowThreshold is the age after which an active workflow is considered stale.
const staleWorkflowThreshold = 30 * time.Minute

// RecoverStaleState examines a repo's git state and rebuilds
// an appropriate workflow if the previous run was interrupted.
// Used for repos that were syncing or in error state before app restart.
func RecoverStaleState(
	ctx context.Context,
	r types.Repo,
	gitOps git.OperationsProvider,
) (types.Repo, string) { // returns updated repo and action label
	// If the repo has an active workflow that started recently, assume sync
	// is still running and do not touch it.
	if r.Workflow != nil && !r.Workflow.StartedAt.IsZero() &&
		time.Since(r.Workflow.StartedAt) < staleWorkflowThreshold {
		return r, ""
	}

	isMerging, unmergedFiles, err := gitOps.IsMergingState(ctx, r.Path)
	if err != nil {
		return r, ""
	}

	if r.Status == types.RepoStatusSyncing {
		if !isMerging {
			// No MERGE_HEAD → interrupted during fetch
			r.Workflow = RebuildWorkflow(r, RebuildFromFetch)
			r.Status = types.RepoStatusSyncNeeded
			r.ErrorMessage = ""
		} else if len(unmergedFiles) > 0 {
			// MERGE_HEAD + conflicts → interrupted during merge, conflicts found
			r.Workflow = RebuildWorkflow(r, RebuildFromConflict,
				fmt.Sprintf("%d files have conflicts", len(unmergedFiles)))
			r.Status = types.RepoStatusConflict
			r.ErrorMessage = "repository has unresolved merge conflicts"
		} else {
			// MERGE_HEAD exists, no conflicts → interrupted during merge/commit
			r.Workflow = RebuildWorkflow(r, RebuildFromMerge)
			r.Status = types.RepoStatusSyncing
		}
		return r, "status-update"
	}

	if r.Status == types.RepoStatusError {
		if isMerging {
			// MERGE_HEAD exists → commit likely failed
			r.Workflow = RebuildWorkflow(r, RebuildFromCommitFailed)
		} else {
			// No MERGE_HEAD → fetch or merge failed, clear workflow
			r.Workflow = nil
			r.Status = types.RepoStatusSyncNeeded
			r.ErrorMessage = ""
		}
		return r, "status-update"
	}

	return r, ""
}

// CleanupStaleWorkflows removes successfully completed workflows and workflows
// that were explicitly aborted/rejected by the user.
func CleanupStaleWorkflows(repos []types.Repo, store repo.Store) {
	for i := range repos {
		wf := repos[i].Workflow
		if wf == nil {
			continue
		}
		if wf.Status == types.WorkflowSuccess || isAbortedWorkflow(wf) {
			repos[i].Workflow = nil
			if updateErr := store.Update(repos[i]); updateErr != nil {
				logger.Error("status: failed to clear stale workflow", "repo", repos[i].Name, "error", updateErr)
			}
		}
	}
}

func isAbortedWorkflow(wf *types.SyncWorkflow) bool {
	if wf == nil || wf.Status != types.WorkflowFailed {
		return false
	}
	for _, step := range wf.Steps {
		if step.Status == types.StepStatusFailed {
			return false
		}
	}
	return true
}

// IsConflictState returns true if the repo is in a conflict-related state.
func IsConflictState(status types.RepoStatus) bool {
	return status == types.RepoStatusConflict ||
		status == types.RepoStatusResolving ||
		status == types.RepoStatusResolved ||
		status == types.RepoStatusWaiting
}

// IsSyncNeeded returns true if upstream has new commits and the repo is in a
// state that allows transitioning to sync_needed.
func IsSyncNeeded(r types.Repo) bool {
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

// NewWorkflowFromRepo creates a minimal workflow for a repo that doesn't have one.
func NewWorkflowFromRepo(r types.Repo) *types.SyncWorkflow {
	return &types.SyncWorkflow{
		RunID:     r.ID,
		Status:    types.WorkflowRunning,
		StartedAt: time.Now(),
		Steps: []types.WorkflowStepRecord{
			{Step: types.StepFetch, Status: types.StepStatusSuccess},
			{Step: types.StepMerge, Status: types.StepStatusSuccess},
			{Step: types.StepCheckConflicts, Status: types.StepStatusSuccess},
			{Step: types.StepResolveStrategy, Status: types.StepStatusPending},
			{Step: types.StepAgentResolve, Status: types.StepStatusPending},
			{Step: types.StepAcceptChanges, Status: types.StepStatusPending},
			{Step: types.StepCommit, Status: types.StepStatusPending},
		},
	}
}
