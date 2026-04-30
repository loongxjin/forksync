package sync

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/loongxjin/forksync/engine/internal/agent"
	"github.com/loongxjin/forksync/engine/internal/agent/session"
	"github.com/loongxjin/forksync/engine/internal/config"
	"github.com/loongxjin/forksync/engine/internal/conflict"
	"github.com/loongxjin/forksync/engine/internal/git"
	"github.com/loongxjin/forksync/engine/internal/history"
	"github.com/loongxjin/forksync/engine/internal/logger"
	"github.com/loongxjin/forksync/engine/internal/notify"
	"github.com/loongxjin/forksync/engine/internal/repo"
	"github.com/loongxjin/forksync/engine/internal/summarizer"
	"github.com/loongxjin/forksync/engine/pkg/types"
)

const (
	defaultTimeout         = 5 * time.Minute
	postSyncCommandTimeout = 60 * time.Second
)

// Syncer handles repository synchronization.
type Syncer struct {
	gitOps       *git.Operations
	store        repo.Store
	cfg          *config.Config
	notifier     *notify.Notifier
	sessionMgr   *session.Manager
	historyStore *history.Store
	summarizer   *summarizer.Summarizer
	configDir    string // base config directory (e.g. ~/.forksync)
	mu           sync.Mutex
	active       map[string]bool // tracks repos currently syncing
}

// NewSyncer creates a new Syncer.
func NewSyncer(store repo.Store) *Syncer {
	return &Syncer{
		gitOps: git.NewOperations(),
		store:  store,
		active: make(map[string]bool),
	}
}

// SetNotifier sets the notification handler.
func (s *Syncer) SetNotifier(n *notify.Notifier) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.notifier = n
}

// SetSessionManager sets the agent session manager for auto-conflict resolution.
func (s *Syncer) SetSessionManager(mgr *session.Manager) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessionMgr = mgr
}

// SetHistoryStore sets the sync history store for recording sync results.
func (s *Syncer) SetHistoryStore(h *history.Store) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.historyStore = h
}

// SetSummarizer sets the AI summarizer for generating sync summaries.
func (s *Syncer) SetSummarizer(sm *summarizer.Summarizer) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.summarizer = sm
}

// pendingInfo holds agent resolution details when awaiting user confirmation.
type pendingInfo struct {
	Files       []string
	Diff        string
	Summary     string
	Agent       string
	CommitError string
}

// Result contains the result of syncing a single repo.
type Result struct {
	RepoID          string
	RepoName        string
	RepoPath        string // used by summarizer to get commit list
	UpstreamRef     string // upstream remote/branch ref for commit diff, e.g. "upstream/main"
	OldHEAD         string // HEAD before merge, used to compute pulled commits
	Status          string // types.RepoStatus values: up_to_date, conflict, error
	CommitsPulled   int
	ConflictFiles   []string
	ErrorMessage    string
	AgentUsed       string                    // agent name if auto-resolve was attempted
	ConflictsFound  int                       // number of conflicts detected
	AutoResolved    int                       // number of files auto-resolved by agent
	PendingConfirm  []string                  // files pending user confirmation
	AgentResult     *types.AgentResolveResult // agent resolution result when pending confirmation
	PostSyncResults []types.PostSyncResult
	HistoryID       int64 // ID of the created history record
	CommitError     string
	Workflow        *types.SyncWorkflow // current workflow run
}

// ToSyncResult converts Result to types.SyncResult for JSON output.
func (r *Result) ToSyncResult() types.SyncResult {
	return types.SyncResult{
		RepoID:          r.RepoID,
		RepoName:        r.RepoName,
		Status:          types.RepoStatus(r.Status),
		CommitsPulled:   r.CommitsPulled,
		ConflictFiles:   r.ConflictFiles,
		ErrorMessage:    r.ErrorMessage,
		AgentUsed:       r.AgentUsed,
		ConflictsFound:  r.ConflictsFound,
		AutoResolved:    r.AutoResolved,
		PendingConfirm:  r.PendingConfirm,
		AgentResult:     r.AgentResult,
		PostSyncResults: r.PostSyncResults,
		CommitError:     r.CommitError,
		Workflow:        r.Workflow,
	}
}

