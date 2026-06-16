package resolve

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/loongxjin/forksync/engine/internal/agent"
	"github.com/loongxjin/forksync/engine/internal/agent/session"
	"github.com/loongxjin/forksync/engine/internal/config"
	"github.com/loongxjin/forksync/engine/internal/git"
	"github.com/loongxjin/forksync/engine/internal/logger"
	"github.com/loongxjin/forksync/engine/internal/repo"
	"github.com/loongxjin/forksync/engine/internal/workflow"
	"github.com/loongxjin/forksync/engine/pkg/types"
)

// Resolver handles conflict resolution via agent CLIs.
type Resolver struct {
	gitOps     git.OperationsProvider
	store      repo.Store
	cfg        *config.Config
	cfgMgr     *config.Manager
	sessionMgr *session.Manager
}

// NewResolver creates a Resolver with the given dependencies.
// sessionMgr is optional (can be nil); only ResolveWithAgent requires it.
func NewResolver(
	gitOps git.OperationsProvider,
	store repo.Store,
	cfg *config.Config,
	cfgMgr *config.Manager,
	sessionMgr *session.Manager,
) *Resolver {
	return &Resolver{
		gitOps:     gitOps,
		store:      store,
		cfg:        cfg,
		cfgMgr:     cfgMgr,
		sessionMgr: sessionMgr,
	}
}

// Reject rolls back the merge and resets the workflow.
func (r *Resolver) Reject(ctx context.Context, repo types.Repo) (types.Repo, error) {
	if r.cfgMgr != nil {
		agent.DeleteAllLogs(r.cfgMgr.ConfigDir(), repo.Name)
	}

	err := r.gitOps.AbortMerge(ctx, repo.Path)
	if err != nil {
		logger.Warn("resolve: merge --abort failed", "repo", repo.Name, "error", err)
	}

	// Mark workflow as aborted (pending steps → Skipped, workflow → Failed)
	if repo.Workflow != nil {
		for i := range repo.Workflow.Steps {
			if repo.Workflow.Steps[i].Status == types.StepStatusPending {
				repo.Workflow.Steps[i].Status = types.StepStatusSkipped
				now := types.Time{Time: time.Now()}
				repo.Workflow.Steps[i].EndedAt = &now
			}
		}
		workflow.MarkWorkflowDone(repo.Workflow, types.WorkflowFailed)
	}

	repo.Status = types.RepoStatusSyncNeeded
	repo.ErrorMessage = ""
	if err := r.store.Update(repo); err != nil {
		logger.Error("resolve: failed to update repo after reject", "repo", repo.Name, "error", err)
	}

	return repo, nil
}

// Prepare marks the workflow as ready for agent resolution without actually
// running the agent.
func (r *Resolver) Prepare(repo types.Repo) (types.Repo, error) {
	wf := repo.Workflow
	if wf == nil {
		wf = workflow.NewWorkflowFromRepo(repo)
	}
	workflow.AdvanceStep(wf, types.StepResolveStrategy, types.StepStatusSuccess, "")
	workflow.AdvanceStep(wf, types.StepAgentResolve, types.StepStatusRunning, "")
	// Restore accept_changes from skipped to pending so it can be used later.
	for i := range wf.Steps {
		if wf.Steps[i].Step == types.StepAcceptChanges && wf.Steps[i].Status == types.StepStatusSkipped {
			wf.Steps[i].Status = types.StepStatusPending
			wf.Steps[i].Message = ""
			wf.Steps[i].EndedAt = nil
		}
	}
	wf.Status = types.WorkflowRunning
	repo.Workflow = wf
	repo.Status = types.RepoStatusConflict
	repo.ErrorMessage = ""
	if err := r.store.Update(repo); err != nil {
		logger.Error("resolve: failed to update repo for prepare", "repo", repo.Name, "error", err)
		return repo, err
	}
	return repo, nil
}

// Accept checks for remaining conflicts and finalizes the commit.
func (r *Resolver) Accept(ctx context.Context, repo types.Repo, manual bool, retry bool) (types.Repo, workflow.CommitResult, error) {
	remaining := r.gitOps.DetectConflicts(ctx, repo.Path)

	if len(remaining) > 0 {
		repo.Status = types.RepoStatusConflict
		repo.ErrorMessage = fmt.Sprintf("%d conflicts still unresolved", len(remaining))
		return repo, workflow.CommitResult{}, fmt.Errorf("%d conflicts still unresolved", len(remaining))
	}

	// Check if we're in a merge state
	mergeHead := filepath.Join(repo.Path, ".git", "MERGE_HEAD")
	if _, err := os.Stat(mergeHead); err != nil {
		repo.Status = types.RepoStatusUpToDate
		repo.ErrorMessage = ""
		if updateErr := r.store.Update(repo); updateErr != nil {
			logger.Error("resolve: failed to update repo after accept-no-merge", "repo", repo.Name, "error", updateErr)
		}
		return repo, workflow.CommitResult{Success: true}, nil
	}

	commitMsg := types.CommitMsgAgentResolved
	if manual {
		commitMsg = types.CommitMsgManualResolved
	}

	var configDir string
	if r.cfgMgr != nil {
		configDir = r.cfgMgr.ConfigDir()
	}

	result, err := workflow.FinalizeCommit(ctx, repo, r.store, r.gitOps, r.cfg, configDir, workflow.CommitParams{
		CommitMsg:          commitMsg,
		SkipAgentAndAccept: manual,
		RecordHistory:      !retry,
	})

	// Reload repo from store to get updated state
	updated, ok := r.store.Get(repo.ID)
	if ok {
		repo = updated
	}

	return repo, result, err
}

