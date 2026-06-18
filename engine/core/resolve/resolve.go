package resolve

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/loongxjin/forksync/engine/core/agent"
	"github.com/loongxjin/forksync/engine/core/agent/session"
	"github.com/loongxjin/forksync/engine/core/config"
	"github.com/loongxjin/forksync/engine/core/git"
	"github.com/loongxjin/forksync/engine/core/logger"
	"github.com/loongxjin/forksync/engine/core/repo"
	"github.com/loongxjin/forksync/engine/core/workflow"
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
	// Stamp a fresh resolve session id so the frontend can locate this
	// resolve's agent log precisely (by session), not by "newest file".
	workflow.SetResolveSessionID(wf, uuid.New().String())
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

// AgentResult groups the outcome of an agent resolution attempt.
type AgentResult struct {
	Success       bool
	Repo          types.Repo
	AgentResult   *agent.AgentResult
	Diff          string
	ResolvedFiles []string
	Unresolved    []string
}

// ResolveWithAgent runs the agent resolution flow and returns the outcome.
// r.sessionMgr must be set (via NewResolver) before calling this method.
//
// This is a thin wrapper over RunAgentResolve for the interactive-resolve path.
// It does NOT transition workflow/repo state — callers own the state machine.
// The auto-sync path should call RunAgentResolve directly.
func (r *Resolver) ResolveWithAgent(
	ctx context.Context,
	repo types.Repo,
	strategy string,
	streamWriter *agent.StreamWriter,
) (*AgentResult, error) {
	return r.RunAgentResolve(ctx, repo, strategy, streamWriter, nil /* detect conflicts */)
}

// RunAgentResolve drives the agent over the conflicts, verifies conflict
// markers are gone, auto-stages resolved files, and fills out an AgentResult.
// It does NOT commit and does NOT transition the workflow/repo status — those
// are the caller's responsibility (the interactive path transitions to
// WorkflowWaiting; the auto-sync path's handleMergeConflicts drives its own
// state machine). conflictPaths, when non-nil, is used directly; when nil the
// conflicts are detected from the working tree.
//
// This is the single shared "resolve core" (drive agent → verify → stage).
// r.sessionMgr must be set (via NewResolver) before calling this method.
func (r *Resolver) RunAgentResolve(
	ctx context.Context,
	repo types.Repo,
	strategy string,
	streamWriter *agent.StreamWriter,
	conflictPaths []string,
) (*AgentResult, error) {
	if r.sessionMgr == nil {
		return nil, fmt.Errorf("resolve: session manager not configured")
	}

	if conflictPaths == nil {
		conflictPaths = r.gitOps.DetectConflicts(ctx, repo.Path)
	}
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

	// Verify: check for remaining conflict markers and auto-stage resolved files.
	// Delegated to the git seam (OperationsProvider.FilterResolvedFiles) so the
	// verify-and-stage step has one implementation shared with the auto-sync path.
	trulyUnresolved := r.gitOps.FilterResolvedFiles(ctx, repo.Path, conflictPaths)

	if len(trulyUnresolved) > 0 {
		// Populate the agent result for the caller (diff/name still useful in
		// the failure case for reporting), but mark unresolved.
		agentName := r.sessionMgr.ProviderName()
		result.ResolvedFiles = conflictPaths
		result.AgentName = agentName
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

	return &AgentResult{
		Success:       true,
		Repo:          repo,
		AgentResult:   result,
		Diff:          result.Diff,
		ResolvedFiles: conflictPaths,
	}, nil
}
