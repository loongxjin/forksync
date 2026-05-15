package cmd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/loongxjin/forksync/engine/internal/agent"
	"github.com/loongxjin/forksync/engine/internal/git"
	"github.com/loongxjin/forksync/engine/internal/history"
	"github.com/loongxjin/forksync/engine/internal/logger"
	"github.com/loongxjin/forksync/engine/internal/repo"
	syncpkg "github.com/loongxjin/forksync/engine/internal/sync"
	"github.com/loongxjin/forksync/engine/pkg/types"
)

// workflowContinueResult is the response for resolve commands that produce workflow output.
type workflowContinueResult struct {
	RepoID   string              `json:"repoId"`
	RepoName string              `json:"repoName"`
	Status   types.RepoStatus    `json:"status"`
	Workflow *types.SyncWorkflow `json:"workflow,omitempty"`
}

// commitWorkflowParams contains the parameters for the commit-with-workflow pattern.
type commitWorkflowParams struct {
	commitMsg          string // fallback commit message
	skipAgentAndAccept bool   // whether to mark agent_resolve and accept_changes as skipped
	recordHistory      bool   // whether to record workflow completion history
	silentOutput       bool   // suppress outputWorkflowResult (used in --stream mode)
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
			if !params.silentOutput {
				outputWorkflowResult(r)
			}
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
	// Run post-sync commands (logs success/failure internally).
	syncpkg.RunPostSyncCommands(ctx, r)

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
			fmt.Sscanf(s.Message, "%d files have conflicts", &info.conflictsFound)
		case types.StepAgentResolve:
			if s.Status == types.StepStatusSuccess && s.Message != "" {
				info.agentUsed = strings.TrimPrefix(s.Message, "resolved by ")
				if info.agentUsed == s.Message {
					info.agentUsed = ""
				}
			}
		}
	}
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