// SyncRepo syncs a single repository.
func (s *Syncer) SyncRepo(ctx context.Context, r types.Repo) *Result {
	// Compute upstream ref for summarizer
	remoteName := r.RemoteName()
	branch := r.Branch
	if branch == "" {
		if b, err := s.gitOps.GetCurrentBranch(ctx, r.Path); err == nil {
			branch = b
		}
	}
	if branch == "" {
		branch = types.DefaultBranch
	}
	upstreamRef := fmt.Sprintf("%s/%s", remoteName, r.GetRemoteBranchForLocal(branch))

	result := &Result{
		RepoID:      r.ID,
		RepoName:    r.Name,
		RepoPath:    r.Path,
		UpstreamRef: upstreamRef,
	}

	// Concurrency guard: mark repo as active
	s.mu.Lock()
	if s.active[r.ID] {
		s.mu.Unlock()
		result.Status = string(types.RepoStatusError)
		result.ErrorMessage = "sync already in progress"
		result.Workflow = newWorkflow(r.ID)
		advanceStep(result.Workflow, types.StepFetch, types.StepStatusFailed, "sync already in progress")
		markWorkflowDone(result.Workflow, types.WorkflowFailed)
		s.finalizeResult(result)
		return result
	}
	s.active[r.ID] = true
	defer func() {
		s.mu.Lock()
		delete(s.active, r.ID)
		s.mu.Unlock()
	}()
	s.mu.Unlock()

	// Initialize workflow
	wf := newWorkflow(r.ID)
	result.Workflow = wf
	s.saveWorkflow(r, wf)

	// Phase 1: Pre-checks (conflict state detection)
	if stopped := s.checkConflictState(ctx, r, result); stopped {
		result.Workflow = workflowFromResult(result)
		s.saveWorkflow(r, result.Workflow)
		return result
	}

	// Phase 2: Execute the actual sync (fetch → status → merge → post-sync)
	return s.executeSync(ctx, r, result)
}

// checkConflictState checks whether the repo is in a conflict or merge state
// that should block syncing.
// Returns true if the sync should be aborted (result is already populated).
func (s *Syncer) checkConflictState(ctx context.Context, r types.Repo, result *Result) bool {
	wf := result.Workflow
	// Check if repo is already in a conflict/merge state before proceeding.
	// This is a pre-check guard, NOT an actual sync attempt — so we skip
	// recording history to avoid polluting the sync log.
	// Note: IsMergingState auto-stages files that have been manually resolved
	// but not yet staged, so unmergedFiles only contains truly conflicted files.
	isMerging, unmergedFiles, err := s.gitOps.IsMergingState(ctx, r.Path)
	if err == nil && isMerging {
		if len(unmergedFiles) == 0 {
			// All conflicts were resolved but not staged — now auto-staged.
			// MERGE_HEAD still exists, transition to resolved state for user confirmation.
			result.Status = string(types.RepoStatusResolved)
			advanceStep(wf, types.StepFetch, types.StepStatusSuccess, "")
			advanceStep(wf, types.StepMerge, types.StepStatusSuccess, "")
			advanceStep(wf, types.StepCheckConflicts, types.StepStatusSuccess, "")
			advanceStep(wf, types.StepResolveStrategy, types.StepStatusSuccess, "")
			markStepSkipped(wf, types.StepAgentResolve)
			advanceStep(wf, types.StepAcceptChanges, types.StepStatusWaiting, "")
			wf.Status = types.WorkflowWaiting
			s.updateRepoStatus(r.ID, types.RepoStatusResolved, "")
			s.saveWorkflow(r, wf)
			s.notifyResult(r.Name, result)
			s.logResult(result)
			return true
		}
		result.ConflictFiles = unmergedFiles
		result.ErrorMessage = "repository has unresolved merge conflicts, please resolve conflicts before syncing"
		result.ConflictsFound = len(unmergedFiles)
		advanceStep(wf, types.StepFetch, types.StepStatusSuccess, "")
		advanceStep(wf, types.StepMerge, types.StepStatusSuccess, "")
		advanceStep(wf, types.StepCheckConflicts, types.StepStatusSuccess,
			fmt.Sprintf("%d files have conflicts", len(unmergedFiles)))
		// If a workflow exists and was waiting at resolve_strategy, preserve waiting status
		if r.Workflow != nil && findStep(r.Workflow, types.StepResolveStrategy) != nil &&
			findStep(r.Workflow, types.StepResolveStrategy).Status == types.StepStatusWaiting {
			result.Status = string(types.RepoStatusWaiting)
			wf.Status = types.WorkflowWaiting
			s.updateRepoStatus(r.ID, types.RepoStatusWaiting, result.ErrorMessage)
		} else {
			result.Status = string(types.RepoStatusConflict)
			advanceStep(wf, types.StepResolveStrategy, types.StepStatusWaiting, "")
			markStepSkipped(wf, types.StepAgentResolve)
			markStepSkipped(wf, types.StepAcceptChanges)
			wf.Status = types.WorkflowWaiting
			s.updateRepoStatus(r.ID, types.RepoStatusConflict, result.ErrorMessage)
		}
		s.saveWorkflow(r, wf)
		s.notifyResult(r.Name, result)
		// DO NOT call finalizeResult — this is not a real sync, don't pollute history
		s.logResult(result)
		return true
	}

	// Also check if stored status indicates a conflict state that hasn't been resolved
	if r.Status == types.RepoStatusConflict || r.Status == types.RepoStatusResolving || r.Status == types.RepoStatusResolved || r.Status == types.RepoStatusWaiting {
		result.Status = string(types.RepoStatusConflict)
		result.ErrorMessage = fmt.Sprintf("repository is in %s state, please resolve conflicts before syncing", r.Status)
		advanceStep(wf, types.StepFetch, types.StepStatusFailed, result.ErrorMessage)
		markWorkflowDone(wf, types.WorkflowFailed)
		s.saveWorkflow(r, wf)
		// DO NOT call finalizeResult — this is not a real sync, don't pollute history
		s.logResult(result)
		return true
	}

	return false
}

