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
	Files   []string
	Diff    string
	Summary string
	Agent   string
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

	// Check if already syncing
	s.mu.Lock()
	if s.active[r.ID] {
		s.mu.Unlock()
		result.Status = string(types.RepoStatusError)
		result.ErrorMessage = "sync already in progress"
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
			s.updateRepoStatus(r.ID, types.RepoStatusResolved, "")
			s.notifyResult(r.Name, result)
			s.logResult(result)
			return result
		}
		result.Status = string(types.RepoStatusConflict)
		result.ConflictFiles = unmergedFiles
		result.ErrorMessage = "repository has unresolved merge conflicts, please resolve conflicts before syncing"
		result.ConflictsFound = len(unmergedFiles)
		s.updateRepoStatus(r.ID, types.RepoStatusConflict, result.ErrorMessage)
		s.notifyResult(r.Name, result)
		// DO NOT call finalizeResult — this is not a real sync, don't pollute history
		s.logResult(result)
		return result
	}

	// Also check if stored status indicates a conflict state that hasn't been resolved
	if r.Status == types.RepoStatusConflict || r.Status == types.RepoStatusResolving || r.Status == types.RepoStatusResolved {
		result.Status = string(types.RepoStatusConflict)
		result.ErrorMessage = fmt.Sprintf("repository is in %s state, please resolve conflicts before syncing", r.Status)
		// DO NOT call finalizeResult — this is not a real sync, don't pollute history
		s.logResult(result)
		return result
	}

	// Set timeout
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	// Update status to syncing
	r.Status = types.RepoStatusSyncing
	if updateErr := s.store.Update(r); updateErr != nil {
		logger.Error("syncer: failed to update repo to syncing", "repo", r.Name, "error", updateErr)
	}

	// Step 1: Fetch
	if err := s.gitOps.Fetch(ctx, r); err != nil {
		result.Status = string(types.RepoStatusError)
		result.ErrorMessage = fmt.Sprintf("fetch failed: %v", err)
		s.updateRepoStatus(r.ID, types.RepoStatusError, result.ErrorMessage)
		s.notifyResult(r.Name, result)
		s.finalizeResult(result)
		return result
	}

	// Step 2: Check ahead/behind
	statusResult, err := s.gitOps.Status(ctx, r)
	if err != nil {
		result.Status = string(types.RepoStatusError)
		result.ErrorMessage = fmt.Sprintf("status check failed: %v", err)
		s.updateRepoStatus(r.ID, types.RepoStatusError, result.ErrorMessage)
		s.finalizeResult(result)
		return result
	}

	if statusResult.BehindBy == 0 {
		result.Status = string(types.RepoStatusUpToDate)
		result.CommitsPulled = 0
		s.updateRepoStatus(r.ID, types.RepoStatusUpToDate, "")
		s.finalizeResult(result)
		return result
	}

	result.CommitsPulled = statusResult.BehindBy

	// Step 3: Merge
	// Remember HEAD before merge for summarizer
	if head, err := s.gitOps.GetHEAD(ctx, r.Path); err == nil {
		result.OldHEAD = head
	}

	mergeResult, err := s.gitOps.Merge(ctx, r)
	if err != nil {
		result.Status = string(types.RepoStatusError)
		result.ErrorMessage = fmt.Sprintf("merge failed: %v", err)
		s.updateRepoStatus(r.ID, types.RepoStatusError, result.ErrorMessage)
		s.notifyResult(r.Name, result)
		s.finalizeResult(result)
		return result
	}

	if mergeResult.HasConflicts {
		return s.handleMergeConflicts(ctx, r, result, mergeResult)
	}

	// Success
	result.Status = string(types.RepoStatusUpToDate)
	s.updateRepoStatus(r.ID, types.RepoStatusUpToDate, "")
	result.PostSyncResults = s.runPostSyncCommands(ctx, r)
	if postSyncErr := s.postSyncError(result.PostSyncResults); postSyncErr != "" {
		result.ErrorMessage = postSyncErr
		s.updateRepoStatus(r.ID, types.RepoStatusUpToDate, result.ErrorMessage)
	}
	s.notifyResult(r.Name, result)
	s.finalizeResult(result)
	return result
}

