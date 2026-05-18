package sync

import (
	"fmt"
	"strings"
	"time"

	"github.com/loongxjin/forksync/engine/internal/workflow"
	"github.com/loongxjin/forksync/engine/pkg/types"
)

// AdvanceStep updates a step to the given status and sets timestamps.
//
// Deprecated: Use workflow.Machine methods instead.
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
//
// Deprecated: Use workflow.Machine methods instead.
func MarkStepSkipped(wf *types.SyncWorkflow, step types.WorkflowStep) {
	AdvanceStep(wf, step, types.StepStatusSkipped, "")
}

// MarkWorkflowDone marks the workflow as completed (success or failed).
//
// Deprecated: Use workflow.Machine.Complete() or Machine.Fail() instead.
func MarkWorkflowDone(wf *types.SyncWorkflow, status types.WorkflowRunStatus) {
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

// newWorkflow creates a new SyncWorkflow with all steps initialized to pending.
func newWorkflow(runID string) *types.SyncWorkflow {
	return workflow.NewMachine(types.Repo{ID: runID}, nil).Workflow()
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
			MarkStepSkipped(wf, types.StepCheckConflicts)
			MarkStepSkipped(wf, types.StepResolveStrategy)
			MarkStepSkipped(wf, types.StepAgentResolve)
			MarkStepSkipped(wf, types.StepAcceptChanges)
			AdvanceStep(wf, types.StepCommit, types.StepStatusSuccess, "")
			MarkWorkflowDone(wf, types.WorkflowSuccess)
			return wf
		}

		AdvanceStep(wf, types.StepCheckConflicts, types.StepStatusSuccess, "")
		MarkStepSkipped(wf, types.StepResolveStrategy)
		MarkStepSkipped(wf, types.StepAgentResolve)
		MarkStepSkipped(wf, types.StepAcceptChanges)
		AdvanceStep(wf, types.StepCommit, types.StepStatusSuccess, "")
		MarkWorkflowDone(wf, types.WorkflowSuccess)
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
		MarkWorkflowDone(wf, types.WorkflowFailed)
		return wf
	}

	return wf
}
