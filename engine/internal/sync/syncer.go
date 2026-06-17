package sync

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/loongxjin/forksync/engine/internal/agent"
	"github.com/loongxjin/forksync/engine/internal/agent/session"
	"github.com/loongxjin/forksync/engine/internal/config"
	"github.com/loongxjin/forksync/engine/internal/git"
	"github.com/loongxjin/forksync/engine/internal/history"
	"github.com/loongxjin/forksync/engine/internal/logger"
	"github.com/loongxjin/forksync/engine/internal/notify"
	"github.com/loongxjin/forksync/engine/internal/repo"
	"github.com/loongxjin/forksync/engine/pkg/types"

	wfpkg "github.com/loongxjin/forksync/engine/internal/workflow"
)

const (
	defaultTimeout         = 5 * time.Minute
	defaultAgentTimeout    = 10 * time.Minute
	maxDiffSize            = 100 * 1024 // 100KB limit for diff output
)

// Syncer handles repository synchronization.
type Syncer struct {
	gitOps       git.OperationsProvider
	store        repo.Store
	cfg          *config.Config
	notifier     *notify.Notifier
	sessionMgr   *session.Manager
	historyStore *history.Store
	configDir    string // base config directory (e.g. ~/.forksync)
	mu           sync.Mutex
	active       map[string]bool // tracks repos currently syncing
}

// Option configures a Syncer during construction.
type Option func(*Syncer)

// WithNotifier sets the notification handler.
func WithNotifier(n *notify.Notifier) Option {
	return func(s *Syncer) { s.notifier = n }
}

// WithHistoryStore sets the sync history store.
func WithHistoryStore(h *history.Store) Option {
	return func(s *Syncer) { s.historyStore = h }
}

// WithSessionManager sets the agent session manager.
func WithSessionManager(mgr *session.Manager) Option {
	return func(s *Syncer) { s.sessionMgr = mgr }
}

