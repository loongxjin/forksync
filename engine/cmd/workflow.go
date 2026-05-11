package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/loongxjin/forksync/engine/internal/agent"
	"github.com/loongxjin/forksync/engine/internal/config"
	"github.com/loongxjin/forksync/engine/internal/git"
	"github.com/loongxjin/forksync/engine/internal/history"
	"github.com/loongxjin/forksync/engine/internal/logger"
	"github.com/loongxjin/forksync/engine/internal/repo"
	syncpkg "github.com/loongxjin/forksync/engine/internal/sync"
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
	RepoID   string              `json:"repoId"`
	RepoName string              `json:"repoName"`
	Status   types.RepoStatus    `json:"status"`
	Workflow *types.SyncWorkflow `json:"workflow,omitempty"`
}

func runWorkflowContinue(cmd *cobra.Command, args []string) error {
	store, err := loadRepoStore()
	if err != nil {
		return err
	}

	r, ok := store.GetByName(args[0])
	if !ok {
		return fmt.Errorf("repository %q not found", args[0])
	}

	ctx := cmd.Context()
	cfg, _ := getSharedConfig()
	gitOps := newGitOps(cfg)

	switch workflowAction {
	case "resolve_with_agent":
		return handleResolveWithAgent(ctx, r, store)
	case "abort":
		return handleWorkflowAbortOrReject(ctx, r, store, cfg)
	case "accept":
		return handleWorkflowAccept(ctx, r, store, cfg)
	case "reject":
		return handleWorkflowAbortOrReject(ctx, r, store, cfg)
	case "retry_commit":
		return handleWorkflowRetryCommit(ctx, r, store, cfg)
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
	syncpkg.AdvanceStep(wf, types.StepResolveStrategy, types.StepStatusSuccess, "")
	syncpkg.AdvanceStep(wf, types.StepAgentResolve, types.StepStatusRunning, "")
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

// handleWorkflowAbortOrReject handles both abort and reject actions which share
// the same logic: abort the merge, clear the workflow, and reset status.
func handleWorkflowAbortOrReject(ctx context.Context, r types.Repo, store repo.Store, cfg *config.Config) error {
	_, cfgMgr := getSharedConfig()
	agent.DeleteAllLogs(cfgMgr.ConfigDir(), r.Name)

	gitOps := newGitOps(cfg)
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
	// User explicitly aborted/rejected — clear the workflow entirely rather than leaving
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

// commitWorkflowParams contains the parameters for the commit-with-workflow pattern.
type commitWorkflowParams struct {
	commitMsg          string // fallback commit message
	skipAgentAndAccept bool   // whether to mark agent_resolve and accept_changes as skipped
	recordHistory      bool   // whether to record workflow completion history
}

func handleWorkflowAccept(ctx context.Context, r types.Repo, store repo.Store, cfg *config.Config) error {
	gitOps := newGitOps(cfg)
	return finalizeCommitWithWorkflow(ctx, r, store, gitOps, commitWorkflowParams{
		commitMsg:          types.CommitMsgAgentResolved,
		skipAgentAndAccept: false,
		recordHistory:      true,
	})
}

func handleWorkflowRetryCommit(ctx context.Context, r types.Repo, store repo.Store, cfg *config.Config) error {
	gitOps := newGitOps(cfg)
	return finalizeCommitWithWorkflow(ctx, r, store, gitOps, commitWorkflowParams{
		commitMsg:          types.CommitMsgAgentResolved,
		skipAgentAndAccept: false,
		recordHistory:      false,
	})
}

func handleWorkflowContinueManual(ctx context.Context, r types.Repo, store repo.Store, gitOps *git.Operations) error {
	remaining := gitOps.DetectConflicts(ctx, r.Path)
	if len(remaining) > 0 {
		wf := r.Workflow
		if wf == nil {
			wf = newWorkflowFromRepo(r)
		}
		syncpkg.AdvanceStep(wf, types.StepResolveStrategy, types.StepStatusWaiting,
			fmt.Sprintf("%d conflicts still unresolved", len(remaining)))
		r.Workflow = wf
		_ = store.Update(r)
		outputWorkflowResult(r)
		return nil
	}

	mergeHead := filepath.Join(r.Path, ".git", "MERGE_HEAD")
	if _, err := os.Stat(mergeHead); err != nil {
		// No merge in progress — already clean
		wf := r.Workflow
		if wf == nil {
			wf = newWorkflowFromRepo(r)
		}
		syncpkg.MarkStepSkipped(wf, types.StepAgentResolve)
		syncpkg.MarkStepSkipped(wf, types.StepAcceptChanges)
		syncpkg.AdvanceStep(wf, types.StepCommit, types.StepStatusSuccess, "")
		wf.Status = types.WorkflowSuccess
		now := types.Time{Time: time.Now()}
		wf.FinishedAt = &now
		r.Workflow = wf
		r.Status = types.RepoStatusUpToDate
		r.ErrorMessage = ""
		r.LastSync = &now
		_ = store.Update(r)
		info := workflowCompletionInfo(wf)
		recordWorkflowComplete(r, 0, info)
		outputWorkflowResult(r)
		return nil
	}

	// MERGE_HEAD exists — stage and commit
	return finalizeCommitWithWorkflow(ctx, r, store, gitOps, commitWorkflowParams{
		commitMsg:          types.CommitMsgManualResolved,
		skipAgentAndAccept: true,
		recordHistory:      true,
	})
}

// finalizeCommitWithWorkflow handles the common pattern: stage → CommitNoEdit → fallback Commit →
// update workflow → update status. This is the shared implementation for accept, retry_commit,
// and continue_manual actions.
func finalizeCommitWithWorkflow(ctx context.Context, r types.Repo, store repo.Store, gitOps *git.Operations, params commitWorkflowParams) error {
	if err := gitOps.StageAll(ctx, r.Path); err != nil {
		logger.Warn("workflow: stage all failed", "repo", r.Name, "error", err)
	}
	if err := gitOps.CommitNoEdit(ctx, r.Path); err != nil {
		if err2 := gitOps.Commit(ctx, r.Path, params.commitMsg); err2 != nil {
			wf := r.Workflow
			if wf == nil {
				wf = newWorkflowFromRepo(r)
			}
			syncpkg.AdvanceStep(wf, types.StepCommit, types.StepStatusFailed, fmt.Sprintf("commit failed: %v", err2))
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

	// Success path
	wf := r.Workflow
	if wf == nil {
		wf = newWorkflowFromRepo(r)
	}
	if params.skipAgentAndAccept {
		syncpkg.MarkStepSkipped(wf, types.StepAgentResolve)
		syncpkg.MarkStepSkipped(wf, types.StepAcceptChanges)
	} else {
		syncpkg.AdvanceStep(wf, types.StepAcceptChanges, types.StepStatusSuccess, "")
	}
	syncpkg.AdvanceStep(wf, types.StepCommit, types.StepStatusSuccess, "")
	wf.Status = types.WorkflowSuccess
	now := types.Time{Time: time.Now()}
	wf.FinishedAt = &now
	r.Workflow = wf
	r.Status = types.RepoStatusUpToDate
	r.ErrorMessage = ""
	r.LastSync = &now

	// Execute post-sync commands now that the merge is committed.
	results := syncpkg.RunPostSyncCommands(ctx, r)
	if err := syncpkg.PostSyncError(results); err != "" {
		logger.Error("workflow: post-sync command failed", "repo", r.Name, "error", err)
	}

	// Workflow completed — agent logs are no longer needed.
	_, cfgMgr := getSharedConfig()
	agent.DeleteAllLogs(cfgMgr.ConfigDir(), r.Name)

	if err := store.Update(r); err != nil {
		logger.Error("workflow: failed to update repo", "repo", r.Name, "error", err)
	}
	if params.recordHistory {
		info := workflowCompletionInfo(wf)
		recordWorkflowComplete(r, 0, info)
	}
	outputWorkflowResult(r)
	return nil
}

// workflowCompletionInfo extracts completion metadata from a workflow.
func workflowCompletionInfo(wf *types.SyncWorkflow) (info workflowCompleteInfo) {
	if wf == nil {
		return
	}
	info.oldHEAD = wf.OldHEAD
	for _, s := range wf.Steps {
		switch s.Step {
		case types.StepCheckConflicts:
			// Message format: "N files have conflicts"
			fmt.Sscanf(s.Message, "%d files have conflicts", &info.conflictsFound)
		case types.StepAgentResolve:
			// Message format: "resolved by <agent>"
			if s.Status == types.StepStatusSuccess && s.Message != "" {
				info.agentUsed = strings.TrimPrefix(s.Message, "resolved by ")
				if info.agentUsed == s.Message {
					info.agentUsed = "" // not a "resolved by" message
				}
			}
		}
	}
	// autoResolved equals conflictsFound when agent resolved all
	if info.agentUsed != "" {
		info.autoResolved = info.conflictsFound
	}
	return
}

// workflowCompleteInfo groups the metadata extracted from a completed workflow.
type workflowCompleteInfo struct {
	autoResolved   int
	conflictsFound int
	agentUsed      string
	oldHEAD        string
}

// recordWorkflowComplete creates a new history record when a paused workflow is
// completed by the user (accept or manual resolution).
func recordWorkflowComplete(r types.Repo, commitsPulled int, info workflowCompleteInfo) {
	_, cfgMgr := getSharedConfig()
	histStore, err := history.NewStore(cfgMgr.ConfigDir())
	if err != nil {
		logger.Error("[workflow] open history store", "error", err)
		return
	}
	defer histStore.Close()

	oldHEAD := info.oldHEAD
	// If oldHEAD is missing (e.g. workflow created before the OldHEAD field was added),
	// try to recover it from git reflog (the commit before the merge commit).
	if oldHEAD == "" && r.Path != "" {
		cfg, _ := getSharedConfig()
		gitOps := newGitOps(cfg)
		if head, err := gitOps.GetPreMergeHEAD(context.Background(), r.Path); err == nil && head != "" {
			oldHEAD = head
			logger.Debug("[workflow] recovered oldHEAD from reflog", "repo", r.Name, "oldHEAD", oldHEAD)
		}
	}

	_, err = histStore.Insert(history.Record{
		RepoID:         r.ID,
		RepoName:       r.Name,
		Status:         string(types.RepoStatusUpToDate),
		CommitsPulled:  commitsPulled,
		AutoResolved:   info.autoResolved,
		ConflictsFound: info.conflictsFound,
		AgentUsed:      info.agentUsed,
		OldHEAD:        oldHEAD,
		CreatedAt:      time.Now(),
	})
	if err != nil {
		logger.Error("[workflow] record completion history", "error", err)
		return
	}

	cfg, _ := getSharedConfig()
	if cfg != nil && cfg.Sync.AutoSummary {
		latest, err := histStore.LatestByRepo(r.ID)
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
