package workflow

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/loongxjin/forksync/engine/internal/agent"
	"github.com/loongxjin/forksync/engine/internal/config"
	"github.com/loongxjin/forksync/engine/internal/git"
	"github.com/loongxjin/forksync/engine/internal/history"
	"github.com/loongxjin/forksync/engine/internal/logger"
	"github.com/loongxjin/forksync/engine/internal/repo"
	"github.com/loongxjin/forksync/engine/pkg/types"
)

// CommitParams contains the parameters for the commit pipeline.
type CommitParams struct {
	CommitMsg          string // fallback commit message
	SkipAgentAndAccept bool   // true for manual resolution
	RecordHistory      bool   // whether to record to history store
	SilentOutput       bool   // suppress output (used in --stream mode)
}

// CommitResult contains the outcome of the commit pipeline.
type CommitResult struct {
	Success   bool
	CommitErr string // non-empty if commit failed
}

// FinalizeCommit runs the full commit pipeline:
// StageAll → CommitNoEdit → (fallback) Commit → update workflow →
// post-sync commands → delete agent logs → record history.
func FinalizeCommit(
	ctx context.Context,
	r types.Repo,
	store repo.Store,
	gitOps git.OperationsProvider,
	cfg *config.Config,
	configDir string,
	params CommitParams,
) (CommitResult, error) {
	if err := gitOps.StageAll(ctx, r.Path); err != nil {
		logger.Warn("workflow: stage all failed", "repo", r.Name, "error", err)
	}
	if err := gitOps.CommitNoEdit(ctx, r.Path); err != nil {
		if err2 := gitOps.Commit(ctx, r.Path, params.CommitMsg); err2 != nil {
			wf := r.Workflow
			if wf == nil {
				wf = NewWorkflow(r.ID)
			}
			advanceStep(wf, types.StepCommit, types.StepStatusFailed, fmt.Sprintf("commit failed: %v", err2))
			wf.Status = types.WorkflowFailed
			now := types.Time{Time: time.Now()}
			wf.FinishedAt = &now
			r.Workflow = wf
			r.Status = types.RepoStatusError
			r.ErrorMessage = fmt.Sprintf("commit failed: %v", err2)
			_ = store.Update(r)
			return CommitResult{Success: false, CommitErr: fmt.Sprintf("commit failed: %v", err2)}, nil
		}
	}

	// Success path
	wf := r.Workflow
	if wf == nil {
		wf = NewWorkflow(r.ID)
	}
	if params.SkipAgentAndAccept {
		markStepSkipped(wf, types.StepAgentResolve)
		markStepSkipped(wf, types.StepAcceptChanges)
	} else {
		advanceStep(wf, types.StepAcceptChanges, types.StepStatusSuccess, "")
	}
	advanceStep(wf, types.StepCommit, types.StepStatusSuccess, "")
	wf.Status = types.WorkflowSuccess
	now := types.Time{Time: time.Now()}
	wf.FinishedAt = &now
	r.Workflow = wf
	r.Status = types.RepoStatusUpToDate
	r.ErrorMessage = ""
	r.LastSync = &now

	// Execute post-sync commands now that the merge is committed.
	RunPostSyncCommands(ctx, r)

	// Workflow completed — agent logs are no longer needed.
	if configDir != "" {
		agent.DeleteAllLogs(configDir, r.Name)
	}

	if err := store.Update(r); err != nil {
		logger.Error("workflow: failed to update repo", "repo", r.Name, "error", err)
	}

	if params.RecordHistory {
		recordWorkflowComplete(r, 0, cfg, configDir, store, gitOps)
	}

	return CommitResult{Success: true}, nil
}

// recordWorkflowComplete creates a new history record when a paused workflow is
// completed by the user (accept or manual resolution).
func recordWorkflowComplete(r types.Repo, commitsPulled int, cfg *config.Config, configDir string, store repo.Store, gitOps git.OperationsProvider) {
	histStore, err := history.NewStore(configDir)
	if err != nil {
		logger.Error("[workflow] open history store", "error", err)
		return
	}
	defer histStore.Close()

	info := workflowCompletionInfo(r.Workflow)
	oldHEAD := info.oldHEAD
	if oldHEAD == "" && r.Path != "" {
		if head, herr := gitOps.GetPreMergeHEAD(context.Background(), r.Path); herr == nil && head != "" {
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

	if cfg != nil && cfg.Sync.AutoSummary {
		latest, err := histStore.LatestByRepo(r.ID)
		if err == nil && latest.SummaryStatus == "" {
			if updateErr := histStore.UpdateSummary(latest.ID, "", string(types.SummaryStatusPending)); updateErr != nil {
				logger.Error("[workflow] update summary status", "error", updateErr)
			}
		}
	}
}

// workflowCompleteInfo groups the metadata extracted from a completed workflow.
type workflowCompleteInfo struct {
	autoResolved   int
	conflictsFound int
	agentUsed      string
	oldHEAD        string
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