// handleMergeConflicts processes merge conflicts, attempting agent auto-resolve if configured.
// Returns the final result for the sync operation.
func (s *Syncer) handleMergeConflicts(ctx context.Context, r types.Repo, result *Result, mergeResult *git.MergeResult) *Result {
	result.ConflictsFound = len(mergeResult.Conflicts)
	result.ConflictFiles = mergeResult.Conflicts

	// Try agent auto-resolve if configured and session manager available
	strategy := r.ConflictStrategy
	if strategy == "" && s.cfg != nil {
		strategy = s.cfg.Agent.ConflictStrategy
	}
	if strategy == types.StrategyAgentResolve && s.sessionMgr != nil {
		resolved, pending := s.tryAgentResolve(ctx, r, mergeResult.Conflicts)
		if resolved {
			result.Status = string(types.RepoStatusUpToDate)
			result.AutoResolved = len(mergeResult.Conflicts)
			s.updateRepoStatus(r.ID, types.RepoStatusUpToDate, "")
			s.notifyResult(r.Name, result)
			s.finalizeResult(result)
			return result
		}
		if pending != nil {
			result.Status = string(types.RepoStatusResolved)
			result.AgentUsed = pending.Agent
			result.AutoResolved = len(pending.Files)
			result.PendingConfirm = pending.Files
			result.AgentResult = &types.AgentResolveResult{
				Success:       true,
				ResolvedFiles: pending.Files,
				Diff:          pending.Diff,
				Summary:       pending.Summary,
				AgentName:     pending.Agent,
			}
			s.updateRepoStatus(r.ID, types.RepoStatusResolved, "")
			s.notifyResult(r.Name, result)
			s.finalizeResult(result)
			return result
		}
	}

	result.Status = string(types.RepoStatusConflict)
	s.updateRepoStatus(r.ID, types.RepoStatusConflict, "")
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
	_, err := s.sessionMgr.GetOrCreate(ctx, r.ID, r.Path)
	if err != nil {
		return false, nil
	}

	// Determine resolve sub-strategy for the agent prompt
	resolveStrategy := ""
	if s.cfg != nil {
		resolveStrategy = s.cfg.Agent.ResolveStrategy
	}
	if resolveStrategy == "" {
		resolveStrategy = types.ResolveStrategyPreserveOurs
	}

	// Resolve conflicts via agent
	result, err := s.sessionMgr.ResolveConflicts(ctx, r.ID, r.Path, conflictPaths, resolveStrategy)
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

	// Verify no conflict markers remain in resolved files
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
		return false, nil
	}

	// Stage resolved files
	for _, file := range result.ResolvedFiles {
		if err := s.gitOps.StageFile(ctx, r.Path, file); err != nil {
			logger.Warn("sync: failed to stage resolved file",
				"repo", r.Name,
				"file", file,
				"error", err,
			)
			return false, nil
		}
	}

	// Check staged changes
	if checkErr := s.gitOps.CheckStaged(ctx, r.Path); checkErr != nil {
		// Log but don't fail — whitespace issues are non-critical
		logger.Debug("sync: staged changes check found issues", "repo", r.Name, "error", checkErr)
	}

	// Check if auto-confirm is enabled
	autoConfirm := true
	if s.cfg != nil {
		autoConfirm = !s.cfg.Agent.ConfirmBeforeCommit
	}

	if !autoConfirm {
		// Agent resolved successfully but user wants to confirm — stop before commit
		logger.Info("sync: agent resolved conflicts, awaiting user confirmation",
			"repo", r.Name,
			"agent", s.sessionMgr.ProviderName(),
			"files", result.ResolvedFiles,
		)
		// Get staged diff for user review
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

	// Complete the merge with a commit
	commitMsg := fmt.Sprintf("Merge upstream (auto-resolved by %s)", s.sessionMgr.ProviderName())
	if err := s.gitOps.Commit(ctx, r.Path, commitMsg, true); err != nil {
		logger.Warn("sync: failed to commit resolved conflicts",
			"repo", r.Name,
			"agent", s.sessionMgr.ProviderName(),
			"error", err,
		)
		return false, nil
	}

	return true, nil
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
			repoCtx, cancel := context.WithTimeout(ctx, defaultTimeout)
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
func NewSyncerFromConfig(cfg *config.Config, store repo.Store) *Syncer {
	var gitOps *git.Operations
	if cfg != nil && cfg.Proxy.Enabled && cfg.Proxy.URL != "" {
		gitOps = git.NewOperationsWithProxy(cfg.Proxy.URL)
	} else {
		gitOps = git.NewOperations()
	}
	return &Syncer{
		gitOps: gitOps,
		store:  store,
		cfg:    cfg,
		active: make(map[string]bool),
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
