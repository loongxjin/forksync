package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/loongxjin/forksync/engine/internal/git"
	"github.com/loongxjin/forksync/engine/internal/logger"
	"github.com/loongxjin/forksync/engine/internal/repo"
	"github.com/loongxjin/forksync/engine/pkg/types"
	"github.com/spf13/cobra"
)

var workflowAction string

var workflowCmd = &cobra.Command{
	Use:   "workflow",
	Short: "Manage sync workflows",
}

var workflowContinueCmd = &cobra.Command{
	Use:   "continue <repo-name>",
	Short: "Continue a paused workflow",
	Long: `Continue a sync workflow that is paused at a decision point.

Actions:
  resolve_with_agent  — mark resolve_strategy as done, set agent_resolve to running
  abort               — abort the merge and end the workflow
  accept              — commit staged changes and complete the workflow
  reject              — abort the merge and end the workflow
  retry_commit        — retry committing staged changes
  continue_manual     — check if conflicts are resolved, then commit`,
	Args: cobra.ExactArgs(1),
	RunE: runWorkflowContinue,
}

func init() {
	workflowContinueCmd.Flags().StringVar(&workflowAction, "action", "", "action to perform (required)")
	_ = workflowContinueCmd.MarkFlagRequired("action")
	workflowCmd.AddCommand(workflowContinueCmd)
	rootCmd.AddCommand(workflowCmd)
}

// workflowContinueResult is the response for workflow continue.
type workflowContinueResult struct {
	RepoID   string             `json:"repoId"`
	RepoName string             `json:"repoName"`
	Status   types.RepoStatus   `json:"status"`
	Workflow *types.SyncWorkflow `json:"workflow,omitempty"`
}

func runWorkflowContinue(cmd *cobra.Command, args []string) error {
	_, cfgMgr := getSharedConfig()
	store := repo.NewJSONStore(cfgMgr.ConfigDir())
	if err := store.Load(); err != nil {
		return fmt.Errorf("load repo store: %w", err)
	}

	r, ok := store.GetByName(args[0])
	if !ok {
		return fmt.Errorf("repository %q not found", args[0])
	}

	ctx := cmd.Context()
	gitOps := git.NewOperations()

	switch workflowAction {
	case "resolve_with_agent":
		return handleResolveWithAgent(ctx, r, store)
	case "abort":
		return handleWorkflowAbort(ctx, r, store)
	case "accept":
		return handleWorkflowAccept(ctx, r, store)
	case "reject":
		return handleWorkflowReject(ctx, r, store)
	case "retry_commit":
		return handleWorkflowRetryCommit(ctx, r, store)
	case "continue_manual":
		return handleWorkflowContinueManual(ctx, r, store, gitOps)
	default:
		return fmt.Errorf("unknown action: %s", workflowAction)
	}
}

func handleResolveWithAgent(_ context.Context, r types.Repo, store repo.Store) error {
	wf := r.Workflow
	if wf == nil {
		wf = newWorkflowFromRepo(r)
	}
	advanceWorkflowStep(wf, types.StepResolveStrategy, types.StepStatusSuccess, "")
	advanceWorkflowStep(wf, types.StepAgentResolve, types.StepStatusRunning, "")
	r.Workflow = wf
	if err := store.Update(r); err != nil {
		logger.Error("workflow: failed to update repo", "repo", r.Name, "error", err)
	}
	outputWorkflowResult(r)
	return nil
}

func handleWorkflowAbort(ctx context.Context, r types.Repo, store repo.Store) error {
	gitOps := git.NewOperations()
	if err := gitOps.AbortMerge(ctx, r.Path); err != nil {
		logger.Warn("workflow: merge --abort failed", "repo", r.Name, "error", err)
	}
	wf := r.Workflow
	if wf == nil {
		wf = newWorkflowFromRepo(r)
	}
	// Mark all pending steps as skipped
	for i := range wf.Steps {
		if wf.Steps[i].Status == types.StepStatusPending {
			wf.Steps[i].Status = types.StepStatusSkipped
			now := types.Time{Time: time.Now()}
			wf.Steps[i].EndedAt = &now
		}
	}
	wf.Status = types.WorkflowFailed
	now := types.Time{Time: time.Now()}
	wf.FinishedAt = &now
	r.Workflow = wf
	r.Status = types.RepoStatusSyncNeeded
	r.ErrorMessage = ""
	if err := store.Update(r); err != nil {
		logger.Error("workflow: failed to update repo", "repo", r.Name, "error", err)
	}
	outputWorkflowResult(r)
	return nil
}

