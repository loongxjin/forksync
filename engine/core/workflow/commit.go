package workflow

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/loongxjin/forksync/engine/core/config"
	"github.com/loongxjin/forksync/engine/core/git"
	"github.com/loongxjin/forksync/engine/core/history"
	"github.com/loongxjin/forksync/engine/core/logger"
	"github.com/loongxjin/forksync/engine/core/repo"
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
//
// histStore is the shared history store used to record the completion. Pass
// nil to open (and close) a one-shot store here — prefer passing the shared
// store from Deps so concurrent syncer/handler writes don't race for the
// SQLite write lock.
func FinalizeCommit(
	ctx context.Context,
	r types.Repo,
	store repo.Store,
	gitOps git.OperationsProvider,
	cfg *config.Config,
	configDir string,
	histStore *history.Store,
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

	if err := store.Update(r); err != nil {
		logger.Error("workflow: failed to update repo", "repo", r.Name, "error", err)
	}

	if params.RecordHistory {
		recordWorkflowComplete(r, 0, cfg, configDir, store, gitOps, histStore)
	}

	return CommitResult{Success: true}, nil
}

// recordWorkflowComplete creates a new history record when a paused workflow is
// completed by the user (accept or manual resolution).
//
// histStore is the shared store to write through. When nil, a one-shot store
// is opened (and closed) here as a fallback.
func recordWorkflowComplete(r types.Repo, commitsPulled int, cfg *config.Config, configDir string, store repo.Store, gitOps git.OperationsProvider, histStore *history.Store) {
	closeAfter := false
	if histStore == nil {
		hs, err := history.NewStore(configDir)
		if err != nil {
			logger.Error("[workflow] open history store", "error", err)
			return
		}
		histStore = hs
		closeAfter = true
	}
	if closeAfter {
		defer histStore.Close()
	}

	info := workflowCompletionInfo(r.Workflow)
	oldHEAD := info.oldHEAD
	if oldHEAD == "" && r.Path != "" {
		if head, herr := gitOps.GetPreMergeHEAD(context.Background(), r.Path); herr == nil && head != "" {
			oldHEAD = head
			logger.Debug("[workflow] recovered oldHEAD from reflog", "repo", r.Name, "oldHEAD", oldHEAD)
		}
	}

	_, err := histStore.Insert(history.Record{
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

// AcceptCommit checks for remaining conflicts, then finalizes the commit.
// It is the single "Resolve Commit" entry point, moved here from the Resolver
// type per CONTEXT.md ("Accept is Resolve Commit, not part of Resolve").
//
// histStore is the shared history store forwarded to FinalizeCommit; pass nil
// to fall back to a one-shot store inside the workflow.
func AcceptCommit(ctx context.Context, repo types.Repo, store repo.Store, gitOps git.OperationsProvider, cfg *config.Config, configDir string, histStore *history.Store, manual bool, retry bool) (types.Repo, CommitResult, error) {
	remaining := gitOps.DetectConflicts(ctx, repo.Path)
	if len(remaining) > 0 {
		repo.Status = types.RepoStatusConflict
		repo.ErrorMessage = fmt.Sprintf("%d conflicts still unresolved", len(remaining))
		return repo, CommitResult{}, fmt.Errorf("%d conflicts still unresolved", len(remaining))
	}

	// Check if we're in a merge state
	mergeHead := filepath.Join(repo.Path, ".git", "MERGE_HEAD")
	if _, err := os.Stat(mergeHead); err != nil {
		repo.Status = types.RepoStatusUpToDate
		repo.ErrorMessage = ""
		if updateErr := store.Update(repo); updateErr != nil {
			logger.Error("resolve: failed to update repo after accept-no-merge", "repo", repo.Name, "error", updateErr)
		}
		return repo, CommitResult{Success: true}, nil
	}

	commitMsg := types.CommitMsgAgentResolved
	if manual {
		commitMsg = types.CommitMsgManualResolved
	}

	result, err := FinalizeCommit(ctx, repo, store, gitOps, cfg, configDir, histStore, CommitParams{
		CommitMsg:          commitMsg,
		SkipAgentAndAccept: manual,
		RecordHistory:      !retry,
	})

	// Reload repo from store to get updated state
	updated, ok := store.Get(repo.ID)
	if ok {
		repo = updated
	}

	return repo, result, err
}
