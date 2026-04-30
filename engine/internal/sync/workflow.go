package sync

import (
	"fmt"
	"time"

	"github.com/loongxjin/forksync/engine/pkg/types"
)

// newWorkflow creates a new SyncWorkflow with all steps initialized to pending.
func newWorkflow(runID string) *types.SyncWorkflow {
	now := time.Now()
	return &types.SyncWorkflow{
		RunID:     runID,
		Status:    types.WorkflowRunning,
		StartedAt: now,
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
}

// advanceStep updates a step to the given status and sets timestamps.
func advanceStep(wf *types.SyncWorkflow, step types.WorkflowStep, status types.WorkflowStepStatus, message string) {
	if wf == nil {
		return
	}
	now := types.Time{Time: time.Now()}
	for i := range wf.Steps {
		if wf.Steps[i].Step == step {
			wf.Steps[i].Status = status
			wf.Steps[i].Message = message
			if status == types.StepStatusRunning && wf.Steps[i].StartedAt == nil {
				wf.Steps[i].StartedAt = &now
			}
			if isTerminalStepStatus(status) {
				wf.Steps[i].EndedAt = &now
			}
			break
		}
	}
}

// markStepSkipped marks a step as skipped.
func markStepSkipped(wf *types.SyncWorkflow, step types.WorkflowStep) {
	advanceStep(wf, step, types.StepStatusSkipped, "")
}

// markWorkflowDone marks the workflow as completed (success or failed).
func markWorkflowDone(wf *types.SyncWorkflow, status types.WorkflowRunStatus) {
	if wf == nil {
		return
	}
	wf.Status = status
	now := types.Time{Time: time.Now()}
	wf.FinishedAt = &now
}

// isTerminalStepStatus returns true if the step status is a terminal state.
func isTerminalStepStatus(s types.WorkflowStepStatus) bool {
	return s == types.StepStatusSuccess || s == types.StepStatusFailed || s == types.StepStatusSkipped
}

// findStep finds a step record by step ID.
func findStep(wf *types.SyncWorkflow, step types.WorkflowStep) *types.WorkflowStepRecord {
	if wf == nil {
		return nil
	}
	for i := range wf.Steps {
		if wf.Steps[i].Step == step {
			return &wf.Steps[i]
		}
	}
	return nil
}

// workflowFromResult rebuilds a lightweight workflow from a sync result for display purposes.
// Used when a completed sync returns its final state.
func workflowFromResult(result *Result) *types.SyncWorkflow {
	if result == nil {
		return nil
	}
	wf := newWorkflow(result.RepoID)
	// Reconstruct steps based on result status
	advanceStep(wf, types.StepFetch, types.StepStatusSuccess, "")
	advanceStep(wf, types.StepMerge, types.StepStatusSuccess, "")

	if result.Status == string(types.RepoStatusUpToDate) && result.CommitsPulled == 0 {
		// No-op: fetch found nothing to sync
		markStepSkipped(wf, types.StepCheckConflicts)
		markStepSkipped(wf, types.StepResolveStrategy)
		markStepSkipped(wf, types.StepAgentResolve)
		markStepSkipped(wf, types.StepAcceptChanges)
		advanceStep(wf, types.StepCommit, types.StepStatusSuccess, "")
		markWorkflowDone(wf, types.WorkflowSuccess)
		return wf
	}

	if result.Status == string(types.RepoStatusUpToDate) && result.CommitsPulled > 0 {
		// Success with commits
		advanceStep(wf, types.StepCheckConflicts, types.StepStatusSuccess, "")
		markStepSkipped(wf, types.StepResolveStrategy)
		markStepSkipped(wf, types.StepAgentResolve)
		markStepSkipped(wf, types.StepAcceptChanges)
		advanceStep(wf, types.StepCommit, types.StepStatusSuccess, "")
		markWorkflowDone(wf, types.WorkflowSuccess)
		return wf
	}

	if result.Status == string(types.RepoStatusConflict) {
		advanceStep(wf, types.StepCheckConflicts, types.StepStatusSuccess,
			fmt.Sprintf("%d files have conflicts", len(result.ConflictFiles)))
		advanceStep(wf, types.StepResolveStrategy, types.StepStatusWaiting, "")
		markStepSkipped(wf, types.StepAgentResolve)
		markStepSkipped(wf, types.StepAcceptChanges)
		wf.Status = types.WorkflowWaiting
		return wf
	}

	if result.Status == string(types.RepoStatusResolving) {
		advanceStep(wf, types.StepCheckConflicts, types.StepStatusSuccess,
			fmt.Sprintf("%d files have conflicts", result.ConflictsFound))
		advanceStep(wf, types.StepResolveStrategy, types.StepStatusSuccess, "")
		advanceStep(wf, types.StepAgentResolve, types.StepStatusRunning, "")
		markStepSkipped(wf, types.StepAcceptChanges)
		return wf
	}

	if result.Status == string(types.RepoStatusResolved) {
		advanceStep(wf, types.StepCheckConflicts, types.StepStatusSuccess,
			fmt.Sprintf("%d files have conflicts", result.ConflictsFound))
		advanceStep(wf, types.StepResolveStrategy, types.StepStatusSuccess, "")
		advanceStep(wf, types.StepAgentResolve, types.StepStatusSuccess,
			fmt.Sprintf("resolved by %s", result.AgentUsed))
		advanceStep(wf, types.StepAcceptChanges, types.StepStatusWaiting, "")
		wf.Status = types.WorkflowWaiting
		return wf
	}

	if result.Status == string(types.RepoStatusError) {
		// Determine which step failed based on error message
		if result.ErrorMessage != "" {
			if contains(result.ErrorMessage, "fetch failed") {
				advanceStep(wf, types.StepFetch, types.StepStatusFailed, result.ErrorMessage)
			} else if contains(result.ErrorMessage, "merge failed") {
				advanceStep(wf, types.StepFetch, types.StepStatusSuccess, "")
				advanceStep(wf, types.StepMerge, types.StepStatusFailed, result.ErrorMessage)
			} else if contains(result.ErrorMessage, "commit") {
				advanceStep(wf, types.StepCheckConflicts, types.StepStatusSuccess, "")
				markStepSkipped(wf, types.StepResolveStrategy)
				markStepSkipped(wf, types.StepAgentResolve)
				markStepSkipped(wf, types.StepAcceptChanges)
				advanceStep(wf, types.StepCommit, types.StepStatusFailed, result.ErrorMessage)
			} else {
				advanceStep(wf, types.StepFetch, types.StepStatusSuccess, "")
				advanceStep(wf, types.StepMerge, types.StepStatusSuccess, "")
				advanceStep(wf, types.StepCheckConflicts, types.StepStatusFailed, result.ErrorMessage)
			}
		}
		markWorkflowDone(wf, types.WorkflowFailed)
		return wf
	}

	return wf
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) > 0 && findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