func handleWorkflowAccept(ctx context.Context, r types.Repo, store repo.Store) error {
	gitOps := git.NewOperations()
	if err := gitOps.StageAll(ctx, r.Path); err != nil {
		logger.Warn("workflow: stage all failed", "repo", r.Name, "error", err)
	}
	if err := gitOps.CommitNoEdit(ctx, r.Path); err != nil {
		if err2 := gitOps.Commit(ctx, r.Path, "Merge upstream changes (agent-resolved conflicts)"); err2 != nil {
			wf := r.Workflow
			if wf == nil {
				wf = newWorkflowFromRepo(r)
			}
			advanceWorkflowStep(wf, types.StepCommit, types.StepStatusFailed, fmt.Sprintf("commit failed: %v", err2))
			wf.Status = types.WorkflowFailed
			now := types.Time{Time: time.Now()}
			wf.FinishedAt = &now
			r.Workflow = wf
			r.Status = types.RepoStatusError
			r.ErrorMessage = fmt.Sprintf("commit failed: %v", err2)
			_ = store.Update(r)
			outputWorkflowResult(r)
			return nil
		}
	}
	wf := r.Workflow
	if wf == nil {
		wf = newWorkflowFromRepo(r)
	}
	advanceWorkflowStep(wf, types.StepAcceptChanges, types.StepStatusSuccess, "")
	advanceWorkflowStep(wf, types.StepCommit, types.StepStatusSuccess, "")
	wf.Status = types.WorkflowSuccess
	now := types.Time{Time: time.Now()}
	wf.FinishedAt = &now
	r.Workflow = wf
	r.Status = types.RepoStatusUpToDate
	r.ErrorMessage = ""
	now2 := types.Time{Time: time.Now()}
	r.LastSync = &now2
	if err := store.Update(r); err != nil {
		logger.Error("workflow: failed to update repo", "repo", r.Name, "error", err)
	}
	outputWorkflowResult(r)
	return nil
}

func handleWorkflowReject(ctx context.Context, r types.Repo, store repo.Store) error {
	gitOps := git.NewOperations()
	if err := gitOps.AbortMerge(ctx, r.Path); err != nil {
		logger.Warn("workflow: merge --abort failed", "repo", r.Name, "error", err)
	}
	wf := r.Workflow
	if wf == nil {
		wf = newWorkflowFromRepo(r)
	}
	for i := range wf.Steps {
		if wf.Steps[i].Status == types.StepStatusPending {
			wf.Steps[i].Status = types.StepStatusSkipped
			now := types.Time{Time: time.Now()}
			wf.Steps[i].EndedAt = &now
		}
	}
	wf.Status = types.WorkflowFailed
	now := types.Time{Time: time.Now()}
	wf.FinishedAt = &now
	r.Workflow = wf
	r.Status = types.RepoStatusSyncNeeded
	r.ErrorMessage = ""
	if err := store.Update(r); err != nil {
		logger.Error("workflow: failed to update repo", "repo", r.Name, "error", err)
	}
	outputWorkflowResult(r)
	return nil
}

func handleWorkflowRetryCommit(ctx context.Context, r types.Repo, store repo.Store) error {
	gitOps := git.NewOperations()
	if err := gitOps.StageAll(ctx, r.Path); err != nil {
		logger.Warn("workflow: stage all failed", "repo", r.Name, "error", err)
	}
	if err := gitOps.CommitNoEdit(ctx, r.Path); err != nil {
		if err2 := gitOps.Commit(ctx, r.Path, "Merge upstream changes (agent-resolved conflicts)"); err2 != nil {
			wf := r.Workflow
			if wf == nil {
				wf = newWorkflowFromRepo(r)
			}
			advanceWorkflowStep(wf, types.StepCommit, types.StepStatusFailed, fmt.Sprintf("commit failed: %v", err2))
			wf.Status = types.WorkflowFailed
			now := types.Time{Time: time.Now()}
			wf.FinishedAt = &now
			r.Workflow = wf
			r.Status = types.RepoStatusError
			r.ErrorMessage = fmt.Sprintf("commit failed: %v", err2)
			_ = store.Update(r)
			outputWorkflowResult(r)
			return nil
		}
	}
	wf := r.Workflow
	if wf == nil {
		wf = newWorkflowFromRepo(r)
	}
	advanceWorkflowStep(wf, types.StepCommit, types.StepStatusSuccess, "")
	wf.Status = types.WorkflowSuccess
	now := types.Time{Time: time.Now()}
	wf.FinishedAt = &now
	r.Workflow = wf
	r.Status = types.RepoStatusUpToDate
	r.ErrorMessage = ""
	now2 := types.Time{Time: time.Now()}
	r.LastSync = &now2
	if err := store.Update(r); err != nil {
		logger.Error("workflow: failed to update repo", "repo", r.Name, "error", err)
	}
	outputWorkflowResult(r)
	return nil
}

