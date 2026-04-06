package sync

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/loongxjin/forksync/engine/internal/ai"
	"github.com/loongxjin/forksync/engine/internal/conflict"
	"github.com/loongxjin/forksync/engine/internal/config"
	"github.com/loongxjin/forksync/engine/internal/git"
	"github.com/loongxjin/forksync/engine/internal/notify"
	"github.com/loongxjin/forksync/engine/internal/repo"
	"github.com/loongxjin/forksync/engine/pkg/types"
)

const defaultTimeout = 5 * time.Minute

// Syncer handles repository synchronization.
type Syncer struct {
	gitOps   *git.Operations
	store    repo.Store
	cfg      *config.Config
	notifier *notify.Notifier
	mu       sync.Mutex
	active   map[string]bool // tracks repos currently syncing
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

// Result contains the result of syncing a single repo.
type Result struct {
	RepoID        string
	RepoName      string
	Status        string // types.RepoStatus values: synced, conflict, up_to_date, error
	CommitsPulled int
	ConflictFiles []string
	ErrorMessage  string
}

// ToSyncResult converts Result to types.SyncResult for JSON output.
func (r *Result) ToSyncResult() types.SyncResult {
	return types.SyncResult{
		RepoID:        r.RepoID,
		RepoName:      r.RepoName,
		Status:        types.RepoStatus(r.Status),
		CommitsPulled: r.CommitsPulled,
		ConflictFiles: r.ConflictFiles,
		ErrorMessage:  r.ErrorMessage,
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
		return result
	}

	// Step 2: Check ahead/behind
	statusResult, err := s.gitOps.Status(ctx, r)
	if err != nil {
		result.Status = "error"
		result.ErrorMessage = fmt.Sprintf("status check failed: %v", err)
		s.updateRepoStatus(r.ID, types.RepoStatusError, result.ErrorMessage)
		return result
	}

	if statusResult.BehindBy == 0 {
		result.Status = string(types.RepoStatusUpToDate)
		result.CommitsPulled = 0
		s.updateRepoStatus(r.ID, types.RepoStatusSynced, "")
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
		return result
	}

	if mergeResult.HasConflicts {
		// Step 4: Try AI auto-resolve if conflictStrategy is "ai_resolve"
		if r.ConflictStrategy == "ai_resolve" && s.cfg != nil {
			resolved := s.tryAIResolve(ctx, r, mergeResult.Conflicts)
			if resolved {
				result.Status = "synced"
				s.updateRepoStatus(r.ID, types.RepoStatusSynced, "")
				return result
			}
			// AI resolve failed, fall through to conflict status
		}

		result.Status = "conflict"
		result.ConflictFiles = mergeResult.Conflicts
		s.updateRepoStatus(r.ID, types.RepoStatusConflict, "")
		s.notifyResult(r.Name, result)
		return result
	}

	// Success
	result.Status = "synced"
	s.updateRepoStatus(r.ID, types.RepoStatusSynced, "")
	s.notifyResult(r.Name, result)
	return result
}

// tryAIResolve attempts to resolve conflicts using AI. Returns true if all conflicts were resolved.
func (s *Syncer) tryAIResolve(ctx context.Context, r types.Repo, conflictPaths []string) bool {
	// Get AI provider config
	providerName := "openai"
	if s.cfg.AI.DefaultProvider != "" {
		providerName = s.cfg.AI.DefaultProvider
	}

	providerCfg, ok := s.cfg.AI.Providers[providerName]
	if !ok || providerCfg.APIKey == "" {
		return false
	}

	provider := ai.NewOpenAIAdapter(providerCfg.APIKey, providerCfg.Model, providerCfg.BaseURL)
	detector := conflict.NewDetector()

	conflictFiles, err := detector.GetConflictFiles(ctx, r.Path, conflictPaths)
	if err != nil {
		return false
	}

	allSucceeded := true
	for _, cf := range conflictFiles {
		req := ai.ConflictRequest{
			FilePath:        cf.Path,
			ConflictContent: cf.OursContent,
			Language:        detectLanguageFromPath(cf.Path),
		}

		resolution, err := provider.ResolveConflicts(ctx, req)
		if err != nil {
			allSucceeded = false
			continue
		}

		// Validate: reject if conflict markers remain
		if conflict.HasConflictMarkers(resolution.MergedContent) {
			allSucceeded = false
			continue
		}

		// Write resolved content
		fullPath := filepath.Join(r.Path, cf.Path)
		if writeErr := os.WriteFile(fullPath, []byte(resolution.MergedContent), 0644); writeErr != nil {
			allSucceeded = false
			continue
		}

		// Stage the file
		gitAddCmd := exec.CommandContext(ctx, "git", "add", cf.Path)
		gitAddCmd.Dir = r.Path
		if addErr := gitAddCmd.Run(); addErr != nil {
			allSucceeded = false
			continue
		}

		// Validate staged changes with git diff --check
		if checkErr := s.gitOps.CheckStaged(ctx, r.Path); checkErr != nil {
			// Log but don't fail — whitespace issues are non-critical
			_ = checkErr
		}
	}

	if !allSucceeded {
		return false
	}

	// Complete the merge with a commit
	commitCmd := exec.CommandContext(ctx, "git", "commit", "-m", "Merge upstream (AI-resolved conflicts)")
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

// detectLanguageFromPath detects programming language from file extension.
func detectLanguageFromPath(path string) string {
	ext := filepath.Ext(path)
	switch ext {
	case ".go":
		return "go"
	case ".ts", ".tsx":
		return "typescript"
	case ".js", ".jsx":
		return "javascript"
	case ".py":
		return "python"
	case ".rs":
		return "rust"
	case ".java":
		return "java"
	case ".rb":
		return "ruby"
	case ".c", ".h":
		return "c"
	case ".cpp", ".cc", ".cxx":
		return "cpp"
	default:
		return ""
	}
}
