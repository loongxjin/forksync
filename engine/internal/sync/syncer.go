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
	"github.com/loongxjin/forksync/engine/internal/conflict"
	"github.com/loongxjin/forksync/engine/internal/config"
	"github.com/loongxjin/forksync/engine/internal/git"
	"github.com/loongxjin/forksync/engine/internal/history"
	"github.com/loongxjin/forksync/engine/internal/logger"
	"github.com/loongxjin/forksync/engine/internal/notify"
	"github.com/loongxjin/forksync/engine/internal/repo"
	"github.com/loongxjin/forksync/engine/internal/summarizer"
	"github.com/loongxjin/forksync/engine/pkg/types"
)

const (
	defaultTimeout        = 5 * time.Minute
	postSyncCommandTimeout = 60 * time.Second
)

// Syncer handles repository synchronization.
type Syncer struct {
	gitOps        *git.Operations
	store         repo.Store
	cfg           *config.Config
	notifier      *notify.Notifier
	sessionMgr    *session.Manager
	historyStore  *history.Store
	summarizer    *summarizer.Summarizer
	mu            sync.Mutex
	active        map[string]bool // tracks repos currently syncing
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
	s.notifier = n
}

// SetSessionManager sets the agent session manager for auto-conflict resolution.
func (s *Syncer) SetSessionManager(mgr *session.Manager) {
	s.sessionMgr = mgr
}

// SetHistoryStore sets the sync history store for recording sync results.
func (s *Syncer) SetHistoryStore(h *history.Store) {
	s.historyStore = h
}

// SetSummarizer sets the AI summarizer for generating sync summaries.
func (s *Syncer) SetSummarizer(sm *summarizer.Summarizer) {
	s.summarizer = sm
}

// Result contains the result of syncing a single repo.
type Result struct {
	RepoID          string
	RepoName        string
	RepoPath        string // used by summarizer to get commit list
	UpstreamRef     string // upstream remote/branch ref for commit diff, e.g. "upstream/main"
	OldHEAD         string // HEAD before merge, used to compute pulled commits
	Status          string // types.RepoStatus values: synced, conflict, up_to_date, error
	CommitsPulled   int
	ConflictFiles   []string
	ErrorMessage    string
	AgentUsed       string    // agent name if auto-resolve was attempted
	ConflictsFound  int       // number of conflicts detected
	AutoResolved    int       // number of files auto-resolved by agent
	PendingConfirm  []string  // files pending user confirmation
	PostSyncResults []types.PostSyncResult
	HistoryID       int64     // ID of the created history record
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
		PostSyncResults: r.PostSyncResults,
	}
}