func handleWorkflowContinueManual(ctx context.Context, r types.Repo, store repo.Store, gitOps *git.Operations) error {
	remaining := gitOps.DetectConflicts(ctx, r.Path)
	if len(remaining) > 0 {
		wf := r.Workflow
		if wf == nil {
			wf = newWorkflowFromRepo(r)
		}
		advanceWorkflowStep(wf, types.StepResolveStrategy, types.StepStatusWaiting,
			fmt.Sprintf("%d conflicts still unresolved", len(remaining)))
		r.Workflow = wf
		_ = store.Update(r)
		outputWorkflowResult(r)
		return nil
	}

	mergeHead := filepath.Join(r.Path, ".git", "MERGE_HEAD")
	if _, err := os.Stat(mergeHead); err != nil {
		wf := r.Workflow
		if wf == nil {
			wf = newWorkflowFromRepo(r)
		}
		markStepSkippedInWorkflow(wf, types.StepAgentResolve)
		markStepSkippedInWorkflow(wf, types.StepAcceptChanges)
		advanceWorkflowStep(wf, types.StepCommit, types.StepStatusSuccess, "")
		wf.Status = types.WorkflowSuccess
		now := types.Time{Time: time.Now()}
		wf.FinishedAt = &now
		r.Workflow = wf
		r.Status = types.RepoStatusUpToDate
		r.ErrorMessage = ""
		now2 := types.Time{Time: time.Now()}
		r.LastSync = &now2
		_ = store.Update(r)
		outputWorkflowResult(r)
		return nil
	}

	if err := gitOps.StageAll(ctx, r.Path); err != nil {
		logger.Warn("workflow: stage all failed", "repo", r.Name, "error", err)
	}
	if err := gitOps.CommitNoEdit(ctx, r.Path); err != nil {
		if err2 := gitOps.Commit(ctx, r.Path, "Merge upstream changes (manual resolution)"); err2 != nil {
			wf := r.Workflow
			if wf == nil {
				wf = newWorkflowFromRepo(r)
			}
			advanceWorkflowStep(wf, types.StepCommit, types.StepStatusFailed, fmt.Sprintf("commit failed: %v", err2))
			wf.Status = types.WorkflowFailed
			now := types.Time{Time: time.Now()}
			wf.FinishedAt = &now
			r.Workflow = wf
			r.Status = types.RepoStatusError
			r.ErrorMessage = fmt.Sprintf("commit failed: %v", err2)
			_ = store.Update(r)
			outputWorkflowResult(r)
			return nil
		}
	}
	wf := r.Workflow
	if wf == nil {
		wf = newWorkflowFromRepo(r)
	}
	markStepSkippedInWorkflow(wf, types.StepAgentResolve)
	markStepSkippedInWorkflow(wf, types.StepAcceptChanges)
	advanceWorkflowStep(wf, types.StepCommit, types.StepStatusSuccess, "")
	wf.Status = types.WorkflowSuccess
	now := types.Time{Time: time.Now()}
	wf.FinishedAt = &now
	r.Workflow = wf
	r.Status = types.RepoStatusUpToDate
	r.ErrorMessage = ""
	now2 := types.Time{Time: time.Now()}
	r.LastSync = &now2
	if err := store.Update(r); err != nil {
		logger.Error("workflow: failed to update repo", "repo", r.Name, "error", err)
	}
	outputWorkflowResult(r)
	return nil
}

func outputWorkflowResult(r types.Repo) {
	if isJSON() {
		outputJSON(workflowContinueResult{
			RepoID:   r.ID,
			RepoName: r.Name,
			Status:   r.Status,
			Workflow: r.Workflow,
		}, nil)
	} else {
		outputText("Workflow updated for %s: %s", r.Name, r.Status)
	}
}

// newWorkflowFromRepo creates a minimal workflow for a repo that doesn't have one.
func newWorkflowFromRepo(r types.Repo) *types.SyncWorkflow {
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

func advanceWorkflowStep(wf *types.SyncWorkflow, step types.WorkflowStep, status types.WorkflowStepStatus, message string) {
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
			if status == types.StepStatusSuccess || status == types.StepStatusFailed || status == types.StepStatusSkipped {
				wf.Steps[i].EndedAt = &now
			}
			break
		}
	}
}

func markStepSkippedInWorkflow(wf *types.SyncWorkflow, step types.WorkflowStep) {
	advanceWorkflowStep(wf, step, types.StepStatusSkipped, "")
}