// AgentResult groups the outcome of an agent resolution attempt.
type AgentResult struct {
	Success       bool
	Repo          types.Repo
	AgentResult   *agent.AgentResult
	Diff          string
	ResolvedFiles []string
	Unresolved    []string
}

// ResolveWithAgent runs the full agent resolution flow.
// r.sessionMgr must be set (via NewResolver) before calling this method.
func (r *Resolver) ResolveWithAgent(
	ctx context.Context,
	repo types.Repo,
	strategy string,
	streamWriter *agent.StreamWriter,
) (*AgentResult, error) {
	if r.sessionMgr == nil {
		return nil, fmt.Errorf("resolve: session manager not configured")
	}

	conflictPaths := r.gitOps.DetectConflicts(ctx, repo.Path)
	if len(conflictPaths) == 0 {
		return &AgentResult{Success: true, Repo: repo}, nil
	}

	// Determine prompt language from config
	language := "zh"
	if r.cfg != nil && r.cfg.Sync.SummaryLanguage != "" {
		language = r.cfg.Sync.SummaryLanguage
	}

	result, err := r.sessionMgr.ResolveConflicts(ctx, repo.ID, repo.Path, conflictPaths, strategy, language, streamWriter)
	if err != nil {
		logger.Error("resolve: agent resolve failed", "repo", repo.Name, "error", err)
		return nil, fmt.Errorf("agent resolve: %w", err)
	}

	// Verify: check for remaining conflict markers
	trulyUnresolved := verifyAgentResolution(ctx, r.gitOps, repo, conflictPaths)

	if len(trulyUnresolved) > 0 {
		return &AgentResult{
			Success:       false,
			Repo:          repo,
			AgentResult:   result,
			ResolvedFiles: conflictPaths,
			Unresolved:    trulyUnresolved,
		}, nil
	}

	agentName := r.sessionMgr.ProviderName()

	// Get diff for user confirmation
	diffBytes, _ := r.gitOps.Diff(ctx, repo.Path)
	result.Diff = string(diffBytes)
	result.ResolvedFiles = conflictPaths
	result.AgentName = agentName

	// Update status — agent resolved successfully
	if repo.Workflow == nil {
		repo.Workflow = workflow.NewWorkflowFromRepo(repo)
	}
	workflow.AdvanceStep(repo.Workflow, types.StepResolveStrategy, types.StepStatusSuccess, "")
	workflow.AdvanceStep(repo.Workflow, types.StepAgentResolve, types.StepStatusSuccess,
		fmt.Sprintf("resolved by %s", agentName))
	workflow.AdvanceStep(repo.Workflow, types.StepAcceptChanges, types.StepStatusWaiting, "")
	repo.Workflow.Status = types.WorkflowWaiting
	repo.Status = types.RepoStatusResolved
	repo.ErrorMessage = ""
	storeErr := r.store.Update(repo)
	if storeErr != nil {
		logger.Error("resolve: failed to update repo after agent resolution", "repo", repo.Name, "error", storeErr)
	}

	// NOTE: the adapter already emits a state_persisted event when it finishes.
	// We no longer emit a second one here — it was redundant and caused the
	// terminal drawer to render an extra empty line.

	return &AgentResult{
		Success:       true,
		Repo:          repo,
		AgentResult:   result,
		Diff:          result.Diff,
		ResolvedFiles: conflictPaths,
	}, nil
}

// verifyAgentResolution checks remaining conflict files and auto-stages those
// that have been resolved (no conflict markers).
func verifyAgentResolution(ctx context.Context, gitOps git.OperationsProvider, r types.Repo, remaining []string) []string {
	if len(remaining) == 0 {
		return nil
	}

	var trulyUnresolved []string
	for _, f := range remaining {
		content, err := gitOps.GetConflictedContent(ctx, r.Path, f)
		if err != nil {
			trulyUnresolved = append(trulyUnresolved, f)
			continue
		}
		if git.HasConflictMarkers(content) {
			trulyUnresolved = append(trulyUnresolved, f)
			continue
		}
		// Markers removed but not staged — auto-stage to mark as resolved
		if stageErr := gitOps.StageFile(ctx, r.Path, f); stageErr != nil {
			logger.Warn("resolve: auto-stage resolved file failed",
				"repo", r.Name, "file", f, "error", stageErr)
			trulyUnresolved = append(trulyUnresolved, f)
		}
	}
	return trulyUnresolved
}
