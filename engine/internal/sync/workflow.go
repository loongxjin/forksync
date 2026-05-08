package sync

import (
	"fmt"
	"strings"
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

// AdvanceStep updates a step to the given status and sets timestamps.
func AdvanceStep(wf *types.SyncWorkflow, step types.WorkflowStep, status types.WorkflowStepStatus, message string) {
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
			if IsTerminalStepStatus(status) {
				wf.Steps[i].EndedAt = &now
			}
			break
		}
	}
}

// MarkStepSkipped marks a step as skipped.
func MarkStepSkipped(wf *types.SyncWorkflow, step types.WorkflowStep) {
	AdvanceStep(wf, step, types.StepStatusSkipped, "")
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

// IsTerminalStepStatus returns true if the step status is a terminal state.
func IsTerminalStepStatus(s types.WorkflowStepStatus) bool {
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
	AdvanceStep(wf, types.StepFetch, types.StepStatusSuccess, "")
	AdvanceStep(wf, types.StepMerge, types.StepStatusSuccess, "")

	switch result.Status {
	case string(types.RepoStatusUpToDate):
		if result.CommitsPulled == 0 {
			// No-op: fetch found nothing to sync
			MarkStepSkipped(wf, types.StepCheckConflicts)
			MarkStepSkipped(wf, types.StepResolveStrategy)
			MarkStepSkipped(wf, types.StepAgentResolve)
			MarkStepSkipped(wf, types.StepAcceptChanges)
			AdvanceStep(wf, types.StepCommit, types.StepStatusSuccess, "")
			markWorkflowDone(wf, types.WorkflowSuccess)
			return wf
		}

		// Success with commits
		AdvanceStep(wf, types.StepCheckConflicts, types.StepStatusSuccess, "")
		MarkStepSkipped(wf, types.StepResolveStrategy)
		MarkStepSkipped(wf, types.StepAgentResolve)
		MarkStepSkipped(wf, types.StepAcceptChanges)
		AdvanceStep(wf, types.StepCommit, types.StepStatusSuccess, "")
		markWorkflowDone(wf, types.WorkflowSuccess)
		return wf

	case string(types.RepoStatusConflict):
		AdvanceStep(wf, types.StepCheckConflicts, types.StepStatusSuccess,
			fmt.Sprintf("%d files have conflicts", len(result.ConflictFiles)))
		AdvanceStep(wf, types.StepResolveStrategy, types.StepStatusWaiting, "")
		MarkStepSkipped(wf, types.StepAgentResolve)
		MarkStepSkipped(wf, types.StepAcceptChanges)
		wf.Status = types.WorkflowWaiting
		return wf

	case string(types.RepoStatusResolving):
		AdvanceStep(wf, types.StepCheckConflicts, types.StepStatusSuccess,
			fmt.Sprintf("%d files have conflicts", result.ConflictsFound))
		AdvanceStep(wf, types.StepResolveStrategy, types.StepStatusSuccess, "")
		AdvanceStep(wf, types.StepAgentResolve, types.StepStatusRunning, "")
		MarkStepSkipped(wf, types.StepAcceptChanges)
		return wf

	case string(types.RepoStatusResolved):
		AdvanceStep(wf, types.StepCheckConflicts, types.StepStatusSuccess,
			fmt.Sprintf("%d files have conflicts", result.ConflictsFound))
		AdvanceStep(wf, types.StepResolveStrategy, types.StepStatusSuccess, "")
		AdvanceStep(wf, types.StepAgentResolve, types.StepStatusSuccess,
			fmt.Sprintf("resolved by %s", result.AgentUsed))
		AdvanceStep(wf, types.StepAcceptChanges, types.StepStatusWaiting, "")
		wf.Status = types.WorkflowWaiting
		return wf

	case string(types.RepoStatusError):
		// Determine which step failed based on error message
		if result.ErrorMessage != "" {
			if strings.Contains(result.ErrorMessage, "fetch failed") {
				AdvanceStep(wf, types.StepFetch, types.StepStatusFailed, result.ErrorMessage)
			} else if strings.Contains(result.ErrorMessage, "merge failed") {
				AdvanceStep(wf, types.StepFetch, types.StepStatusSuccess, "")
				AdvanceStep(wf, types.StepMerge, types.StepStatusFailed, result.ErrorMessage)
			} else if strings.Contains(result.ErrorMessage, "commit") {
				AdvanceStep(wf, types.StepCheckConflicts, types.StepStatusSuccess, "")
				MarkStepSkipped(wf, types.StepResolveStrategy)
				MarkStepSkipped(wf, types.StepAgentResolve)
				MarkStepSkipped(wf, types.StepAcceptChanges)
				AdvanceStep(wf, types.StepCommit, types.StepStatusFailed, result.ErrorMessage)
			} else {
				AdvanceStep(wf, types.StepFetch, types.StepStatusSuccess, "")
				AdvanceStep(wf, types.StepMerge, types.StepStatusSuccess, "")
				AdvanceStep(wf, types.StepCheckConflicts, types.StepStatusFailed, result.ErrorMessage)
			}
		}
		markWorkflowDone(wf, types.WorkflowFailed)
		return wf
	}

	return wf
}