// executeSync performs the actual sync: fetch → status check → merge → post-sync commands.
func (s *Syncer) executeSync(ctx context.Context, r types.Repo, result *Result) *Result {
	wf := result.Workflow
	// Set timeout — use agent timeout if auto-resolve is configured,
	// otherwise the default 5 minutes may SIGKILL long-running agents.
	timeout := defaultTimeout
	if s.shouldUseAgentResolve(r) {
		timeout = agentResolveTimeout(s.cfg)
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Update status to syncing
	r.Status = types.RepoStatusSyncing
	if wf != nil {
		r.Workflow = wf
	}
	if updateErr := s.store.Update(r); updateErr != nil {
		logger.Error("syncer: failed to update repo to syncing", "repo", r.Name, "error", updateErr)
	}

	// Step 1: Fetch
	advanceStep(wf, types.StepFetch, types.StepStatusRunning, "")
	s.saveWorkflow(r, wf)
	if err := s.gitOps.Fetch(ctx, r); err != nil {
		advanceStep(wf, types.StepFetch, types.StepStatusFailed, fmt.Sprintf("fetch failed: %v", err))
		markWorkflowDone(wf, types.WorkflowFailed)
		result.Status = string(types.RepoStatusError)
		result.ErrorMessage = fmt.Sprintf("fetch failed: %v", err)
		s.updateRepoStatus(r.ID, types.RepoStatusError, result.ErrorMessage)
		s.notifyResult(r.Name, result)
		s.saveWorkflow(r, wf)
		s.finalizeResult(result)
		return result
	}
	advanceStep(wf, types.StepFetch, types.StepStatusSuccess, "")
	s.saveWorkflow(r, wf)

	// Step 2: Check ahead/behind
	statusResult, err := s.gitOps.Status(ctx, r)
	if err != nil {
		advanceStep(wf, types.StepMerge, types.StepStatusFailed, fmt.Sprintf("status check failed: %v", err))
		markWorkflowDone(wf, types.WorkflowFailed)
		result.Status = string(types.RepoStatusError)
		result.ErrorMessage = fmt.Sprintf("status check failed: %v", err)
		s.updateRepoStatus(r.ID, types.RepoStatusError, result.ErrorMessage)
		s.saveWorkflow(r, wf)
		s.finalizeResult(result)
		return result
	}

	if statusResult.BehindBy == 0 {
		advanceStep(wf, types.StepMerge, types.StepStatusSuccess, "")
		markStepSkipped(wf, types.StepCheckConflicts)
		markStepSkipped(wf, types.StepResolveStrategy)
		markStepSkipped(wf, types.StepAgentResolve)
		markStepSkipped(wf, types.StepAcceptChanges)
		advanceStep(wf, types.StepCommit, types.StepStatusSuccess, "")
		markWorkflowDone(wf, types.WorkflowSuccess)
		result.Status = string(types.RepoStatusUpToDate)
		result.CommitsPulled = 0
		s.updateRepoStatus(r.ID, types.RepoStatusUpToDate, "")
		s.saveWorkflow(r, wf)
		s.finalizeResult(result)
		return result
	}

	result.CommitsPulled = statusResult.BehindBy

	// Step 3: Merge
	// Remember HEAD before merge for summarizer
	if head, err := s.gitOps.GetHEAD(ctx, r.Path); err == nil {
		result.OldHEAD = head
	}

	advanceStep(wf, types.StepMerge, types.StepStatusRunning, "")
	s.saveWorkflow(r, wf)
	mergeResult, err := s.gitOps.Merge(ctx, r)
	if err != nil {
		advanceStep(wf, types.StepMerge, types.StepStatusFailed, fmt.Sprintf("merge failed: %v", err))
		markWorkflowDone(wf, types.WorkflowFailed)
		result.Status = string(types.RepoStatusError)
		result.ErrorMessage = fmt.Sprintf("merge failed: %v", err)
		s.updateRepoStatus(r.ID, types.RepoStatusError, result.ErrorMessage)
		s.notifyResult(r.Name, result)
		s.saveWorkflow(r, wf)
		s.finalizeResult(result)
		return result
	}
	advanceStep(wf, types.StepMerge, types.StepStatusSuccess, "")
	s.saveWorkflow(r, wf)

	// Step 4: Check conflicts
	advanceStep(wf, types.StepCheckConflicts, types.StepStatusRunning, "")
	s.saveWorkflow(r, wf)
	if mergeResult.HasConflicts {
		advanceStep(wf, types.StepCheckConflicts, types.StepStatusSuccess,
			fmt.Sprintf("%d files have conflicts", len(mergeResult.Conflicts)))
		s.saveWorkflow(r, wf)
		return s.handleMergeConflicts(ctx, r, result, mergeResult)
	}
	advanceStep(wf, types.StepCheckConflicts, types.StepStatusSuccess, "")
	markStepSkipped(wf, types.StepResolveStrategy)
	markStepSkipped(wf, types.StepAgentResolve)
	markStepSkipped(wf, types.StepAcceptChanges)

	// Step 7: Commit (and post-sync)
	advanceStep(wf, types.StepCommit, types.StepStatusRunning, "")
	s.saveWorkflow(r, wf)
	result.Status = string(types.RepoStatusUpToDate)
	s.updateRepoStatus(r.ID, types.RepoStatusUpToDate, "")
	result.PostSyncResults = s.runPostSyncCommands(ctx, r)
	if postSyncErr := s.postSyncError(result.PostSyncResults); postSyncErr != "" {
		result.ErrorMessage = postSyncErr
		s.updateRepoStatus(r.ID, types.RepoStatusUpToDate, result.ErrorMessage)
	}
	advanceStep(wf, types.StepCommit, types.StepStatusSuccess, "")
	markWorkflowDone(wf, types.WorkflowSuccess)
	s.saveWorkflow(r, wf)
	s.notifyResult(r.Name, result)
	s.finalizeResult(result)
	return result
}

// handleMergeConflicts processes merge conflicts, attempting agent auto-resolve if configured.
// Returns the final result for the sync operation.
func (s *Syncer) handleMergeConflicts(ctx context.Context, r types.Repo, result *Result, mergeResult *git.MergeResult) *Result {
	wf := result.Workflow
	result.ConflictsFound = len(mergeResult.Conflicts)
	result.ConflictFiles = mergeResult.Conflicts

	// Step 5: Resolve strategy (decision point)
	advanceStep(wf, types.StepResolveStrategy, types.StepStatusRunning, "")
	s.saveWorkflow(r, wf)

	// Determine auto-resolve strategy from global config
	autoAgentResolve := false
	if s.cfg != nil && s.cfg.Agent.ConflictStrategy == types.StrategyAgentResolve {
		autoAgentResolve = true
	}

	if autoAgentResolve && s.sessionMgr != nil {
		advanceStep(wf, types.StepResolveStrategy, types.StepStatusSuccess, "")
		advanceStep(wf, types.StepAgentResolve, types.StepStatusRunning, "")
		s.saveWorkflow(r, wf)

		resolved, pending := s.tryAgentResolve(ctx, r, mergeResult.Conflicts)
		if resolved {
			// Agent resolved and auto-committed
			advanceStep(wf, types.StepAgentResolve, types.StepStatusSuccess,
				fmt.Sprintf("resolved by %s", s.sessionMgr.ProviderName()))
			markStepSkipped(wf, types.StepAcceptChanges)
			advanceStep(wf, types.StepCommit, types.StepStatusSuccess, "")
			markWorkflowDone(wf, types.WorkflowSuccess)
			result.Status = string(types.RepoStatusUpToDate)
			result.AutoResolved = len(mergeResult.Conflicts)
			s.updateRepoStatus(r.ID, types.RepoStatusUpToDate, "")
			s.saveWorkflow(r, wf)
			s.notifyResult(r.Name, result)
			s.finalizeResult(result)
			return result
		}
		if pending != nil {
			// Agent resolved but needs confirmation
			advanceStep(wf, types.StepAgentResolve, types.StepStatusSuccess,
				fmt.Sprintf("resolved by %s", pending.Agent))
			advanceStep(wf, types.StepAcceptChanges, types.StepStatusWaiting, "")
			wf.Status = types.WorkflowWaiting
			result.Status = string(types.RepoStatusResolved)
			result.AgentUsed = pending.Agent
			result.AutoResolved = len(pending.Files)
			result.PendingConfirm = pending.Files
			result.CommitError = pending.CommitError
			result.AgentResult = &types.AgentResolveResult{
				Success:       true,
				ResolvedFiles: pending.Files,
				Diff:          pending.Diff,
				Summary:       pending.Summary,
				AgentName:     pending.Agent,
			}
			s.updateRepoStatus(r.ID, types.RepoStatusResolved, "")
			s.saveWorkflow(r, wf)
			s.notifyResult(r.Name, result)
			s.finalizeResult(result)
			return result
		}
		// Agent failed
		advanceStep(wf, types.StepAgentResolve, types.StepStatusFailed, "agent failed to resolve conflicts")
		markWorkflowDone(wf, types.WorkflowFailed)
		result.Status = string(types.RepoStatusConflict)
		s.updateRepoStatus(r.ID, types.RepoStatusConflict, "")
		s.saveWorkflow(r, wf)
		s.notifyResult(r.Name, result)
		s.finalizeResult(result)
		return result
	}

	// Manual resolve path: pause at resolve_strategy
	advanceStep(wf, types.StepResolveStrategy, types.StepStatusWaiting, "")
	markStepSkipped(wf, types.StepAgentResolve)
	markStepSkipped(wf, types.StepAcceptChanges)
	wf.Status = types.WorkflowWaiting
	result.Status = string(types.RepoStatusWaiting)
	s.updateRepoStatus(r.ID, types.RepoStatusWaiting, "")
	s.saveWorkflow(r, wf)
	s.notifyResult(r.Name, result)
	s.finalizeResult(result)
	return result
}

// tryAgentResolve attempts to resolve conflicts using the agent CLI.
// Returns (resolved, pending):
//   - resolved=true, pending=nil: all conflicts resolved and committed (auto-confirm)
//   - resolved=false, pending=*pendingInfo: conflicts resolved but awaiting user confirmation
//   - resolved=false, pending=nil: agent failed to resolve
func (s *Syncer) tryAgentResolve(ctx context.Context, r types.Repo, conflictPaths []string) (bool, *pendingInfo) {
	if s.sessionMgr == nil {
		return false, nil
	}

	// Create or reuse a session for this repo
	if _, err := s.sessionMgr.GetOrCreate(ctx, r.ID, r.Path); err != nil {
		return false, nil
	}

	// Determine resolve sub-strategy for the agent prompt
	resolveStrategy := s.resolveStrategyOrDefault()

	// Set up log writer for auto-sync background runs so users can replay later
	var streamWriter *agent.StreamWriter
	lw, lwErr := agent.NewLogWriter(s.configDir, r.Name)
	if lwErr != nil {
		logger.Warn("sync: failed to create agent log writer", "repo", r.Name, "error", lwErr)
	}
	if lw != nil {
		defer lw.Close()
		streamWriter = lw.StreamWriter()
		logger.Info("sync: agent log writer active", "repo", r.Name)
	}

	// Resolve conflicts via agent
	result, err := s.sessionMgr.ResolveConflicts(ctx, r.ID, r.Path, conflictPaths, resolveStrategy, streamWriter)
	if err != nil {
		logger.Warn("sync: agent resolve failed",
			"repo", r.Name,
			"agent", s.sessionMgr.ProviderName(),
			"error", err,
		)
		return false, nil
	}
	if !result.Success {
		logger.Warn("sync: agent reported unsuccessful resolve",
			"repo", r.Name,
			"agent", s.sessionMgr.ProviderName(),
			"summary", result.Summary,
		)
		return false, nil
	}

	// Verify no conflict markers remain and stage resolved files
	if !s.verifyAndStageResolvedFiles(ctx, r, result) {
		return false, nil
	}

	// Check staged changes (log but don't fail — whitespace issues are non-critical)
	if checkErr := s.gitOps.CheckStaged(ctx, r.Path); checkErr != nil {
		logger.Debug("sync: staged changes check found issues", "repo", r.Name, "error", checkErr)
	}

	// Check if auto-confirm is enabled
	autoConfirm := true
	if s.cfg != nil {
		autoConfirm = !s.cfg.Agent.ConfirmBeforeCommit
	}

	logger.Info("sync: auto-confirm check",
		"repo", r.Name,
		"autoConfirm", autoConfirm,
		"confirmBeforeCommit", s.cfg.Agent.ConfirmBeforeCommit,
		"cfg_nil", s.cfg == nil,
	)

	if !autoConfirm {
		return s.buildPendingInfo(ctx, r, result)
	}

	// Complete the merge with a commit
	commitMsg := fmt.Sprintf("Merge upstream (auto-resolved by %s)", s.sessionMgr.ProviderName())
	if err := s.gitOps.Commit(ctx, r.Path, commitMsg); err != nil {
		logger.Warn("sync: auto-commit failed after agent resolution, falling back to confirmation",
			"repo", r.Name,
			"agent", s.sessionMgr.ProviderName(),
			"error", err,
		)
		_, pending := s.buildPendingInfo(ctx, r, result)
		pending.CommitError = fmt.Sprintf("auto-commit failed: %v", err)
		return false, pending
	}

	return true, nil
}

// resolveStrategyOrDefault returns the resolve strategy from config, or the default.
func (s *Syncer) resolveStrategyOrDefault() string {
	return config.ResolveStrategyOrDefault(s.cfg)
}

// shouldUseAgentResolve checks whether agent auto-resolve is configured globally.
func (s *Syncer) shouldUseAgentResolve(r types.Repo) bool {
	if s.cfg != nil {
		return s.cfg.Agent.ConflictStrategy == types.StrategyAgentResolve
	}
	return false
}

// saveWorkflow updates the repo's workflow in the store.
func (s *Syncer) saveWorkflow(r types.Repo, wf *types.SyncWorkflow) {
	if s.store == nil {
		return
	}
	stored, ok := s.store.Get(r.ID)
	if !ok {
		return
	}
	stored.Workflow = wf
	if updateErr := s.store.Update(stored); updateErr != nil {
		logger.Error("syncer: failed to save workflow", "repo", r.Name, "error", updateErr)
	}
}

// agentResolveTimeout returns the timeout for agent conflict resolution.
// Falls back to 10 minutes if no config is available.
func agentResolveTimeout(cfg *config.Config) time.Duration {
	if cfg != nil && cfg.Agent.Timeout != "" {
		if d, err := time.ParseDuration(cfg.Agent.Timeout); err == nil && d > 0 {
			return d
		}
	}
	return 10 * time.Minute
}

// verifyAndStageResolvedFiles checks that resolved files have no conflict markers and stages them.
func (s *Syncer) verifyAndStageResolvedFiles(ctx context.Context, r types.Repo, result *agent.AgentResult) bool {
	var stillConflicted []string
	for _, file := range result.ResolvedFiles {
		content, err := s.gitOps.GetConflictedContent(ctx, r.Path, file)
		if err != nil {
			continue
		}
		if conflict.HasConflictMarkers(content) {
			stillConflicted = append(stillConflicted, file)
		}
	}
	if len(stillConflicted) > 0 {
		logger.Warn("sync: agent left conflict markers in files",
			"repo", r.Name,
			"agent", s.sessionMgr.ProviderName(),
			"files", stillConflicted,
			"summary", result.Summary,
		)
		return false
	}

	for _, file := range result.ResolvedFiles {
		if err := s.gitOps.StageFile(ctx, r.Path, file); err != nil {
			logger.Warn("sync: failed to stage resolved file",
				"repo", r.Name,
				"file", file,
				"error", err,
			)
			return false
		}
	}
	return true
}

// buildPendingInfo creates a pendingInfo for user confirmation flow.
func (s *Syncer) buildPendingInfo(ctx context.Context, r types.Repo, result *agent.AgentResult) (bool, *pendingInfo) {
	logger.Info("sync: agent resolved conflicts, awaiting user confirmation",
		"repo", r.Name,
		"agent", s.sessionMgr.ProviderName(),
		"files", result.ResolvedFiles,
	)

	diffBytes, diffErr := s.gitOps.DiffStaged(ctx, r.Path)
	diff := ""
	if diffErr == nil {
		diff = string(diffBytes)
		const maxDiffSize = 100 * 1024 // 100KB limit
		if len(diff) > maxDiffSize {
			diff = diff[:maxDiffSize] + "\n\n... (diff truncated)"
		}
	}

	return false, &pendingInfo{
		Files:   result.ResolvedFiles,
		Diff:    diff,
		Summary: result.Summary,
		Agent:   s.sessionMgr.ProviderName(),
	}
}

// SyncAll syncs all managed repositories.
func (s *Syncer) SyncAll(ctx context.Context) []*Result {
	repos, err := s.store.List()
	if err != nil {
		return []*Result{{
			Status:       string(types.RepoStatusError),
			ErrorMessage: fmt.Sprintf("list repos: %v", err),
		}}
	}

	// Filter repos with upstream
	var targetRepos []types.Repo
	for _, r := range repos {
		if r.Upstream != "" {
			targetRepos = append(targetRepos, r)
		}
	}

	results := make([]*Result, len(targetRepos))
	var wg sync.WaitGroup
	sem := make(chan struct{}, types.DefaultMaxConcurrency) // limit concurrency to avoid overwhelming network/disk

	for i, r := range targetRepos {
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int, repo types.Repo) {
			defer wg.Done()
			defer func() { <-sem }()
			timeout := defaultTimeout
			if s.shouldUseAgentResolve(repo) {
				timeout = agentResolveTimeout(s.cfg)
			}
			repoCtx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()
			results[idx] = s.SyncRepo(repoCtx, repo)
		}(i, r)
	}
	wg.Wait()

	return results
}

