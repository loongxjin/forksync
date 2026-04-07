package sync

import (
	"context"
	"fmt"
	"os/exec"
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
	"github.com/loongxjin/forksync/engine/pkg/types"
)

const defaultTimeout = 5 * time.Minute

// Syncer handles repository synchronization.
type Syncer struct {
	gitOps        *git.Operations
	store         repo.Store
	cfg           *config.Config
	notifier      *notify.Notifier
	sessionMgr    *session.Manager
	historyStore  *history.Store
	logger        *logger.Logger
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

// SetLogger sets the file logger for sync operations.
func (s *Syncer) SetLogger(l *logger.Logger) {
	s.logger = l
}

// Result contains the result of syncing a single repo.
type Result struct {
	RepoID        string
	RepoName      string
	Status        string // types.RepoStatus values: synced, conflict, up_to_date, error
	CommitsPulled int
	ConflictFiles []string
	ErrorMessage  string
	AgentUsed     string // agent name if auto-resolve was attempted
	ConflictsFound int   // number of conflicts detected
	AutoResolved   int   // number of files auto-resolved by agent
	PendingConfirm []string // files pending user confirmation
}

// ToSyncResult converts Result to types.SyncResult for JSON output.
func (r *Result) ToSyncResult() types.SyncResult {
	return types.SyncResult{
		RepoID:         r.RepoID,
		RepoName:       r.RepoName,
		Status:         types.RepoStatus(r.Status),
		CommitsPulled:  r.CommitsPulled,
		ConflictFiles:  r.ConflictFiles,
		ErrorMessage:   r.ErrorMessage,
		AgentUsed:      r.AgentUsed,
		ConflictsFound: r.ConflictsFound,
		AutoResolved:   r.AutoResolved,
		PendingConfirm: r.PendingConfirm,
	}
}

// SyncRepo syncs a single repository.
func (s *Syncer) SyncRepo(ctx context.Context, r types.Repo) *Result {
	result := &Result{
		RepoID:   r.ID,
		RepoName: r.Name,
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

	// Set timeout
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	// Update status to syncing
	r.Status = types.RepoStatusSyncing
	_ = s.store.Update(r)

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
	result, err := s.sessionMgr.ResolveConflicts(ctx, r.ID, conflictPaths, strategy)
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
	_ = s.store.Update(r)
}

// NewSyncerFromConfig creates a Syncer using config defaults.
func NewSyncerFromConfig(cfg *config.Config, store repo.Store) *Syncer {
	return &Syncer{
		gitOps: git.NewOperations(),
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
func (s *Syncer) recordHistory(result *Result) {
	if s.historyStore == nil {
		return
	}
	_ = s.historyStore.Record(history.Record{
		RepoID:         result.RepoID,
		RepoName:       result.RepoName,
		Status:         result.Status,
		CommitsPulled:  result.CommitsPulled,
		ConflictFiles:  result.ConflictFiles,
		AgentUsed:      result.AgentUsed,
		ConflictsFound: result.ConflictsFound,
		AutoResolved:   result.AutoResolved,
		ErrorMessage:   result.ErrorMessage,
		CreatedAt:      time.Now(),
	})
}

// logResult writes the sync result to the log file.
func (s *Syncer) logResult(result *Result) {
	if s.logger == nil {
		return
	}
	switch result.Status {
	case "synced":
		s.logger.Info("%s: synced (%d commits pulled)", result.RepoName, result.CommitsPulled)
	case "up_to_date":
		s.logger.Info("%s: already up to date", result.RepoName)
	case "conflict":
		s.logger.Warn("%s: conflicts in %d files", result.RepoName, len(result.ConflictFiles))
	case "error":
		s.logger.Error("%s: %s", result.RepoName, result.ErrorMessage)
	}
}

// finalizeResult records history and logs the result.
func (s *Syncer) finalizeResult(result *Result) {
	s.recordHistory(result)
	s.logResult(result)
}
