package workflow

import (
	"testing"

	"github.com/loongxjin/forksync/engine/pkg/types"
)

func TestNewMachine(t *testing.T) {
	r := types.Repo{ID: "test-repo"}
	m := NewMachine(r, nil)

	if m == nil {
		t.Fatal("NewMachine returned nil")
	}
	wf := m.Workflow()
	if wf == nil {
		t.Fatal("Workflow is nil")
	}
	if wf.RunID != "test-repo" {
		t.Errorf("expected RunID 'test-repo', got %q", wf.RunID)
	}
	if wf.Status != types.WorkflowRunning {
		t.Errorf("expected status Running, got %q", wf.Status)
	}
	if len(wf.Steps) != 7 {
		t.Errorf("expected 7 steps, got %d", len(wf.Steps))
	}
}

func TestNewMachine_ResumesExisting(t *testing.T) {
	existingWF := &types.SyncWorkflow{
		RunID:  "test-repo",
		Status: types.WorkflowWaiting,
		Steps: []types.WorkflowStepRecord{
			{Step: types.StepFetch, Status: types.StepStatusSuccess},
			{Step: types.StepResolveStrategy, Status: types.StepStatusWaiting},
		},
	}
	r := types.Repo{ID: "test-repo", Workflow: existingWF}
	m := NewMachine(r, nil)

	if m.Workflow() != existingWF {
		t.Error("expected to resume existing workflow")
	}
}

func TestStart(t *testing.T) {
	r := types.Repo{ID: "test"}
	m := NewMachine(r, nil)
	m.Start()
	if m.Workflow().Status != types.WorkflowRunning {
		t.Error("expected Running status")
	}
}

func TestComplete(t *testing.T) {
	r := types.Repo{ID: "test"}
	m := NewMachine(r, nil)
	m.Complete()
	wf := m.Workflow()
	if wf.Status != types.WorkflowSuccess {
		t.Errorf("expected Success, got %q", wf.Status)
	}
	if wf.FinishedAt == nil {
		t.Error("expected FinishedAt to be set")
	}
}

func TestFail(t *testing.T) {
	r := types.Repo{ID: "test"}
	m := NewMachine(r, nil)
	m.Fail(types.StepFetch, "fetch failed")
	wf := m.Workflow()
	if wf.Status != types.WorkflowFailed {
		t.Errorf("expected Failed, got %q", wf.Status)
	}
	if wf.FinishedAt == nil {
		t.Error("expected FinishedAt to be set")
	}
	// Check that the step was marked failed
	step := findStepInWorkflow(wf, types.StepFetch)
	if step == nil || step.Status != types.StepStatusFailed {
		t.Error("expected StepFetch to be failed")
	}
	if step.Message != "fetch failed" {
		t.Errorf("expected message 'fetch failed', got %q", step.Message)
	}
}

func TestAdvanceStep(t *testing.T) {
	r := types.Repo{ID: "test"}
	m := NewMachine(r, nil)
	m.AdvanceStep(types.StepFetch, types.StepStatusSuccess, "")
	wf := m.Workflow()
	step := findStepInWorkflow(wf, types.StepFetch)
	if step == nil || step.Status != types.StepStatusSuccess {
		t.Error("expected StepFetch to be success")
	}
}

func TestSkipRemaining(t *testing.T) {
	r := types.Repo{ID: "test"}
	m := NewMachine(r, nil)
	// Advance fetch and merge to success
	m.AdvanceStep(types.StepFetch, types.StepStatusSuccess, "")
	m.AdvanceStep(types.StepMerge, types.StepStatusSuccess, "")
	// Skip from resolve_strategy onward
	m.SkipRemaining(types.StepResolveStrategy)
	wf := m.Workflow()
	for _, step := range wf.Steps {
		if step.Step == types.StepFetch || step.Step == types.StepMerge {
			if step.Status != types.StepStatusSuccess {
				t.Errorf("expected %s to be success, got %q", step.Step, step.Status)
			}
		}
		if step.Step == types.StepResolveStrategy || step.Step == types.StepAgentResolve ||
			step.Step == types.StepAcceptChanges || step.Step == types.StepCommit {
			if step.Status != types.StepStatusSkipped {
				t.Errorf("expected %s to be skipped, got %q", step.Step, step.Status)
			}
		}
	}
}

func TestCommitSuccess(t *testing.T) {
	r := types.Repo{ID: "test"}
	m := NewMachine(r, nil)
	m.CommitSuccess()
	wf := m.Workflow()
	if wf.Status != types.WorkflowSuccess {
		t.Errorf("expected Success, got %q", wf.Status)
	}
	step := findStepInWorkflow(wf, types.StepCommit)
	if step == nil || step.Status != types.StepStatusSuccess {
		t.Error("expected StepCommit to be success")
	}
}

func TestMarkCleanSync(t *testing.T) {
	r := types.Repo{ID: "test"}
	m := NewMachine(r, nil)
	m.MarkCleanSync()
	wf := m.Workflow()

	expected := []struct {
		step   types.WorkflowStep
		status types.WorkflowStepStatus
	}{
		{types.StepFetch, types.StepStatusSuccess},
		{types.StepMerge, types.StepStatusSuccess},
		{types.StepCheckConflicts, types.StepStatusSuccess},
		{types.StepResolveStrategy, types.StepStatusSkipped},
		{types.StepAgentResolve, types.StepStatusSkipped},
		{types.StepAcceptChanges, types.StepStatusSkipped},
		{types.StepCommit, types.StepStatusRunning},
	}
	for _, exp := range expected {
		step := findStepInWorkflow(wf, exp.step)
		if step == nil || step.Status != exp.status {
			t.Errorf("expected %s to be %q, got %q", exp.step, exp.status, step.Status)
		}
	}
}