// runPostSyncCommands executes the repo's post-sync commands in order.
// It stops on the first failure. The sync status remains "up_to_date" regardless.
func (s *Syncer) runPostSyncCommands(ctx context.Context, r types.Repo) []types.PostSyncResult {
	if len(r.PostSyncCommands) == 0 {
		return nil
	}

	var results []types.PostSyncResult
	for _, cmd := range r.PostSyncCommands {
		logger.Info("sync: executing post-sync command", "repo", r.Name, "command", cmd.Name, "cmd", cmd.Cmd)
		cmdCtx, cancel := context.WithTimeout(ctx, postSyncCommandTimeout)
		sh, flag := shell()
		c := exec.CommandContext(cmdCtx, sh, flag, cmd.Cmd)
		c.Dir = r.Path

		var stdout, stderr bytes.Buffer
		c.Stdout = &stdout
		c.Stderr = &stderr

		err := c.Run()
		cancel()

		res := types.PostSyncResult{
			Name: cmd.Name,
			Cmd:  cmd.Cmd,
		}

		if err != nil {
			res.Success = false
			res.Error = strings.TrimSpace(stderr.String())
			if res.Error == "" {
				res.Error = err.Error()
			}
			results = append(results, res)
			break // stop on first failure
		}

		res.Success = true
		res.Output = strings.TrimSpace(stdout.String())
		results = append(results, res)
	}

	return results
}

