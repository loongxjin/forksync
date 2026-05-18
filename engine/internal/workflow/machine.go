package workflow

import (
	"fmt"
	"time"

	"github.com/loongxjin/forksync/engine/internal/repo"
	"github.com/loongxjin/forksync/engine/pkg/types"
)

// Machine manages the lifecycle of a SyncWorkflow.
// It encapsulates all step transitions and persistence,
// so callers never manipulate steps directly.
type Machine struct {
	wf    *types.SyncWorkflow
	repo  types.Repo
	store repo.Store
}

// NewMachine creates a workflow machine for the given repo.
// If the repo already has an active workflow, it resumes it;
// otherwise it creates a fresh one.
func NewMachine(r types.Repo, store repo.Store) *Machine {
	wf := r.Workflow
	if wf == nil || wf.Status == "" {
		wf = newWorkflow(r.ID)
	}
	return &Machine{wf: wf, repo: r, store: store}
}

// Workflow returns the underlying SyncWorkflow.
func (m *Machine) Workflow() *types.SyncWorkflow {
	return m.wf
}

// Save persists the workflow to the repo store.
func (m *Machine) Save() {
	m.repo.Workflow = m.wf
	if m.store != nil {
		_ = m.store.Update(m.repo)
	}
}

// IsDone returns true if the workflow is in a terminal state.
func (m *Machine) IsDone() bool {
	return m.wf.Status == types.WorkflowSuccess || m.wf.Status == types.WorkflowFailed
}

// --- Basic lifecycle methods ---

// Start marks the workflow as running (called at sync start).
func (m *Machine) Start() {
	m.wf.Status = types.WorkflowRunning
}

// Complete sets the workflow status to Success and FinishedAt.
// Does NOT modify step statuses — callers must advance steps
// explicitly via AdvanceStep or scenario methods before calling Complete.
func (m *Machine) Complete() {
	m.wf.Status = types.WorkflowSuccess
	now := types.Time{Time: time.Now()}
	m.wf.FinishedAt = &now
}

// Fail marks the given step as Failed and the workflow as Failed.
func (m *Machine) Fail(step types.WorkflowStep, errMsg string) {
	advanceStep(m.wf, step, types.StepStatusFailed, errMsg)
	m.wf.Status = types.WorkflowFailed
	now := types.Time{Time: time.Now()}
	m.wf.FinishedAt = &now
}

// Wait marks the workflow as waiting for user action at the given step.
func (m *Machine) Wait(atStep types.WorkflowStep) {
	m.wf.Status = types.WorkflowWaiting
}

// --- Basic step advancement ---

// RunStep marks a step as Running and persists.
func (m *Machine) RunStep(step types.WorkflowStep) {
	advanceStep(m.wf, step, types.StepStatusRunning, "")
}

// AdvanceStep marks a step with the given status and message.
func (m *Machine) AdvanceStep(step types.WorkflowStep, status types.WorkflowStepStatus, msg string) {
	advanceStep(m.wf, step, status, msg)
}

// SkipRemaining marks all steps from `from` onward as Skipped.
func (m *Machine) SkipRemaining(from types.WorkflowStep) {
	skipping := false
	for i := range m.wf.Steps {
		if m.wf.Steps[i].Step == from {
			skipping = true
		}
		if skipping && m.wf.Steps[i].Status == types.StepStatusPending {
			advanceStep(m.wf, m.wf.Steps[i].Step, types.StepStatusSkipped, "")
		}
	}
}

// CommitSuccess marks the Commit step as Success and completes the workflow.
// Convenience method for the common "commit succeeded, all done" pattern.
func (m *Machine) CommitSuccess() {
	advanceStep(m.wf, types.StepCommit, types.StepStatusSuccess, "")
	m.Complete()
}

// --- Core scenario methods (6) ---

