package workflow

import (
	"time"

	"github.com/loongxjin/forksync/engine/pkg/types"
)

// NewWorkflow creates a new SyncWorkflow with all steps initialized to pending.
func NewWorkflow(runID string) *types.SyncWorkflow {
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

func markStepSkipped(wf *types.SyncWorkflow, step types.WorkflowStep) {
	advanceStep(wf, step, types.StepStatusSkipped, "")
}

func markWorkflowDone(wf *types.SyncWorkflow, status types.WorkflowRunStatus) {
	if wf == nil {
		return
	}
	wf.Status = status
	now := types.Time{Time: time.Now()}
	wf.FinishedAt = &now
}

func isTerminalStepStatus(s types.WorkflowStepStatus) bool {
	return s == types.StepStatusSuccess || s == types.StepStatusFailed || s == types.StepStatusSkipped
}

// IsTerminalStepStatus returns true if the step status is a terminal state.
var IsTerminalStepStatus = isTerminalStepStatus

// AdvanceStep updates a step to the given status and sets timestamps.
// This is a free function for callers that need direct workflow step manipulation.
func AdvanceStep(wf *types.SyncWorkflow, step types.WorkflowStep, status types.WorkflowStepStatus, message string) {
	advanceStep(wf, step, status, message)
}

// MarkStepSkipped marks a step as skipped.
func MarkStepSkipped(wf *types.SyncWorkflow, step types.WorkflowStep) {
	advanceStep(wf, step, types.StepStatusSkipped, "")
}

// MarkWorkflowDone marks the workflow as completed (success or failed).
func MarkWorkflowDone(wf *types.SyncWorkflow, status types.WorkflowRunStatus) {
	markWorkflowDone(wf, status)
}

// FindStep returns the step record for the given step type, or nil if not found.
func FindStep(wf *types.SyncWorkflow, step types.WorkflowStep) *types.WorkflowStepRecord {
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