// postSyncError returns a summary error message if any post-sync command failed.
func (s *Syncer) postSyncError(results []types.PostSyncResult) string {
	for _, r := range results {
		if !r.Success {
			return fmt.Sprintf("post-sync command \"%s\" failed: %s", r.Name, r.Error)
		}
	}
	return ""
}

func (s *Syncer) updateRepoStatus(id string, status types.RepoStatus, errMsg string) {
	r, ok := s.store.Get(id)
	if !ok {
		return
	}
	r.Status = status
	r.ErrorMessage = errMsg
	if status == types.RepoStatusUpToDate {
		now := types.Time{Time: time.Now()}
		r.LastSync = &now
	}
	if updateErr := s.store.Update(r); updateErr != nil {
		logger.Error("syncer: failed to update repo status", "repo", r.Name, "error", updateErr)
	}
}

// NewSyncerFromConfig creates a Syncer using config defaults.
func NewSyncerFromConfig(cfg *config.Config, store repo.Store, configDir string) *Syncer {
	var gitOps *git.Operations
	if cfg != nil && cfg.Proxy.Enabled && cfg.Proxy.URL != "" {
		gitOps = git.NewOperationsWithProxy(cfg.Proxy.URL)
	} else {
		gitOps = git.NewOperations()
	}
	return &Syncer{
		gitOps:    gitOps,
		store:     store,
		cfg:       cfg,
		configDir: configDir,
		active:    make(map[string]bool),
	}
}

