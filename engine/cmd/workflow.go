package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/loongxjin/forksync/engine/internal/git"
	"github.com/loongxjin/forksync/engine/internal/history"
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
	// Restore accept_changes from skipped to pending so it can be used later.
	for i := range wf.Steps {
		if wf.Steps[i].Step == types.StepAcceptChanges && wf.Steps[i].Status == types.StepStatusSkipped {
			wf.Steps[i].Status = types.StepStatusPending
			wf.Steps[i].Message = ""
			wf.Steps[i].EndedAt = nil
		}
	}
	wf.Status = types.WorkflowRunning
	r.Workflow = wf
	// Keep repo status as conflict so `forksync resolve` can pick it up.
	r.Status = types.RepoStatusConflict
	r.ErrorMessage = ""
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
	// User explicitly aborted — clear the workflow entirely rather than leaving
	// a failed record behind. The git state has already been rolled back.
	r.Workflow = nil
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
	autoResolved, conflictsFound, agentUsed, oldHEAD := workflowCompletionInfo(wf)
	recordWorkflowComplete(r.ID, r.Name, 0, autoResolved, conflictsFound, agentUsed, oldHEAD, r.Path)
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
	// User explicitly rejected — clear the workflow entirely rather than leaving
	// a failed record behind. The git state has already been rolled back.
	r.Workflow = nil
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
		autoResolved, conflictsFound, agentUsed, oldHEAD := workflowCompletionInfo(wf)
		recordWorkflowComplete(r.ID, r.Name, 0, autoResolved, conflictsFound, agentUsed, oldHEAD, r.Path)
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
	autoResolved, conflictsFound, agentUsed, oldHEAD := workflowCompletionInfo(wf)
	recordWorkflowComplete(r.ID, r.Name, 0, autoResolved, conflictsFound, agentUsed, oldHEAD, r.Path)
	outputWorkflowResult(r)
	return nil
}

// workflowCompletionInfo extracts completion metadata from a workflow.
func workflowCompletionInfo(wf *types.SyncWorkflow) (autoResolved int, conflictsFound int, agentUsed string, oldHEAD string) {
	if wf == nil {
		return
	}
	oldHEAD = wf.OldHEAD
	for _, s := range wf.Steps {
		switch s.Step {
		case types.StepCheckConflicts:
			// Message format: "N files have conflicts"
			fmt.Sscanf(s.Message, "%d files have conflicts", &conflictsFound)
		case types.StepAgentResolve:
			// Message format: "resolved by <agent>"
			if s.Status == types.StepStatusSuccess && s.Message != "" {
				agentUsed = strings.TrimPrefix(s.Message, "resolved by ")
				if agentUsed == s.Message {
					agentUsed = "" // not a "resolved by" message
				}
			}
		}
	}
	// autoResolved equals conflictsFound when agent resolved all
	if agentUsed != "" {
		autoResolved = conflictsFound
	}
	return
}

// recordWorkflowComplete creates a new history record when a paused workflow is
// completed by the user (accept or manual resolution). This replaces the old
// updateWorkflowHistoryToUpToDate which modified an intermediate record that
// no longer exists.
func recordWorkflowComplete(repoID, repoName string, commitsPulled, autoResolved, conflictsFound int, agentUsed, oldHEAD string, repoPath string) {
	_, cfgMgr := getSharedConfig()
	histStore, err := history.NewStore(cfgMgr.ConfigDir())
	if err != nil {
		logger.Error("[workflow] open history store", "error", err)
		return
	}
	defer histStore.Close()

	// If oldHEAD is missing (e.g. workflow created before the OldHEAD field was added),
	// try to recover it from git reflog (the commit before the merge commit).
	if oldHEAD == "" && repoPath != "" {
		gitOps := git.NewOperations()
		if head, err := gitOps.GetPreMergeHEAD(context.Background(), repoPath); err == nil && head != "" {
			oldHEAD = head
			logger.Debug("[workflow] recovered oldHEAD from reflog", "repo", repoName, "oldHEAD", oldHEAD)
		}
	}

	_, err = histStore.Record(history.Record{
		RepoID:         repoID,
		RepoName:       repoName,
		Status:         string(types.RepoStatusUpToDate),
		CommitsPulled:  commitsPulled,
		AutoResolved:   autoResolved,
		ConflictsFound: conflictsFound,
		AgentUsed:      agentUsed,
		OldHEAD:        oldHEAD,
		CreatedAt:      time.Now(),
	})
	if err != nil {
		logger.Error("[workflow] record completion history", "error", err)
		return
	}

	cfg, _ := getSharedConfig()
	if cfg != nil && cfg.Sync.AutoSummary {
		latest, err := histStore.LatestByRepo(repoID)
		if err == nil && latest.SummaryStatus == "" {
			if updateErr := histStore.UpdateSummary(latest.ID, "", string(types.SummaryStatusPending)); updateErr != nil {
				logger.Error("[workflow] update summary status", "error", updateErr)
			}
		}
	}
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