// NewSyncer creates a new Syncer with the given store and options.
func NewSyncer(store repo.Store, opts ...Option) *Syncer {
	s := &Syncer{
		gitOps: git.NewOperations(),
		store:  store,
		active: make(map[string]bool),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
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
	logger.Debug("sync: SyncRepo started",
		"repo", r.Name,
		"id", r.ID,
		"path", r.Path,
		"status", string(r.Status),
		"branch", r.Branch,
		"upstream", r.Upstream,
	)

	upstreamRef := s.gitOps.ResolveUpstreamRef(ctx, r)

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
		result.Workflow = wfpkg.NewWorkflow(r.ID)
		wfpkg.AdvanceStep(result.Workflow, types.StepFetch, types.StepStatusFailed, "sync already in progress")
		wfpkg.MarkWorkflowDone(result.Workflow, types.WorkflowFailed)
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
	wf := wfpkg.NewWorkflow(r.ID)
	result.Workflow = wf
	s.saveWorkflow(r, wf)

	// Phase 1: Pre-checks (conflict state detection)
	logger.Debug("sync: checking conflict state", "repo", r.Name)
	if stopped := s.checkConflictState(ctx, r, result); stopped {
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
	logger.Debug("sync: conflict state check result",
		"repo", r.Name,
		"isMerging", isMerging,
		"unmergedFiles", unmergedFiles,
		"error", err,
	)
	if err == nil && isMerging {
		if len(unmergedFiles) == 0 {
			// All conflicts were resolved but not staged — now auto-staged.
			// MERGE_HEAD still exists, transition to resolved state for user confirmation.
			result.Status = string(types.RepoStatusResolved)
			wfpkg.AdvanceStep(wf, types.StepFetch, types.StepStatusSuccess, "")
			wfpkg.AdvanceStep(wf, types.StepMerge, types.StepStatusSuccess, "")
			wfpkg.AdvanceStep(wf, types.StepCheckConflicts, types.StepStatusSuccess, "")
			wfpkg.AdvanceStep(wf, types.StepResolveStrategy, types.StepStatusSuccess, "")
			wfpkg.MarkStepSkipped(wf, types.StepAgentResolve)
			wfpkg.AdvanceStep(wf, types.StepAcceptChanges, types.StepStatusWaiting, "")
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
		wfpkg.AdvanceStep(wf, types.StepFetch, types.StepStatusSuccess, "")
		wfpkg.AdvanceStep(wf, types.StepMerge, types.StepStatusSuccess, "")
		wfpkg.AdvanceStep(wf, types.StepCheckConflicts, types.StepStatusSuccess,
			fmt.Sprintf("%d files have conflicts", len(unmergedFiles)))
		// If a workflow exists and was waiting at resolve_strategy, preserve waiting status
		if r.Workflow != nil && wfpkg.FindStep(r.Workflow, types.StepResolveStrategy) != nil &&
			wfpkg.FindStep(r.Workflow, types.StepResolveStrategy).Status == types.StepStatusWaiting {
			result.Status = string(types.RepoStatusWaiting)
			wf.Status = types.WorkflowWaiting
			s.updateRepoStatus(r.ID, types.RepoStatusWaiting, result.ErrorMessage)
		} else {
			result.Status = string(types.RepoStatusConflict)
			wfpkg.AdvanceStep(wf, types.StepResolveStrategy, types.StepStatusWaiting, "")
			wfpkg.MarkStepSkipped(wf, types.StepAgentResolve)
			wfpkg.MarkStepSkipped(wf, types.StepAcceptChanges)
			wf.Status = types.WorkflowWaiting
			s.updateRepoStatus(r.ID, types.RepoStatusConflict, result.ErrorMessage)
		}
		s.saveWorkflow(r, wf)
		s.notifyResult(r.Name, result)
		// DO NOT call finalizeResult — this is not a real sync, don't pollute history
		s.logResult(result)
		return true
	}

	logger.Debug("sync: no active merge, checking stored status", "repo", r.Name, "stored_status", string(r.Status))

	// Also check if stored status indicates a conflict state that hasn't been resolved
	if r.Status == types.RepoStatusConflict || r.Status == types.RepoStatusResolving || r.Status == types.RepoStatusResolved || r.Status == types.RepoStatusWaiting {
		result.Status = string(types.RepoStatusConflict)
		result.ErrorMessage = fmt.Sprintf("repository is in %s state, please resolve conflicts before syncing", r.Status)
		wfpkg.AdvanceStep(wf, types.StepFetch, types.StepStatusFailed, result.ErrorMessage)
		wfpkg.MarkWorkflowDone(wf, types.WorkflowFailed)
		s.saveWorkflow(r, wf)
		// DO NOT call finalizeResult — this is not a real sync, don't pollute history
		s.logResult(result)
		return true
	}

	return false
}

// failSync is a helper that populates a failed sync result with consistent workflow/bookkeeping updates.
// It advances the given step to failed, marks the workflow done, sets the result status/message,
// updates repo status, optionally notifies, saves workflow, and finalizes the result.
func (s *Syncer) failSync(r types.Repo, result *Result, wf *types.SyncWorkflow, step types.WorkflowStep, errMsg string, notify bool) *Result {
	wfpkg.AdvanceStep(wf, step, types.StepStatusFailed, errMsg)
	wfpkg.MarkWorkflowDone(wf, types.WorkflowFailed)
	result.Status = string(types.RepoStatusError)
	result.ErrorMessage = errMsg
	s.updateRepoStatus(r.ID, types.RepoStatusError, result.ErrorMessage)
	if notify {
		s.notifyResult(r.Name, result)
	}
	s.saveWorkflow(r, wf)
	s.finalizeResult(result)
	return result
}

// executeSync performs the actual sync: fetch → status check → merge → post-sync commands.
func (s *Syncer) executeSync(ctx context.Context, r types.Repo, result *Result) *Result {
	wf := result.Workflow
	// Pick up config changes made via the settings UI since the server started.
	// The Syncer's cfg and sessionMgr are snapshotted at boot; without this a
	// user flipping conflict_strategy to agent_resolve would have to restart the
	// app before auto-resolve took effect.
	s.reloadConfigAndSessionMgr()

	// Set timeout — use agent timeout if auto-resolve is configured,
	// otherwise the default 5 minutes may SIGKILL long-running agents.
	timeout := defaultTimeout
	if s.shouldUseAgentResolve() {
		timeout = agentResolveTimeout(s.cfg)
	}
	logger.Info("sync: executeSync starting",
		"repo", r.Name,
		"timeout", timeout,
		"agent_resolve", s.shouldUseAgentResolve(),
		"conflict_strategy", func() string {
			if s.cfg != nil {
				return s.cfg.Agent.ConflictStrategy
			}
			return "<nil cfg>"
		}(),
		"sessionMgr_nil", s.sessionMgr == nil,
		"cfg_nil", s.cfg == nil,
	)
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
	wfpkg.AdvanceStep(wf, types.StepFetch, types.StepStatusRunning, "")
	s.saveWorkflow(r, wf)
	if err := s.gitOps.Fetch(ctx, r); err != nil {
		return s.failSync(r, result, wf, types.StepFetch, fmt.Sprintf("fetch failed: %v", err), true)
	}
	wfpkg.AdvanceStep(wf, types.StepFetch, types.StepStatusSuccess, "")
	s.saveWorkflow(r, wf)

	// Step 2: Check ahead/behind
	statusResult, err := s.gitOps.Status(ctx, r)
	if err != nil {
		return s.failSync(r, result, wf, types.StepMerge, fmt.Sprintf("status check failed: %v", err), false)
	}

	logger.Debug("sync: status result",
		"repo", r.Name,
		"branch", statusResult.Branch,
		"ahead", statusResult.AheadBy,
		"behind", statusResult.BehindBy,
		"oldHEAD", result.OldHEAD,
	)

	if statusResult.BehindBy == 0 {
		wfpkg.AdvanceStep(wf, types.StepMerge, types.StepStatusSuccess, "")
		wfpkg.MarkStepSkipped(wf, types.StepCheckConflicts)
		wfpkg.MarkStepSkipped(wf, types.StepResolveStrategy)
		wfpkg.MarkStepSkipped(wf, types.StepAgentResolve)
		wfpkg.MarkStepSkipped(wf, types.StepAcceptChanges)
		wfpkg.AdvanceStep(wf, types.StepCommit, types.StepStatusSuccess, "")
		wfpkg.MarkWorkflowDone(wf, types.WorkflowSuccess)
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
		wf.OldHEAD = head
	}

	wfpkg.AdvanceStep(wf, types.StepMerge, types.StepStatusRunning, "")
	s.saveWorkflow(r, wf)
	mergeResult, err := s.gitOps.Merge(ctx, r)
	if err != nil {
		return s.failSync(r, result, wf, types.StepMerge, fmt.Sprintf("merge failed: %v", err), true)
	}
	wfpkg.AdvanceStep(wf, types.StepMerge, types.StepStatusSuccess, "")
	s.saveWorkflow(r, wf)

	// Step 4: Check conflicts
	wfpkg.AdvanceStep(wf, types.StepCheckConflicts, types.StepStatusRunning, "")
	s.saveWorkflow(r, wf)
	if mergeResult.HasConflicts {
		wfpkg.AdvanceStep(wf, types.StepCheckConflicts, types.StepStatusSuccess,
			fmt.Sprintf("%d files have conflicts", len(mergeResult.Conflicts)))
		s.saveWorkflow(r, wf)
		return s.handleMergeConflicts(ctx, r, result, mergeResult)
	}
	wfpkg.AdvanceStep(wf, types.StepCheckConflicts, types.StepStatusSuccess, "")
	wfpkg.MarkStepSkipped(wf, types.StepResolveStrategy)
	wfpkg.MarkStepSkipped(wf, types.StepAgentResolve)
	wfpkg.MarkStepSkipped(wf, types.StepAcceptChanges)

	// Step 7: Commit (and post-sync)
	wfpkg.AdvanceStep(wf, types.StepCommit, types.StepStatusRunning, "")
	s.saveWorkflow(r, wf)
	result.Status = string(types.RepoStatusUpToDate)
	s.updateRepoStatus(r.ID, types.RepoStatusUpToDate, "")
	result.PostSyncResults = wfpkg.RunPostSyncCommands(ctx, r)
	if postSyncErr := wfpkg.PostSyncError(result.PostSyncResults); postSyncErr != "" {
		result.ErrorMessage = postSyncErr
		s.updateRepoStatus(r.ID, types.RepoStatusUpToDate, result.ErrorMessage)
	}
	wfpkg.AdvanceStep(wf, types.StepCommit, types.StepStatusSuccess, "")
	wfpkg.MarkWorkflowDone(wf, types.WorkflowSuccess)
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
	wfpkg.AdvanceStep(wf, types.StepResolveStrategy, types.StepStatusRunning, "")
	s.saveWorkflow(r, wf)

	// Determine auto-resolve strategy from global config
	autoAgentResolve := false
	if s.cfg != nil && s.cfg.Agent.ConflictStrategy == types.StrategyAgentResolve {
		autoAgentResolve = true
	}

	logger.Info("sync: handleMergeConflicts strategy decision",
		"repo", r.Name,
		"conflicts", len(mergeResult.Conflicts),
		"conflict_strategy", s.resolveStrategyOrDefault(),
		"autoAgentResolve", autoAgentResolve,
		"sessionMgr_nil", s.sessionMgr == nil,
	)

	if autoAgentResolve && s.sessionMgr != nil {
		wfpkg.AdvanceStep(wf, types.StepResolveStrategy, types.StepStatusSuccess, "")
		wfpkg.AdvanceStep(wf, types.StepAgentResolve, types.StepStatusRunning, "")
		s.saveWorkflow(r, wf)

		resolved, pending := s.tryAgentResolve(ctx, r, mergeResult.Conflicts)
		if resolved {
			// Agent resolved and auto-committed
			wfpkg.AdvanceStep(wf, types.StepAgentResolve, types.StepStatusSuccess,
				fmt.Sprintf("resolved by %s", s.sessionMgr.ProviderName()))
			wfpkg.MarkStepSkipped(wf, types.StepAcceptChanges)
			wfpkg.AdvanceStep(wf, types.StepCommit, types.StepStatusSuccess, "")
			wfpkg.MarkWorkflowDone(wf, types.WorkflowSuccess)
			result.Status = string(types.RepoStatusUpToDate)
			result.AutoResolved = len(mergeResult.Conflicts)
			result.PostSyncResults = wfpkg.RunPostSyncCommands(ctx, r)
			if postSyncErr := wfpkg.PostSyncError(result.PostSyncResults); postSyncErr != "" {
				result.ErrorMessage = postSyncErr
				s.updateRepoStatus(r.ID, types.RepoStatusUpToDate, result.ErrorMessage)
			} else {
				s.updateRepoStatus(r.ID, types.RepoStatusUpToDate, "")
			}
			s.saveWorkflow(r, wf)
			s.notifyResult(r.Name, result)
			s.finalizeResult(result)
			return result
		}
		if pending != nil {
			// Agent resolved but needs confirmation
			wfpkg.AdvanceStep(wf, types.StepAgentResolve, types.StepStatusSuccess,
				fmt.Sprintf("resolved by %s", pending.Agent))
			wfpkg.AdvanceStep(wf, types.StepAcceptChanges, types.StepStatusWaiting, "")
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
			// History recorded when user accepts the resolution
			return result
		}
		// Agent failed
		wfpkg.AdvanceStep(wf, types.StepAgentResolve, types.StepStatusFailed, "agent failed to resolve conflicts")
		wfpkg.MarkWorkflowDone(wf, types.WorkflowFailed)
		result.Status = string(types.RepoStatusConflict)
		s.updateRepoStatus(r.ID, types.RepoStatusConflict, "")
		s.saveWorkflow(r, wf)
		s.notifyResult(r.Name, result)
		s.finalizeResult(result)
		return result
	}

	// Manual resolve path: pause at resolve_strategy
	logger.Info("sync: entering MANUAL resolve path (repo left in waiting state)",
		"repo", r.Name,
		"reason", func() string {
			if s.cfg == nil {
				return "cfg is nil"
			}
			if s.cfg.Agent.ConflictStrategy != types.StrategyAgentResolve {
				return "conflict_strategy != agent_resolve (is " + s.cfg.Agent.ConflictStrategy + ")"
			}
			if s.sessionMgr == nil {
				return "sessionMgr is nil (no agent available at server boot)"
			}
			return "unknown"
		}(),
	)
	wfpkg.AdvanceStep(wf, types.StepResolveStrategy, types.StepStatusWaiting, "")
	wfpkg.MarkStepSkipped(wf, types.StepAgentResolve)
	wfpkg.MarkStepSkipped(wf, types.StepAcceptChanges)
	wf.Status = types.WorkflowWaiting
	result.Status = string(types.RepoStatusWaiting)
	s.updateRepoStatus(r.ID, types.RepoStatusWaiting, "")
	s.saveWorkflow(r, wf)
	s.notifyResult(r.Name, result)
	// History recorded when user manually resolves and commits
	return result
}

// tryAgentResolve attempts to resolve conflicts using the agent CLI.
// Returns (resolved, pending):
//   - resolved=true, pending=nil: all conflicts resolved and committed (auto-confirm)
//   - resolved=false, pending=*pendingInfo: conflicts resolved but awaiting user confirmation
//   - resolved=false, pending=nil: agent failed to resolve
func (s *Syncer) tryAgentResolve(ctx context.Context, r types.Repo, conflictPaths []string) (bool, *pendingInfo) {
	if s.sessionMgr == nil {
		logger.Warn("sync: tryAgentResolve skipped — sessionMgr is nil",
			"repo", r.Name,
			"hint", "agent_resolve strategy requires a session manager; server may have started before an agent was installed",
		)
		return false, nil
	}

	logger.Info("sync: tryAgentResolve starting",
		"repo", r.Name,
		"conflicts", len(conflictPaths),
		"provider", s.sessionMgr.ProviderName(),
		"conflict_strategy", func() string {
			if s.cfg != nil {
				return s.cfg.Agent.ConflictStrategy
			}
			return "<nil>"
		}(),
		"confirm_before_commit", func() bool {
			if s.cfg != nil {
				return s.cfg.Agent.ConfirmBeforeCommit
			}
			return false
		}(),
	)

	// Create or reuse a session for this repo
	sess, err := s.sessionMgr.GetOrCreate(ctx, r.ID, r.Path)
	if err != nil {
		logger.Error("sync: tryAgentResolve GetOrCreate session failed",
			"repo", r.Name, "error", err)
		return false, nil
	}
	logger.Info("sync: tryAgentResolve session ready",
		"repo", r.Name, "session_id", sess.ID, "is_new", sess.IsNew)

	// Determine resolve sub-strategy for the agent prompt
	resolveStrategy := s.resolveStrategyOrDefault()

	// Determine prompt language from config
	language := "zh"
	if s.cfg != nil && s.cfg.Sync.SummaryLanguage != "" {
		language = s.cfg.Sync.SummaryLanguage
	}

	// Set up log writer for auto-sync background runs so users can replay later
	var streamWriter *agent.StreamWriter
	lw, lwErr := agent.NewLogWriter(s.configDir, r.Name)
	if lwErr != nil {
		logger.Warn("sync: failed to create agent log writer", "repo", r.Name, "error", lwErr)
	}
	if lw != nil {
		defer lw.Close()
		streamWriter = lw.StreamWriter()
			logger.Debug("sync: agent log writer active", "repo", r.Name)
	}

	// Resolve conflicts via agent
	logger.Info("sync: tryAgentResolve invoking agent",
		"repo", r.Name,
		"agent", s.sessionMgr.ProviderName(),
		"resolve_strategy", resolveStrategy,
		"language", language,
		"conflicts", len(conflictPaths),
	)
	result, err := s.sessionMgr.ResolveConflicts(ctx, r.ID, r.Path, conflictPaths, resolveStrategy, language, streamWriter)
	if err != nil {
			logger.Error("sync: agent resolve failed",
			"repo", r.Name,
			"agent", s.sessionMgr.ProviderName(),
			"error", err,
		)
		return false, nil
	}
	if !result.Success {
			logger.Error("sync: agent reported unsuccessful resolve",
			"repo", r.Name,
			"agent", s.sessionMgr.ProviderName(),
			"summary", result.Summary,
		)
		return false, nil
	}

	// The agent adapters don't populate ResolvedFiles, but we know exactly
	// which files had conflicts. Tell verifyAndStageResolvedFiles to check
	// and stage those so the subsequent commit succeeds.
	result.ResolvedFiles = conflictPaths
	// Populate Diff so the disk-log done frame carries it for replay.
	if diffBytes, dErr := s.gitOps.Diff(ctx, r.Path); dErr == nil {
		result.Diff = string(diffBytes)
	}

	// Write a terminal 'done' frame to the disk log so readAgentLog reports
	// isRunning=false. This carries ResolvedFiles/Diff/AgentName/Summary so the
	// frontend can restore resolve details when replaying the log.
	if streamWriter != nil {
		_ = streamWriter.WriteEvent(agent.DoneEventFromResult(result))
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

	logger.Debug("sync: auto-confirm check",
		"repo", r.Name,
		"autoConfirm", autoConfirm,
		"confirmBeforeCommit", s.cfg.Agent.ConfirmBeforeCommit,
		"cfg_nil", s.cfg == nil,
	)

	if !autoConfirm {
		logger.Info("sync: tryAgentResolve awaiting user confirmation",
			"repo", r.Name, "reason", "confirm_before_commit=true")
		return s.buildPendingInfo(ctx, r, result)
	}

	// Complete the merge with a commit
	commitMsg := fmt.Sprintf("Merge upstream changes (auto-resolved by %s)", s.sessionMgr.ProviderName())
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

	// Agent logs are meaningless once auto-resolved and committed.
	agent.DeleteAllLogs(s.configDir, r.Name)

	return true, nil
}

// resolveStrategyOrDefault returns the resolve strategy from config, or the default.
func (s *Syncer) resolveStrategyOrDefault() string {
	return config.ResolveStrategyOrDefault(s.cfg)
}

// shouldUseAgentResolve checks whether agent auto-resolve is configured globally.
func (s *Syncer) shouldUseAgentResolve() bool {
	if s.cfg != nil {
		return s.cfg.Agent.ConflictStrategy == types.StrategyAgentResolve
	}
	return false
}

// reloadConfigAndSessionMgr picks up config changes made via the settings UI
// since the server started. The Syncer's cfg/sessionMgr are snapshotted at
// boot (in app.BuildDeps), so without this a user flipping conflict_strategy
// to agent_resolve (or installing an agent after launch) would need to restart
// the whole app before auto-resolve took effect.
//
// It reloads s.cfg from disk, and if agent_resolve is now enabled but the
// session manager was never built (because no agent was available at boot, or
// the strategy was off), it lazily constructs one. Safe to call on every sync.
func (s *Syncer) reloadConfigAndSessionMgr() {
	if s.configDir == "" {
		return
	}
	mgr := config.NewManagerWithDir(s.configDir)
	cfg, err := mgr.Load()
	if err != nil {
		return // keep using the in-memory snapshot
	}
	prevStrategy := ""
	if s.cfg != nil {
		prevStrategy = s.cfg.Agent.ConflictStrategy
	}
	s.cfg = cfg

	// Nothing to (re)build if agent_resolve isn't enabled.
	if cfg.Agent.ConflictStrategy != types.StrategyAgentResolve {
		return
	}
	// Already have a session manager — just refresh its provider so a newly
	// installed/preferred agent is picked up without a restart.
	if s.sessionMgr != nil {
		reg := agent.NewRegistry(cfg.Agent.Preferred)
		if p, perr := reg.GetPreferred(); perr == nil {
			s.sessionMgr.SetProvider(p)
		}
		return
	}
	// agent_resolve is on but no session manager yet — lazily build one now
	// that an agent may have become available.
	reg := agent.NewRegistry(cfg.Agent.Preferred)
	provider, perr := reg.GetPreferred()
	if perr != nil {
		logger.Info("sync: agent_resolve enabled but no agent available yet",
			"preferred_cfg", cfg.Agent.Preferred, "error", perr)
		return
	}
	sessionStore := session.NewSessionStore(filepath.Join(s.configDir, "sessions"))
	if initErr := sessionStore.Init(); initErr != nil {
		logger.Error("sync: failed to init session store for lazy sessionMgr", "error", initErr)
		return
	}
	s.sessionMgr = session.NewManager(sessionStore, provider)
	logger.Info("sync: lazily created session manager after config change",
		"prev_strategy", prevStrategy,
		"new_strategy", cfg.Agent.ConflictStrategy,
		"provider", provider.Name(),
	)
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
// Falls back to defaultAgentTimeout if no config is available.
func agentResolveTimeout(cfg *config.Config) time.Duration {
	if cfg != nil && cfg.Agent.Timeout != "" {
		if d, err := time.ParseDuration(cfg.Agent.Timeout); err == nil && d > 0 {
			return d
		}
	}
	return defaultAgentTimeout
}

// verifyAndStageResolvedFiles checks that resolved files have no conflict
// markers and stages them. It delegates to the shared git seam
// (OperationsProvider.FilterResolvedFiles) so the verify-and-stage step has one
// implementation across the Conflict Resolver and the auto-sync path.
func (s *Syncer) verifyAndStageResolvedFiles(ctx context.Context, r types.Repo, result *agent.AgentResult) bool {
	stillConflicted := s.gitOps.FilterResolvedFiles(ctx, r.Path, result.ResolvedFiles)
	if len(stillConflicted) > 0 {
		logger.Error("sync: agent left conflict markers in files",
			"repo", r.Name,
			"agent", s.sessionMgr.ProviderName(),
			"files", stillConflicted,
			"summary", result.Summary,
		)
		return false
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
		const limit = maxDiffSize
		if len(diff) > limit {
			diff = diff[:limit] + "\n\n... (diff truncated)"
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
			// Note: executeSync already sets its own timeout based on agent config,
			// so we don't add a second timeout layer here to avoid unpredictable
			// interactions between the two.
			results[idx] = s.SyncRepo(ctx, repo)
		}(i, r)
	}
	wg.Wait()

	return results
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
		r.BehindBy = 0
	}
	if updateErr := s.store.Update(r); updateErr != nil {
		logger.Error("syncer: failed to update repo status", "repo", r.Name, "error", updateErr)
	}
}

// NewSyncerFromConfig creates a Syncer using config defaults.
func NewSyncerFromConfig(cfg *config.Config, store repo.Store, configDir string, opts ...Option) *Syncer {
	var gitOps git.OperationsProvider
	if cfg != nil && cfg.Proxy.Enabled && cfg.Proxy.URL != "" {
		gitOps = git.NewOperationsWithProxy(cfg.Proxy.URL)
	} else {
		gitOps = git.NewOperations()
	}
	s := &Syncer{
		gitOps:    gitOps,
		store:     store,
		cfg:       cfg,
		configDir: configDir,
		active:    make(map[string]bool),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
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

// shouldRecordHistory returns true if the sync result is worth recording.
// Skip no-op syncs (up_to_date with 0 commits) to avoid polluting the log.
func shouldRecordHistory(result *Result) bool {
	if result.CommitsPulled > 0 {
		return true
	}
	switch types.RepoStatus(result.Status) {
	case types.RepoStatusConflict, types.RepoStatusError,
		types.RepoStatusResolved, types.RepoStatusWaiting:
		return true
	}
	return false
}

// recordHistory saves the sync result to the history store.
// Skips recording for 'up_to_date' status as it's just a check, not an actual sync.
// Returns the history record ID if recording succeeded.
func (s *Syncer) recordHistory(result *Result) int64 {
	if s.historyStore == nil {
		return 0
	}
	if !shouldRecordHistory(result) {
		return 0
	}

	// Pre-set summary_status to "pending" if auto-summarization is enabled
	summaryStatus := ""
	if s.cfg != nil && s.cfg.Sync.AutoSummary &&
		result.Status == string(types.RepoStatusUpToDate) && result.CommitsPulled > 0 {
		summaryStatus = string(types.SummaryStatusPending)
	}

	id, err := s.historyStore.Insert(history.Record{
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