// notifyResult sends a notification based on the sync result.
func (s *Syncer) notifyResult(repoName string, result *Result) {
	if s.notifier == nil {
		return
	}
	switch result.Status {
	case string(types.RepoStatusUpToDate):
		if result.CommitsPulled > 0 {
			s.notifier.NotifySyncSuccess(repoName, result.CommitsPulled)
		}
	case string(types.RepoStatusResolved):
		s.notifier.NotifyResolved(repoName, len(result.PendingConfirm), result.AgentUsed)
	case string(types.RepoStatusConflict):
		s.notifier.NotifyConflict(repoName, len(result.ConflictFiles))
	case string(types.RepoStatusError):
		s.notifier.NotifyError(repoName, result.ErrorMessage)
	}
}

// recordHistory saves the sync result to the history store.
// Skips recording for 'up_to_date' status as it's just a check, not an actual sync.
// Returns the history record ID if recording succeeded.
func (s *Syncer) recordHistory(result *Result) int64 {
	if s.historyStore == nil {
		return 0
	}
	// Don't record to history if there were no actual changes
	if result.CommitsPulled == 0 && result.Status != string(types.RepoStatusConflict) && result.Status != string(types.RepoStatusError) && result.Status != string(types.RepoStatusResolved) {
		return 0
	}

	// Pre-set summary_status to "pending" if auto-summarization is enabled
	summaryStatus := ""
	if s.cfg != nil && s.cfg.Sync.AutoSummary &&
		result.Status == string(types.RepoStatusUpToDate) && result.CommitsPulled > 0 {
		summaryStatus = string(types.SummaryStatusPending)
	}

	id, err := s.historyStore.Record(history.Record{
		RepoID:         result.RepoID,
		RepoName:       result.RepoName,
		Status:         result.Status,
		CommitsPulled:  result.CommitsPulled,
		ConflictFiles:  result.ConflictFiles,
		AgentUsed:      result.AgentUsed,
		ConflictsFound: result.ConflictsFound,
		AutoResolved:   result.AutoResolved,
		ErrorMessage:   result.ErrorMessage,
		SummaryStatus:  summaryStatus,
		OldHEAD:        result.OldHEAD,
		CreatedAt:      time.Now(),
	})
	if err != nil {
		logger.Error("record history error", "error", err)
		return 0
	}
	result.HistoryID = id
	return id
}

