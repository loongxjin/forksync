package sync

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/loongxjin/forksync/engine/internal/agent"
	"github.com/loongxjin/forksync/engine/internal/agent/session"
	"github.com/loongxjin/forksync/engine/internal/config"
	"github.com/loongxjin/forksync/engine/internal/git"
	"github.com/loongxjin/forksync/engine/internal/history"
	"github.com/loongxjin/forksync/engine/internal/logger"
	"github.com/loongxjin/forksync/engine/internal/notify"
	"github.com/loongxjin/forksync/engine/internal/repo"
	respkg "github.com/loongxjin/forksync/engine/internal/resolve"
	"github.com/loongxjin/forksync/engine/pkg/types"

	wfpkg "github.com/loongxjin/forksync/engine/internal/workflow"
)

const (
	defaultTimeout = 5 * time.Minute
	maxDiffSize    = 100 * 1024 // 100KB limit for diff output
)

// Syncer handles repository synchronization.
type Syncer struct {
	gitOps       git.OperationsProvider
	store        repo.Store
	cfgProvider  config.Provider // live config, refreshed each sync via refreshConfig
	cfgSnapshot  *config.Config  // snapshot taken at sync start; use config() to access
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

// config returns the config snapshot for the current sync. Call refreshConfig
// at the start of each sync to pick up the latest settings from disk.
// Never returns nil — returns a zero-value Config if no provider is set.
func (s *Syncer) config() *config.Config {
	if s.cfgSnapshot == nil {
		if s.cfgProvider != nil {
			s.refreshConfig()
		}
		if s.cfgSnapshot == nil {
			return &config.Config{}
		}
	}
	return s.cfgSnapshot
}

// refreshConfig reloads the configuration from the provider so the next sync
// sees the latest settings. Replaces the old reloadConfigAndSessionMgr.
func (s *Syncer) refreshConfig() {
	if s.cfgProvider == nil {
		return
	}
	prevStrategy := ""
	if s.cfgSnapshot != nil {
		prevStrategy = s.cfgSnapshot.Agent.ConflictStrategy
	}
	s.cfgSnapshot = s.cfgProvider.Config()

	// Lazily build or refresh sessionMgr if agent_resolve is now enabled.
	if s.cfgSnapshot.Agent.ConflictStrategy != types.StrategyAgentResolve {
		return
	}
	if s.sessionMgr != nil {
		// Refresh provider so a newly installed/preferred agent is picked up.
		if p, perr := agent.ResolveProvider(s.cfgSnapshot.Agent.Preferred, ""); perr == nil {
			s.sessionMgr.SetProvider(p)
		}
		return
	}
	// agent_resolve is on but no session manager yet — lazily build one.
	provider, perr := agent.ResolveProvider(s.cfgSnapshot.Agent.Preferred, "")
	if perr != nil {
		logger.Info("sync: agent_resolve enabled but no agent available yet",
			"preferred_cfg", s.cfgSnapshot.Agent.Preferred, "error", perr)
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
		"new_strategy", s.cfgSnapshot.Agent.ConflictStrategy,
		"provider", provider.Name(),
	)
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
	s.refreshConfig()

	// Set timeout — use agent timeout if auto-resolve is configured,
	// otherwise the default 5 minutes may SIGKILL long-running agents.
	timeout := defaultTimeout
	if s.shouldUseAgentResolve() {
		timeout = config.AgentTimeout(s.config())
	}
	logger.Info("sync: executeSync starting",
		"repo", r.Name,
		"timeout", timeout,
		"agent_resolve", s.shouldUseAgentResolve(),
		"conflict_strategy", s.config().Agent.ConflictStrategy,
		"sessionMgr_nil", s.sessionMgr == nil,
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
	autoAgentResolve := s.config().Agent.ConflictStrategy == types.StrategyAgentResolve

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
		// Stamp a fresh resolve session id so the frontend can locate this
		// resolve's agent log precisely (by session), not by "newest file".
		wfpkg.SetResolveSessionID(wf, uuid.New().String())
		s.saveWorkflow(r, wf)

		// Read back the session id we just stamped so tryAgentResolve can name
		// its log file precisely (passed explicitly rather than re-derived).
		resolveSessionID := ""
		if step := wfpkg.FindStep(wf, types.StepAgentResolve); step != nil {
			resolveSessionID = step.ResolveSessionID
		}
		resolved, pending := s.tryAgentResolve(ctx, r, mergeResult.Conflicts, resolveSessionID)
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
			// Agent resolved but needs confirmation.
			wfpkg.TransitionAgentResolved(wf, pending.Agent)
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
		// Agent failed. Roll the merge back via the shared Reject path so
		// the repo is not left stuck mid-merge (MERGE_HEAD present, unmerged
		// index entries) across scheduler ticks — same path the interactive
		// resolve uses, instead of the old inline AbortMerge.
		wfpkg.AdvanceStep(wf, types.StepAgentResolve, types.StepStatusFailed, "agent failed to resolve conflicts")
		wfpkg.MarkWorkflowDone(wf, types.WorkflowFailed)
		resolver := respkg.NewResolver(s.gitOps, s.store, s.config(), nil, s.sessionMgr)
		if _, rejectErr := resolver.Reject(ctx, r); rejectErr != nil {
			logger.Warn("sync: reject after agent failure", "repo", r.Name, "error", rejectErr)
		}
		result.Status = string(types.RepoStatusSyncNeeded)
		// Reject clears ErrorMessage; restore a useful one for the UI.
		s.updateRepoStatus(r.ID, types.RepoStatusSyncNeeded, "agent failed; merge rolled back")
		s.saveWorkflow(r, wf)
		s.notifyResult(r.Name, result)
		s.finalizeResult(result)
		return result
	}

	// Manual resolve path: pause at resolve_strategy
	logger.Info("sync: entering MANUAL resolve path (repo left in waiting state)",
		"repo", r.Name,
		"conflict_strategy", s.config().Agent.ConflictStrategy,
		"sessionMgr_nil", s.sessionMgr == nil,
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
//
// The resolve core (drive agent → verify markers → stage) is shared with the
// interactive resolve path via resolve.Resolver.RunAgentResolve. What stays
// sync-specific: the disk-log done frame, auto-commit + post-sync + history,
// and the pending-confirmation hand-off. Workflow/state transitions are owned
// by handleMergeConflicts (the caller), NOT by the Resolver core — the auto
// path's state machine is centralized there.
func (s *Syncer) tryAgentResolve(ctx context.Context, r types.Repo, conflictPaths []string, resolveSessionID string) (bool, *pendingInfo) {
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
		"conflict_strategy", s.config().Agent.ConflictStrategy,
		"confirm_before_commit", s.config().Agent.ConfirmBeforeCommit,
	)

	// Set up log writer for auto-sync background runs so users can replay later.
	// Uses the shared agent.NewResolveLogWriter (same disk-log setup as the
	// interactive resolve path); no live sink — this is a background job.
	streamWriter, closeLog := agent.NewResolveLogWriter(s.configDir, r.Name, resolveSessionID)
	defer closeLog()
	logger.Debug("sync: agent log writer active", "repo", r.Name)

	// Drive the agent + verify + stage via the shared Resolver core. conflictPaths
	// come from the merge result (authoritative); pass them in so the core does
	// not re-detect. The core does NOT commit and does NOT transition state —
	// both are sync-owned.
	resolver := respkg.NewResolver(s.gitOps, s.store, s.config(), nil, s.sessionMgr)
	out, err := resolver.RunAgentResolve(ctx, r, s.resolveStrategyOrDefault(), streamWriter, conflictPaths)
	if err != nil {
		logger.Error("sync: agent resolve failed",
			"repo", r.Name,
			"agent", s.sessionMgr.ProviderName(),
			"error", err,
		)
		return false, nil
	}
	// No conflicts to resolve — nothing to do.
	if out.AgentResult == nil {
		return false, nil
	}
	result := out.AgentResult

	// Verify failed: agent left conflict markers. The core already reported
	// them in out.Unresolved; nothing to stage or commit.
	if !out.Success || len(out.Unresolved) > 0 {
		logger.Error("sync: agent left conflict markers in files",
			"repo", r.Name,
			"agent", s.sessionMgr.ProviderName(),
			"files", out.Unresolved,
			"summary", result.Summary,
		)
		return false, nil
	}

	// Write a terminal 'done' frame to the disk log so readAgentLog reports
	// isRunning=false — but ONLY when commit has actually landed (auto-commit)
	// or when the agent is truly done and waiting for user review (waiting-confirm).
	// Previously this was emitted unconditionally before the commit, which caused
	// a frontend race: readAgentLog saw done, triggered a status refresh, but
	// the commit hadn't landed yet → the frontend's verifyPoll patch papered
	// over that gap by retrying.
	//
	// Now the done frame is emitted inside each branch at the correct timing:
	//  - waiting-confirm: agent finished, right before buildPendingInfo.
	//  - auto-commit: AFTER the commit succeeds, so frontend refresh sees the
	//    terminal state (up_to_date) on the first try — verifyPoll becomes
	//    unnecessary and will be removed.

	// Check if auto-confirm is enabled
	autoConfirm := !s.config().Agent.ConfirmBeforeCommit

	if !autoConfirm {
		logger.Info("sync: tryAgentResolve awaiting user confirmation",
			"repo", r.Name, "reason", "confirm_before_commit=true")
		// Agent is done, write done frame so the frontend can read the log
		// and show diff/summary for the user to review.
		_ = streamWriter.WriteEvent(agent.DoneEventFromResult(result))
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

	// Auto-commit succeeded. Now emit the done frame — the frontend's next
	// status refresh will see up_to_date, no transitional state to retry over.
	_ = streamWriter.WriteEvent(agent.DoneEventFromResult(result))

	return true, nil
}

// resolveStrategyOrDefault returns the resolve strategy from config, or the default.
func (s *Syncer) resolveStrategyOrDefault() string {
	return config.ResolveStrategyOrDefault(s.config())
}

// shouldUseAgentResolve checks whether agent auto-resolve is configured globally.
func (s *Syncer) shouldUseAgentResolve() bool {
	return s.config().Agent.ConflictStrategy == types.StrategyAgentResolve
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

// NewSyncerFromConfig creates a Syncer using a live config provider.
// The provider is queried for the latest config at the start of each sync,
// so settings changes take effect without restarting the app.
func NewSyncerFromConfig(cfgProvider config.Provider, store repo.Store, configDir string, opts ...Option) *Syncer {
	var gitOps git.OperationsProvider
	cfg := cfgProvider.Config()
	if cfg.Proxy.Enabled && cfg.Proxy.URL != "" {
		gitOps = git.NewOperationsWithProxy(cfg.Proxy.URL)
	} else {
		gitOps = git.NewOperations()
	}
	s := &Syncer{
		gitOps:      gitOps,
		store:       store,
		cfgProvider: cfgProvider,
		cfgSnapshot: cfg,
		configDir:   configDir,
		active:      make(map[string]bool),
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
	if s.config().Sync.AutoSummary &&
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