func TestMarkConflictDetected(t *testing.T) {
	r := types.Repo{ID: "test"}
	m := NewMachine(r, nil)
	m.MarkConflictDetected(3, []string{"file1.txt", "file2.txt", "file3.txt"})
	wf := m.Workflow()

	if wf.Status != types.WorkflowWaiting {
		t.Errorf("expected Waiting, got %q", wf.Status)
	}
	step := findStepInWorkflow(wf, types.StepResolveStrategy)
	if step == nil || step.Status != types.StepStatusWaiting {
		t.Error("expected resolve_strategy to be waiting")
	}
	ccStep := findStepInWorkflow(wf, types.StepCheckConflicts)
	if ccStep == nil || ccStep.Message != "3 files have conflicts" {
		t.Errorf("expected '3 files have conflicts', got %q", ccStep.Message)
	}
}

func TestMarkAgentResolving(t *testing.T) {
	r := types.Repo{ID: "test"}
	m := NewMachine(r, nil)
	m.MarkAgentResolving()
	wf := m.Workflow()
	rs := findStepInWorkflow(wf, types.StepResolveStrategy)
	if rs == nil || rs.Status != types.StepStatusSuccess {
		t.Error("expected resolve_strategy to be success")
	}
	ar := findStepInWorkflow(wf, types.StepAgentResolve)
	if ar == nil || ar.Status != types.StepStatusRunning {
		t.Error("expected agent_resolve to be running")
	}
}

func TestMarkAgentResolved(t *testing.T) {
	r := types.Repo{ID: "test"}
	m := NewMachine(r, nil)
	m.MarkAgentResolved("claude")
	wf := m.Workflow()
	if wf.Status != types.WorkflowWaiting {
		t.Errorf("expected Waiting, got %q", wf.Status)
	}
	ar := findStepInWorkflow(wf, types.StepAgentResolve)
	if ar == nil || ar.Status != types.StepStatusSuccess {
		t.Error("expected agent_resolve to be success")
	}
	if ar.Message != "resolved by claude" {
		t.Errorf("expected 'resolved by claude', got %q", ar.Message)
	}
	ac := findStepInWorkflow(wf, types.StepAcceptChanges)
	if ac == nil || ac.Status != types.StepStatusWaiting {
		t.Error("expected accept_changes to be waiting")
	}
}

func TestMarkAgentAutoCommitted(t *testing.T) {
	r := types.Repo{ID: "test"}
	m := NewMachine(r, nil)
	m.MarkAgentAutoCommitted("claude")
	wf := m.Workflow()
	if wf.Status != types.WorkflowSuccess {
		t.Errorf("expected Success, got %q", wf.Status)
	}
	ac := findStepInWorkflow(wf, types.StepAcceptChanges)
	if ac == nil || ac.Status != types.StepStatusSkipped {
		t.Error("expected accept_changes to be skipped")
	}
	commit := findStepInWorkflow(wf, types.StepCommit)
	if commit == nil || commit.Status != types.StepStatusSuccess {
		t.Error("expected commit to be success")
	}
}

func TestMarkManualPath(t *testing.T) {
	r := types.Repo{ID: "test"}
	m := NewMachine(r, nil)
	m.MarkManualPath()
	wf := m.Workflow()
	if wf.Status != types.WorkflowWaiting {
		t.Errorf("expected Waiting, got %q", wf.Status)
	}
	rs := findStepInWorkflow(wf, types.StepResolveStrategy)
	if rs == nil || rs.Status != types.StepStatusWaiting {
		t.Error("expected resolve_strategy to be waiting")
	}
	ar := findStepInWorkflow(wf, types.StepAgentResolve)
	if ar == nil || ar.Status != types.StepStatusSkipped {
		t.Error("expected agent_resolve to be skipped")
	}
}

func TestReject(t *testing.T) {
	r := types.Repo{ID: "test"}
	m := NewMachine(r, nil)
	// Advance some steps first
	m.AdvanceStep(types.StepFetch, types.StepStatusSuccess, "")
	m.AdvanceStep(types.StepMerge, types.StepStatusSuccess, "")
	m.Reject()
	wf := m.Workflow()
	if wf.Status != types.WorkflowFailed {
		t.Errorf("expected Failed, got %q", wf.Status)
	}
	if wf.FinishedAt == nil {
		t.Error("expected FinishedAt to be set")
	}
	// All non-success steps should be skipped
	for _, step := range wf.Steps {
		if step.Step == types.StepFetch || step.Step == types.StepMerge {
			if step.Status != types.StepStatusSuccess {
				t.Errorf("expected %s to remain success", step.Step)
			}
		} else {
			if step.Status != types.StepStatusSkipped {
				t.Errorf("expected %s to be skipped, got %q", step.Step, step.Status)
			}
		}
	}
}

func TestIsDone(t *testing.T) {
	r := types.Repo{ID: "test"}
	m := NewMachine(r, nil)
	if m.IsDone() {
		t.Error("new machine should not be done")
	}
	m.Complete()
	if !m.IsDone() {
		t.Error("completed machine should be done")
	}
}

// Helper for tests
func findStepInWorkflow(wf *types.SyncWorkflow, step types.WorkflowStep) *types.WorkflowStepRecord {
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