// MarkCleanSync advances fetch+merge+checkConflicts to Success,
// skips resolve_strategy + agent_resolve + accept_changes,
// sets commit to Running. Caller should call CommitSuccess() after commit.
func (m *Machine) MarkCleanSync() {
	advanceStep(m.wf, types.StepFetch, types.StepStatusSuccess, "")
	advanceStep(m.wf, types.StepMerge, types.StepStatusSuccess, "")
	advanceStep(m.wf, types.StepCheckConflicts, types.StepStatusSuccess, "")
	advanceStep(m.wf, types.StepResolveStrategy, types.StepStatusSkipped, "")
	advanceStep(m.wf, types.StepAgentResolve, types.StepStatusSkipped, "")
	advanceStep(m.wf, types.StepAcceptChanges, types.StepStatusSkipped, "")
	advanceStep(m.wf, types.StepCommit, types.StepStatusRunning, "")
}

// MarkConflictDetected advances through fetch+merge+checkConflicts,
// sets resolve_strategy to Waiting, skips agent_resolve and accept_changes.
func (m *Machine) MarkConflictDetected(conflictCount int, files []string) {
	advanceStep(m.wf, types.StepFetch, types.StepStatusSuccess, "")
	advanceStep(m.wf, types.StepMerge, types.StepStatusSuccess, "")
	advanceStep(m.wf, types.StepCheckConflicts, types.StepStatusSuccess,
		fmt.Sprintf("%d files have conflicts", conflictCount))
	advanceStep(m.wf, types.StepResolveStrategy, types.StepStatusWaiting, "")
	advanceStep(m.wf, types.StepAgentResolve, types.StepStatusSkipped, "")
	advanceStep(m.wf, types.StepAcceptChanges, types.StepStatusSkipped, "")
	m.Wait(types.StepResolveStrategy)
}

// MarkAgentResolving advances resolve_strategy=Success, agent_resolve=Running.
func (m *Machine) MarkAgentResolving() {
	advanceStep(m.wf, types.StepResolveStrategy, types.StepStatusSuccess, "")
	advanceStep(m.wf, types.StepAgentResolve, types.StepStatusRunning, "")
}

// MarkAgentResolved advances agent_resolve=Success, accept_changes=Waiting,
// workflow Waiting.
func (m *Machine) MarkAgentResolved(agentName string) {
	advanceStep(m.wf, types.StepAgentResolve, types.StepStatusSuccess,
		fmt.Sprintf("resolved by %s", agentName))
	advanceStep(m.wf, types.StepAcceptChanges, types.StepStatusWaiting, "")
	m.Wait(types.StepAcceptChanges)
}

// MarkAgentAutoCommitted advances agent_resolve=Success,
// skips accept_changes, calls CommitSuccess(). Workflow completed.
func (m *Machine) MarkAgentAutoCommitted(agentName string) {
	advanceStep(m.wf, types.StepAgentResolve, types.StepStatusSuccess,
		fmt.Sprintf("resolved by %s", agentName))
	advanceStep(m.wf, types.StepAcceptChanges, types.StepStatusSkipped, "")
	m.CommitSuccess()
}

// MarkManualPath advances resolve_strategy=Waiting,
// skips agent_resolve and accept_changes, workflow Waiting.
func (m *Machine) MarkManualPath() {
	advanceStep(m.wf, types.StepResolveStrategy, types.StepStatusWaiting, "")
	advanceStep(m.wf, types.StepAgentResolve, types.StepStatusSkipped, "")
	advanceStep(m.wf, types.StepAcceptChanges, types.StepStatusSkipped, "")
	m.Wait(types.StepResolveStrategy)
}

// Reject aborts the workflow: marks all pending steps as skipped, marks workflow Failed.
func (m *Machine) Reject() {
	for i := range m.wf.Steps {
		if m.wf.Steps[i].Status == types.StepStatusPending || m.wf.Steps[i].Status == types.StepStatusRunning || m.wf.Steps[i].Status == types.StepStatusWaiting {
			advanceStep(m.wf, m.wf.Steps[i].Step, types.StepStatusSkipped, "")
		}
	}
	m.wf.Status = types.WorkflowFailed
	now := types.Time{Time: time.Now()}
	m.wf.FinishedAt = &now
}

// --- Internal helpers (migrated from sync/workflow.go) ---

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

// IsTerminalStepStatus returns true if the step status is a terminal state.
// Exported for compatibility.
var IsTerminalStepStatus = isTerminalStepStatus

// AdvanceStep updates a step to the given status and sets timestamps.
// This is a free function for callers that need direct workflow step manipulation
// without creating a Machine.
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
