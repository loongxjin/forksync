package cmd

import (
	"context"

	"github.com/loongxjin/forksync/engine/internal/git"
	"github.com/loongxjin/forksync/engine/internal/repo"
	"github.com/loongxjin/forksync/engine/internal/workflow"
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
func finalizeCommitWithWorkflow(ctx context.Context, r types.Repo, store repo.Store, gitOps git.OperationsProvider, params commitWorkflowParams) error {
	_, cfgMgr := getSharedConfig()
	cfg, _ := getSharedConfig()

	result, err := workflow.FinalizeCommit(ctx, r, store, gitOps, cfg, cfgMgr.ConfigDir(), workflow.CommitParams{
		CommitMsg:          params.commitMsg,
		SkipAgentAndAccept: params.skipAgentAndAccept,
		RecordHistory:      params.recordHistory,
		SilentOutput:       params.silentOutput,
	})
	if err != nil {
		return err
	}

	// The workflow.FinalizeCommit already updated the repo in the store.
	// We need to retrieve the updated repo for output.
	updated, ok := store.Get(r.ID)
	if ok {
		r = updated
	}

	if !result.Success {
		if !params.silentOutput {
			outputWorkflowResult(r)
		}
		return nil
	}

	if params.silentOutput {
		return nil
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