// logResult writes the sync result to the log file.
func (s *Syncer) logResult(result *Result) {
	switch result.Status {
	case string(types.RepoStatusUpToDate):
		if result.CommitsPulled > 0 {
			logger.Info("repo synced", "repo", result.RepoName, "commits_pulled", result.CommitsPulled)
			for _, ps := range result.PostSyncResults {
				if ps.Success {
					logger.Info("post-sync OK", "command", ps.Name)
				} else {
					logger.Error("post-sync failed", "command", ps.Name, "error", ps.Error)
				}
			}
		} else {
			logger.Info("repo already up to date", "repo", result.RepoName)
		}
	case string(types.RepoStatusResolved):
		logger.Info("repo conflicts resolved, awaiting confirmation",
			"repo", result.RepoName,
			"files", len(result.PendingConfirm),
			"agent", result.AgentUsed)
	case string(types.RepoStatusConflict):
		logger.Warn("repo conflicts", "repo", result.RepoName, "files", len(result.ConflictFiles))
	case string(types.RepoStatusError):
		logger.Error("repo sync error", "repo", result.RepoName, "error", result.ErrorMessage)
	}
}

// finalizeResult records history and logs the result.
func (s *Syncer) finalizeResult(result *Result) {
	s.recordHistory(result)
	s.logResult(result)
}

// shell returns the system shell for executing commands.
// Uses "cmd" on Windows, "sh" on all other platforms.
func shell() (string, string) {
	if runtime.GOOS == "windows" {
		return "cmd", "/c"
	}
	return "sh", "-c"
}