// SyncRepo syncs a single repository.
func (s *Syncer) SyncRepo(ctx context.Context, r types.Repo) *Result {
	// Compute upstream ref for summarizer (same logic as mergeCLI)
	remoteName := "upstream"
	if r.Upstream == "" {
		remoteName = "origin"
	}
	branch := r.Branch
	if branch == "" {
		if b, err := s.gitOps.GetCurrentBranch(ctx, r.Path); err == nil {
			branch = b
		}
	}
	if branch == "" {
		branch = "main"
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
		result.Status = "error"
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
	isMerging, unmergedFiles, err := s.gitOps.IsMergingState(ctx, r.Path)
	if err == nil && isMerging {
		result.Status = "conflict"
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
		result.Status = "conflict"
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
		result.Status = "error"
		result.ErrorMessage = fmt.Sprintf("fetch failed: %v", err)
		s.updateRepoStatus(r.ID, types.RepoStatusError, result.ErrorMessage)
		s.notifyResult(r.Name, result)
		s.finalizeResult(result)
		return result
	}

	// Step 2: Check ahead/behind
	statusResult, err := s.gitOps.Status(ctx, r)
	if err != nil {
		result.Status = "error"
		result.ErrorMessage = fmt.Sprintf("status check failed: %v", err)
		s.updateRepoStatus(r.ID, types.RepoStatusError, result.ErrorMessage)
		s.finalizeResult(result)
		return result
	}

	if statusResult.BehindBy == 0 {
		result.Status = string(types.RepoStatusUpToDate)
		result.CommitsPulled = 0
		s.updateRepoStatus(r.ID, types.RepoStatusSynced, "")
		s.finalizeResult(result)
		return result
	}

	result.CommitsPulled = statusResult.BehindBy

	// Step 3: Merge
	// Remember HEAD before merge for summarizer
	if out, err := exec.CommandContext(ctx, "git", "-C", r.Path, "rev-parse", "HEAD").Output(); err == nil {
		result.OldHEAD = strings.TrimSpace(string(out))
	}

	mergeResult, err := s.gitOps.Merge(ctx, r)
	if err != nil {
		result.Status = "error"
		result.ErrorMessage = fmt.Sprintf("merge failed: %v", err)
		s.updateRepoStatus(r.ID, types.RepoStatusError, result.ErrorMessage)
		s.notifyResult(r.Name, result)
		s.finalizeResult(result)
		return result
	}

	if mergeResult.HasConflicts {
		result.ConflictsFound = len(mergeResult.Conflicts)
		result.ConflictFiles = mergeResult.Conflicts

		// Step 4: Try agent auto-resolve if configured and session manager available
		strategy := r.ConflictStrategy
		if strategy == "" && s.cfg != nil {
			strategy = s.cfg.Agent.ConflictStrategy
		}
		if strategy == "agent_resolve" && s.sessionMgr != nil {
			resolved := s.tryAgentResolve(ctx, r, mergeResult.Conflicts)
			if resolved {
				result.Status = "synced"
				result.AutoResolved = len(mergeResult.Conflicts)
				s.updateRepoStatus(r.ID, types.RepoStatusSynced, "")
				s.notifyResult(r.Name, result)
				s.finalizeResult(result)
				return result
			}
			// Agent resolve failed or needs confirmation — fall through to conflict status
		}

		result.Status = "conflict"
		s.updateRepoStatus(r.ID, types.RepoStatusConflict, "")
		s.notifyResult(r.Name, result)
		s.finalizeResult(result)
		return result
	}

	// Success
	result.Status = "synced"
	s.updateRepoStatus(r.ID, types.RepoStatusSynced, "")
	result.PostSyncResults = s.runPostSyncCommands(ctx, r)
	if postSyncErr := s.postSyncError(result.PostSyncResults); postSyncErr != "" {
		result.ErrorMessage = postSyncErr
		s.updateRepoStatus(r.ID, types.RepoStatusSynced, result.ErrorMessage)
	}
	s.notifyResult(r.Name, result)
	s.finalizeResult(result)
	return result
}

// tryAgentResolve attempts to resolve conflicts using the agent CLI.
// Returns true if all conflicts were successfully resolved and committed.
func (s *Syncer) tryAgentResolve(ctx context.Context, r types.Repo, conflictPaths []string) bool {
	if s.sessionMgr == nil {
		return false
	}

	// Create or reuse a session for this repo
	_, err := s.sessionMgr.GetOrCreate(ctx, r.ID, r.Path)
	if err != nil {
		return false
	}

	// Determine conflict strategy
	strategy := r.ConflictStrategy
	if strategy == "" && s.cfg != nil {
		strategy = s.cfg.Agent.ConflictStrategy
	}

	// Resolve conflicts via agent
	result, err := s.sessionMgr.ResolveConflicts(ctx, r.ID, r.Path, conflictPaths, strategy)
	if err != nil || !result.Success {
		return false
	}

	// Verify no conflict markers remain in resolved files
	for _, file := range result.ResolvedFiles {
		content, err := s.gitOps.GetConflictedContent(ctx, r.Path, file)
		if err != nil {
			continue
		}
		if conflict.HasConflictMarkers(content) {
			return false // still has markers
		}
	}

	// Stage resolved files
	for _, file := range result.ResolvedFiles {
		gitAddCmd := exec.CommandContext(ctx, "git", "add", file)
		gitAddCmd.Dir = r.Path
		if err := gitAddCmd.Run(); err != nil {
			return false
		}
	}

	// Check staged changes
	if checkErr := s.gitOps.CheckStaged(ctx, r.Path); checkErr != nil {
		// Log but don't fail — whitespace issues are non-critical
		_ = checkErr
	}

	// Complete the merge with a commit
	commitCmd := exec.CommandContext(ctx, "git", "commit", "-m",
		fmt.Sprintf("Merge upstream (auto-resolved by %s)", s.sessionMgr.ProviderName()))
	commitCmd.Dir = r.Path
	if commitErr := commitCmd.Run(); commitErr != nil {
		return false
	}

	return true
}

// SyncAll syncs all managed repositories.
func (s *Syncer) SyncAll(ctx context.Context) []*Result {
	repos, err := s.store.List()
	if err != nil {
		return []*Result{{
			Status:       "error",
			ErrorMessage: fmt.Sprintf("list repos: %v", err),
		}}
	}

	var results []*Result
	for _, r := range repos {
		if r.Upstream == "" {
			continue // skip repos without upstream
		}
		result := s.SyncRepo(ctx, r)
		results = append(results, result)
	}

	return results
}

// runPostSyncCommands executes the repo's post-sync commands in order.
// It stops on the first failure. The sync status remains "synced" regardless.
func (s *Syncer) runPostSyncCommands(ctx context.Context, r types.Repo) []types.PostSyncResult {
	if len(r.PostSyncCommands) == 0 {
		return nil
	}

	var results []types.PostSyncResult
	for _, cmd := range r.PostSyncCommands {
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
	if status == types.RepoStatusSynced {
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
	if cfg.Proxy.Enabled && cfg.Proxy.URL != "" {
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
	case "synced":
		if result.CommitsPulled > 0 {
			s.notifier.NotifySyncSuccess(repoName, result.CommitsPulled)
		}
	case "conflict":
		s.notifier.NotifyConflict(repoName, len(result.ConflictFiles))
	case "error":
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
	// Don't record up_to_date to history - it's just a status check
	if result.Status == "up_to_date" {
		return 0
	}

	// Pre-set summary_status to "pending" if auto-summarization will be triggered
	summaryStatus := ""
	if s.summarizer != nil && s.cfg != nil && s.cfg.Sync.AutoSummary &&
		result.Status == "synced" && result.CommitsPulled > 0 {
		summaryStatus = "pending"
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
	case "synced":
		logger.Info("repo synced", "repo", result.RepoName, "commits_pulled", result.CommitsPulled)
		for _, ps := range result.PostSyncResults {
			if ps.Success {
				logger.Info("post-sync OK", "command", ps.Name)
			} else {
				logger.Error("post-sync failed", "command", ps.Name, "error", ps.Error)
			}
		}
	case "up_to_date":
		logger.Info("repo already up to date", "repo", result.RepoName)
	case "conflict":
		logger.Warn("repo conflicts", "repo", result.RepoName, "files", len(result.ConflictFiles))
	case "error":
		logger.Error("repo sync error", "repo", result.RepoName, "error", result.ErrorMessage)
	}
}

// finalizeResult records history, logs the result, and triggers summarization if needed.
func (s *Syncer) finalizeResult(result *Result) {
	historyID := s.recordHistory(result)
	s.logResult(result)

	// Trigger async summary if: auto_summary enabled, synced, commits pulled > 0, summarizer available
	if s.summarizer != nil && s.cfg != nil && s.cfg.Sync.AutoSummary &&
		result.Status == "synced" && result.CommitsPulled > 0 && historyID > 0 {
		go s.triggerSummarize(result, historyID)
	}
}

// triggerSummarize fetches commit list and enqueues a summarization task.
func (s *Syncer) triggerSummarize(result *Result, historyID int64) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	commits, err := s.gitOps.GetCommitLog(ctx, result.RepoPath, result.OldHEAD, result.UpstreamRef)
	if err != nil || len(commits) == 0 {
		return
	}

	// Map git.CommitInfo to summarizer.CommitInfo
	var summarizerCommits []summarizer.CommitInfo
	for _, c := range commits {
		summarizerCommits = append(summarizerCommits, summarizer.CommitInfo{
			Hash:    c.Hash,
			Message: c.Message,
		})
	}

	// Determine language from config (default zh)
	lang := s.cfg.Sync.SummaryLanguage
	if lang == "" {
		lang = "zh"
	}

	s.summarizer.Enqueue(summarizer.Task{
		HistoryID: historyID,
		RepoName:  result.RepoName,
		Commits:   summarizerCommits,
		Language:  lang,
	})
}



// shell returns the system shell for executing commands.
// Uses "cmd" on Windows, "sh" on all other platforms.
func shell() (string, string) {
	if runtime.GOOS == "windows" {
		return "cmd", "/c"
	}
	return "sh", "-c"
}
