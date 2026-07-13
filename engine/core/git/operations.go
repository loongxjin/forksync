package git

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	git "github.com/go-git/go-git/v5"
	gitConfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	gitTransport "github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/loongxjin/forksync/engine/core/logger"
	"github.com/loongxjin/forksync/engine/pkg/types"
)

// errStop is a sentinel error used to break out of iterator ForEach loops.
var errStop = errors.New("stop")

// OperationsProvider defines the interface for git operations.
// All sync and resolve logic depends on this interface, not the concrete type.
type OperationsProvider interface {
	// --- Repo-centric (takes types.Repo) ---
	Fetch(ctx context.Context, repo types.Repo) error
	Status(ctx context.Context, repo types.Repo) (*StatusResult, error)
	Merge(ctx context.Context, repo types.Repo) (*MergeResult, error)
	ResolveUpstreamRef(ctx context.Context, r types.Repo) string

	// --- Path-centric (takes repoPath string) ---
	IsGitRepo(ctx context.Context, path string) bool
	IsMergingState(ctx context.Context, repoPath string) (bool, []string, error)
	DetectConflicts(ctx context.Context, repoPath string) []string
	GetConflictedContent(ctx context.Context, repoPath, filePath string) (string, error)
	// FilterResolvedFiles checks the given files for conflict markers and
	// auto-stages those that are clean (no markers). Returns the subset that
	// still has markers (or could not be read/staged). It is the single
	// verify-and-stage step shared by the Conflict Resolver and the auto-sync
	// path.
	FilterResolvedFiles(ctx context.Context, repoPath string, files []string) []string
	Diff(ctx context.Context, repoPath string) ([]byte, error)
	DiffStaged(ctx context.Context, repoPath string) ([]byte, error)
	GetHEAD(ctx context.Context, repoPath string) (string, error)
	GetPreMergeHEAD(ctx context.Context, repoPath string) (string, error)
	StageFile(ctx context.Context, repoPath, file string) error
	StageAll(ctx context.Context, repoPath string) error
	Commit(ctx context.Context, repoPath, message string) error
	CommitNoEdit(ctx context.Context, repoPath string) error
	AbortMerge(ctx context.Context, repoPath string) error
	CheckStaged(ctx context.Context, repoPath string) error

	// --- Info queries ---
	GetRemotes(ctx context.Context, repoPath string) ([]RemoteInfo, error)
	FindRemoteURL(ctx context.Context, repoPath, remoteName string) string
	GetLocalBranches(ctx context.Context, repoPath string) ([]string, error)
	GetRemoteBranches(ctx context.Context, repoPath string, remoteName string) ([]string, error)
	GetRemoteBranchesFromURL(ctx context.Context, repoPath string, remoteURL string) ([]string, error)
	GetCommitLog(ctx context.Context, repoPath, oldHEAD, upstreamRef string) ([]CommitInfo, error)
}

// Operations provides git operations with go-git primary and CLI fallback.
var _ OperationsProvider = (*Operations)(nil)

type Operations struct {
	proxyURL string
	mu       sync.Mutex // protects os.Setenv calls for go-git proxy
}

// NewOperations creates a new Operations instance.
func NewOperations() *Operations {
	return &Operations{}
}

// NewOperationsWithProxy creates a new Operations instance with proxy support.
// The proxyURL is applied to both go-git (via environment) and CLI git commands.
//
// For socks5 proxies, InitTransport installs a custom go-git transport that
// routes through a socks5 dialer — without it go-git treats socks5 as an HTTP
// CONNECT proxy and every fetch fails with EOF.
func NewOperationsWithProxy(proxyURL string) *Operations {
	InitTransport(proxyURL)
	return &Operations{proxyURL: proxyURL}
}

// proxyEnv returns environment variables with proxy settings for CLI git commands.
// Sets HTTP_PROXY, HTTPS_PROXY (both cases) so all git traffic goes through the proxy.
func (o *Operations) proxyEnv() []string {
	env := os.Environ()
	if o.proxyURL == "" {
		return env
	}
	return append(env,
		"HTTP_PROXY="+o.proxyURL,
		"http_proxy="+o.proxyURL,
		"HTTPS_PROXY="+o.proxyURL,
		"https_proxy="+o.proxyURL,
	)
}

// runGit runs a git command in the repo directory and returns stdout.
func (o *Operations) runGit(ctx context.Context, repoPath string, args ...string) ([]byte, error) {
	start := time.Now()
	fullArgs := append([]string{"-C", repoPath}, args...)
	cmd := exec.CommandContext(ctx, "git", fullArgs...)
	cmd.Env = o.proxyEnv()
	output, err := cmd.Output()
	elapsed := time.Since(start)
	if err != nil {
		logger.Warn("git command failed",
			"args", strings.Join(fullArgs, " "),
			"elapsed", elapsed,
			"error", err,
		)
	} else {
		logger.Debug("git command",
			"args", strings.Join(fullArgs, " "),
			"elapsed", elapsed,
		)
	}
	return output, err
}

// runGitCombined runs a git command and returns combined stdout+stderr.
func (o *Operations) runGitCombined(ctx context.Context, repoPath string, args ...string) ([]byte, error) {
	start := time.Now()
	fullArgs := append([]string{"-C", repoPath}, args...)
	cmd := exec.CommandContext(ctx, "git", fullArgs...)
	cmd.Env = o.proxyEnv()
	output, err := cmd.CombinedOutput()
	elapsed := time.Since(start)
	if err != nil {
		logger.Warn("git command failed",
			"args", strings.Join(fullArgs, " "),
			"elapsed", elapsed,
			"output", string(output),
			"error", err,
		)
	} else {
		logger.Debug("git command",
			"args", strings.Join(fullArgs, " "),
			"elapsed", elapsed,
		)
	}
	return output, err
}

// IsGitRepo checks if the given path is a valid git repository.
func (o *Operations) IsGitRepo(_ context.Context, path string) bool {
	_, err := git.PlainOpen(path)
	return err == nil
}

// Fetch fetches from the specified remote for the given repo.
//
// The status check's whole purpose is to detect new upstream commits, so a real
// fetch runs whenever upstream may have changed. To avoid re-transferring the
// same large set of objects when upstream hasn't moved (e.g. a repo already
// 181 commits behind, refreshed repeatedly), an ls-remote preflight compares
// the upstream's current branch hash against the locally-tracked ref: if they
// match, the fetch is skipped. The preflight is best-effort — any failure
// falls through to a normal fetch. Event-driven refresh storms are also
// bounded: eventsStore coalesces bursts (300ms window) and the frontend
// dedupes in-flight status calls.
func (o *Operations) Fetch(ctx context.Context, repo types.Repo) error {
	if o.upstreamUnchanged(ctx, repo) {
		logger.Debug("git: skipping fetch, upstream HEAD matches local tracking ref",
			"repo", repo.Name)
		return nil
	}

	// Try go-git first
	err := o.fetchGoGit(ctx, repo)
	if err == nil {
		return nil
	}
	// Fallback to CLI
	logger.Info("git: go-git fetch failed, falling back to CLI",
		"repo", repo.Name,
		"path", repo.Path,
		"error", err,
	)
	return o.fetchCLI(ctx, repo)
}

// upstreamUnchanged returns true if an ls-remote of the upstream's branch hash
// equals the locally-tracked ref hash, meaning a full fetch would transfer no
// new objects. It is best-effort: on any error or unknown hash it returns
// false so a real fetch always runs.
func (o *Operations) upstreamUnchanged(ctx context.Context, repo types.Repo) bool {
	remoteHash := o.remoteHeadHash(ctx, repo)
	return shouldSkipFetch(remoteHash, o.localTrackingHash(repo))
}

// proxyOptions returns transport.ProxyOptions if a proxy is configured.
func (o *Operations) proxyOptions() gitTransport.ProxyOptions {
	if o.proxyURL == "" {
		return gitTransport.ProxyOptions{}
	}
	return gitTransport.ProxyOptions{URL: o.proxyURL}
}

func (o *Operations) fetchGoGit(ctx context.Context, repo types.Repo) error {
	r, err := git.PlainOpen(repo.Path)
	if err != nil {
		return fmt.Errorf("open repo: %w", err)
	}

	remoteName := repo.RemoteName()
	logger.Debug("git: fetchGoGit starting", "repo", repo.Name, "remote", remoteName)

	remote, err := r.Remote(remoteName)
	if err != nil {
		// If upstream remote doesn't exist, try to add it
		if repo.Upstream != "" {
			logger.Info("git: remote not found, creating upstream remote",
				"repo", repo.Name,
				"remote", remoteName,
				"upstream", repo.Upstream,
			)
			_, err = r.CreateRemote(&gitConfig.RemoteConfig{
				Name: "upstream",
				URLs: []string{repo.Upstream},
			})
			if err != nil {
				return fmt.Errorf("create upstream remote: %w", err)
			}
			remote, err = r.Remote("upstream")
			if err != nil {
				return fmt.Errorf("get upstream remote: %w", err)
			}
		} else {
			return fmt.Errorf("get remote %s: %w", remoteName, err)
		}
	}

	err = remote.FetchContext(ctx, &git.FetchOptions{
		ProxyOptions: o.proxyOptions(),
	})
	if err != nil && err != git.NoErrAlreadyUpToDate {
		return fmt.Errorf("fetch: %w", err)
	}
	logger.Debug("git: fetchGoGit completed",
		"repo", repo.Name,
		"already_up_to_date", err == git.NoErrAlreadyUpToDate,
	)
	return nil
}

func (o *Operations) fetchCLI(ctx context.Context, repo types.Repo) error {
	remoteName := repo.RemoteName()

	// Ensure the remote exists before fetching
	remotes, _ := o.getRemotesCLI(ctx, repo.Path)
	remoteExists := false
	for _, r := range remotes {
		if r.Name == remoteName {
			remoteExists = true
			break
		}
	}
	if !remoteExists && repo.Upstream != "" {
		logger.Info("git: CLI creating remote", "repo", repo.Name, "remote", remoteName, "url", repo.Upstream)
		if _, err := o.runGit(ctx, repo.Path, "remote", "add", remoteName, repo.Upstream); err != nil {
			return fmt.Errorf("git remote add %s: %w", remoteName, err)
		}
	}

	output, err := o.runGitCombined(ctx, repo.Path, "fetch", remoteName)
	if err != nil {
		return fmt.Errorf("git fetch %s: %s: %w", remoteName, string(output), err)
	}
	return nil
}

// StatusResult contains ahead/behind counts.
type StatusResult struct {
	AheadBy  int
	BehindBy int
	Branch   string
}

// Status returns the ahead/behind count against the upstream branch.
func (o *Operations) Status(ctx context.Context, repo types.Repo) (*StatusResult, error) {
	// Try go-git first
	result, err := o.statusGoGit(ctx, repo)
	if err == nil {
		return result, nil
	}
	// Fallback to CLI
	logger.Info("git: go-git status failed, falling back to CLI",
		"repo", repo.Name,
		"path", repo.Path,
		"error", err,
	)
	return o.statusCLI(ctx, repo)
}

func (o *Operations) statusGoGit(ctx context.Context, repo types.Repo) (*StatusResult, error) {
	r, err := git.PlainOpen(repo.Path)
	if err != nil {
		return nil, fmt.Errorf("open repo: %w", err)
	}

	// Use stored branch if available, otherwise detect from HEAD
	branch := repo.Branch
	if branch == "" {
		head, err := r.Head()
		if err != nil {
			return nil, fmt.Errorf("get HEAD: %w", err)
		}
		branch = head.Name().Short()
		logger.Debug("git: statusGoGit branch from HEAD", "repo", repo.Name, "branch", branch)
	} else {
		logger.Debug("git: statusGoGit using stored branch", "repo", repo.Name, "branch", branch)
	}

	// Resolve local ref for the stored branch
	localRef, err := r.Reference(plumbing.ReferenceName("refs/heads/"+branch), true)
	if err != nil {
		// Branch doesn't exist locally — fall back to HEAD
		head, headErr := r.Head()
		if headErr != nil {
			return nil, fmt.Errorf("get HEAD: %w", headErr)
		}
		branch = head.Name().Short()
		localRef = head
		logger.Debug("git: statusGoGit branch ref not found, fell back to HEAD",
			"repo", repo.Name,
			"branch", branch,
		)
	}

	remoteBranchName := repo.GetRemoteBranchForLocal(branch)
	remoteBranch := fmt.Sprintf("refs/remotes/%s/%s", repo.RemoteName(), remoteBranchName)

	remoteRef, err := r.Reference(plumbing.ReferenceName(remoteBranch), true)
	if err != nil {
		// Remote ref cannot be resolved — upstream not configured, never fetched,
		// or branch name mismatch. Surface this as an error so callers don't
		// mistake "cannot compare" for "up to date" (BehindBy == 0).
		logger.Debug("git: statusGoGit remote ref not found",
			"repo", repo.Name,
			"remote_branch", remoteBranch,
			"error", err,
		)
		return nil, fmt.Errorf("resolve remote ref %s: %w", remoteBranch, err)
	}

	ahead, behind, err := o.countDivergence(ctx, repo.Path, localRef.Hash().String(), remoteRef.Hash().String())
	if err != nil {
		return nil, err
	}

	logger.Debug("git: statusGoGit result",
		"repo", repo.Name,
		"branch", branch,
		"ahead", ahead,
		"behind", behind,
		"local_hash", localRef.Hash().String()[:8],
		"remote_hash", remoteRef.Hash().String()[:8],
	)

	return &StatusResult{AheadBy: ahead, BehindBy: behind, Branch: branch}, nil
}

func (o *Operations) countDivergence(ctx context.Context, repoPath, local, remote string) (ahead, behind int, err error) {
	output, err := o.runGit(ctx, repoPath,
		"rev-list", "--left-right", "--count", local+"..."+remote)
	if err != nil {
		return 0, 0, fmt.Errorf("count divergence: %w", err)
	}
	parts := strings.Fields(strings.TrimSpace(string(output)))
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("unexpected rev-list output: %q", output)
	}
	if _, err := fmt.Sscanf(parts[0], "%d", &ahead); err != nil {
		return 0, 0, fmt.Errorf("parse ahead count from %q: %w", parts[0], err)
	}
	if _, err := fmt.Sscanf(parts[1], "%d", &behind); err != nil {
		return 0, 0, fmt.Errorf("parse behind count from %q: %w", parts[1], err)
	}
	return ahead, behind, nil
}

func (o *Operations) statusCLI(ctx context.Context, repo types.Repo) (*StatusResult, error) {
	// Use stored branch if available, otherwise detect from HEAD
	branch := repo.Branch
	if branch == "" {
		output, err := o.runGit(ctx, repo.Path, "rev-parse", "--abbrev-ref", "HEAD")
		if err != nil {
			return nil, fmt.Errorf("get branch: %w", err)
		}
		branch = strings.TrimSpace(string(output))
		logger.Debug("git: statusCLI branch from HEAD", "repo", repo.Name, "branch", branch)
	}

	// Verify the local branch exists
	localRef := fmt.Sprintf("refs/heads/%s", branch)
	if _, err := o.runGit(ctx, repo.Path, "rev-parse", "--verify", localRef); err != nil {
		// Branch doesn't exist locally — fall back to HEAD
		output, headErr := o.runGit(ctx, repo.Path, "rev-parse", "--abbrev-ref", "HEAD")
		if headErr != nil {
			return nil, fmt.Errorf("get branch: %w", headErr)
		}
		branch = strings.TrimSpace(string(output))
		logger.Debug("git: statusCLI branch ref not found, fell back to HEAD",
			"repo", repo.Name,
			"branch", branch,
		)
	}

	remoteName := repo.RemoteName()
	remoteBranchName := repo.GetRemoteBranchForLocal(branch)
	upstreamRef := fmt.Sprintf("%s/%s", remoteName, remoteBranchName)

	// Get ahead count
	ahead, err := o.revListCount(ctx, repo.Path, upstreamRef, branch)
	if err != nil {
		// Upstream ref may not exist yet — surface as error so callers don't
		// mistake "cannot compare" for "up to date" (BehindBy == 0).
		logger.Debug("git: statusCLI ahead count failed (upstream ref may not exist)",
			"repo", repo.Name,
			"upstream_ref", upstreamRef,
			"error", err,
		)
		return nil, fmt.Errorf("resolve upstream ref %s: %w", upstreamRef, err)
	}

	// Get behind count
	behind, err := o.revListCount(ctx, repo.Path, branch, upstreamRef)
	if err != nil {
		logger.Debug("git: statusCLI behind count failed",
			"repo", repo.Name,
			"upstream_ref", upstreamRef,
			"error", err,
		)
		return nil, fmt.Errorf("resolve upstream ref %s: %w", upstreamRef, err)
	}

	logger.Debug("git: statusCLI result",
		"repo", repo.Name,
		"branch", branch,
		"upstream_ref", upstreamRef,
		"ahead", ahead,
		"behind", behind,
	)

	return &StatusResult{AheadBy: ahead, BehindBy: behind, Branch: branch}, nil
}

func (o *Operations) revListCount(ctx context.Context, dir, exclude, include string) (int, error) {
	output, err := o.runGit(ctx, dir, "rev-list", "--count", fmt.Sprintf("%s..%s", exclude, include))
	if err != nil {
		return 0, err
	}
	var count int
	if _, err := fmt.Sscanf(strings.TrimSpace(string(output)), "%d", &count); err != nil {
		return 0, fmt.Errorf("parse rev-list output %q: %w", output, err)
	}
	return count, nil
}

// MergeResult contains the result of a merge operation.
type MergeResult struct {
	HasConflicts bool
	Conflicts    []string
}

// Merge merges the upstream branch into the current branch.
func (o *Operations) Merge(ctx context.Context, repo types.Repo) (*MergeResult, error) {
	// For merge, CLI is more reliable for conflict detection
	return o.mergeCLI(ctx, repo)
}

func (o *Operations) mergeCLI(ctx context.Context, repo types.Repo) (*MergeResult, error) {
	remoteName := repo.RemoteName()

	branch := repo.Branch
	if branch == "" {
		// Get current branch
		output, err := o.runGit(ctx, repo.Path, "rev-parse", "--abbrev-ref", "HEAD")
		if err != nil {
			return nil, fmt.Errorf("get branch: %w", err)
		}
		branch = strings.TrimSpace(string(output))
	}

	logger.Debug("git: mergeCLI starting",
		"repo", repo.Name,
		"path", repo.Path,
		"branch", branch,
		"remote", remoteName,
	)

	// Ensure we are on the target branch before merging
	currentBranch, branchErr := o.GetCurrentBranch(ctx, repo.Path)
	if branchErr != nil {
		return nil, fmt.Errorf("get current branch: %w", branchErr)
	}
	if currentBranch != "" && currentBranch != branch {
		logger.Debug("git: mergeCLI checkout before merge",
			"repo", repo.Name,
			"current_branch", currentBranch,
			"target_branch", branch,
		)
		if _, err := o.runGitCombined(ctx, repo.Path, "checkout", branch); err != nil {
			return nil, fmt.Errorf("checkout %s: %w", branch, err)
		}
	}

	remoteBranchName := repo.GetRemoteBranchForLocal(branch)
	upstreamRef := fmt.Sprintf("%s/%s", remoteName, remoteBranchName)
	output, err := o.runGitCombined(ctx, repo.Path, "merge", upstreamRef)
	outputStr := string(output)

	if err != nil {
		// git merge exit code 1 = conflict (locale-independent).
		// Also check "冲突" for non-English locales.
		isConflict := strings.Contains(outputStr, "CONFLICT") || strings.Contains(outputStr, "冲突")
		if !isConflict {
			if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
				isConflict = true
			}
		}
		if isConflict {
			conflicts := o.DetectConflicts(ctx, repo.Path)
			logger.Warn("git: mergeCLI detected CONFLICT",
				"repo", repo.Name,
				"upstream_ref", upstreamRef,
				"raw_conflicts", conflicts,
			)
			// Filter out files that have been manually resolved but not yet staged.
			// Auto-stage them so they don't appear as unresolved conflicts.
			stillConflicted := o.FilterResolvedFiles(ctx, repo.Path, conflicts)
			logger.Info("git: mergeCLI after FilterResolvedFiles",
				"repo", repo.Name,
				"still_conflicted", stillConflicted,
				"auto_staged_count", len(conflicts)-len(stillConflicted),
			)
			if len(stillConflicted) == 0 {
				// All conflicts were auto-staged — no real conflicts remain.
				// MERGE_HEAD still exists, caller should handle this state.
				logger.Info("git: mergeCLI all conflicts auto-staged, MERGE_HEAD remains",
					"repo", repo.Name,
				)
				return &MergeResult{HasConflicts: false}, nil
			}
			return &MergeResult{HasConflicts: true, Conflicts: stillConflicted}, nil
		}
		return nil, fmt.Errorf("merge %s: %s: %w", upstreamRef, outputStr, err)
	}

	logger.Debug("git: mergeCLI completed successfully",
		"repo", repo.Name,
		"upstream_ref", upstreamRef,
	)
	return &MergeResult{HasConflicts: false}, nil
}

// DetectConflicts runs git diff to find files with unresolved conflicts.
func (o *Operations) DetectConflicts(ctx context.Context, repoPath string) []string {
	output, err := o.runGit(ctx, repoPath, "diff", "--name-only", "--diff-filter=U")
	if err != nil {
		logger.Debug("git: DetectConflicts error",
			"path", repoPath,
			"error", err,
		)
		return nil
	}

	files := strings.Split(strings.TrimSpace(string(output)), "\n")
	var result []string
	for _, f := range files {
		if f != "" {
			result = append(result, f)
		}
	}
	return result
}

// GetFileContent reads a file's content at a specific reference.
func (o *Operations) GetFileContent(ctx context.Context, repoPath, ref, filePath string) (string, error) {
	output, err := o.runGit(ctx, repoPath, "show", fmt.Sprintf("%s:%s", ref, filePath))
	if err != nil {
		return "", fmt.Errorf("get file content: %w", err)
	}
	return string(output), nil
}

// GetConflictedContent reads the current conflicted content of a file.
func (o *Operations) GetConflictedContent(_ context.Context, repoPath, filePath string) (string, error) {
	fullPath := filepath.Join(repoPath, filePath)
	content, err := os.ReadFile(fullPath)
	if err != nil {
		return "", fmt.Errorf("read conflicted file: %w", err)
	}
	return string(content), nil
}

// IsMergingState checks if the repository has an in-progress merge (unmerged files).
// It runs `git ls-files --unmerge` to check for unmerged entries.
// Files that have been manually resolved (no conflict markers) but not yet staged
// are automatically staged.
func (o *Operations) IsMergingState(ctx context.Context, repoPath string) (bool, []string, error) {
	// Check for MERGE_HEAD which indicates a merge is in progress
	mergeHead := filepath.Join(repoPath, ".git", "MERGE_HEAD")
	if _, err := os.Stat(mergeHead); os.IsNotExist(err) {
		return false, nil, nil
	}

	logger.Debug("git: MERGE_HEAD exists, checking unmerged files", "path", repoPath)

	// MERGE_HEAD exists — check for unmerged files
	unmergedFiles := o.DetectConflicts(ctx, repoPath)
	if len(unmergedFiles) == 0 {
		logger.Info("git: MERGE_HEAD exists but no unmerged files", "path", repoPath)
		return true, nil, nil
	}

	// Filter out files that have been manually resolved but not staged.
	// Auto-stage them so they are counted as resolved.
	stillConflicted := o.FilterResolvedFiles(ctx, repoPath, unmergedFiles)
	logger.Debug("git: IsMergingState result",
		"path", repoPath,
		"unmerged_files", unmergedFiles,
		"still_conflicted", stillConflicted,
	)
	return true, stillConflicted, nil
}

// FilterResolvedFiles checks unmerged files and auto-stages those without conflict markers.
// Returns the list of files that still have conflict markers.
// Files that have been resolved (no markers) are automatically staged via git add.
func (o *Operations) FilterResolvedFiles(ctx context.Context, repoPath string, unmergedFiles []string) []string {
	var stillConflicted []string
	for _, file := range unmergedFiles {
		content, err := o.GetConflictedContent(ctx, repoPath, file)
		if err != nil {
			// Can't read file — assume it's still conflicted
			logger.Warn("git: FilterResolvedFiles can't read file, assuming conflicted",
				"file", file,
				"error", err,
			)
			stillConflicted = append(stillConflicted, file)
			continue
		}
		if HasConflictMarkers(content) {
			logger.Debug("git: FilterResolvedFiles file still has conflict markers", "file", file)
			stillConflicted = append(stillConflicted, file)
		} else {
			// File is resolved but not staged — auto-stage it
			if stageErr := o.StageFile(ctx, repoPath, file); stageErr != nil {
				logger.Warn("git: FilterResolvedFiles failed to auto-stage resolved file",
					"file", file,
					"error", stageErr,
				)
				stillConflicted = append(stillConflicted, file)
			} else {
				logger.Debug("git: FilterResolvedFiles auto-staged resolved file", "file", file)
			}
		}
	}
	return stillConflicted
}

// AbortMerge aborts an in-progress merge.
func (o *Operations) AbortMerge(ctx context.Context, repoPath string) error {
	_, err := o.runGit(ctx, repoPath, "merge", "--abort")
	return err
}

// CheckStaged runs `git diff --check` on staged files to detect whitespace
// and other issues. Returns nil if clean, or an error with details.
func (o *Operations) CheckStaged(ctx context.Context, repoPath string) error {
	output, err := o.runGitCombined(ctx, repoPath, "diff", "--check", "--cached")
	if err != nil {
		return fmt.Errorf("whitespace/style issues detected:\n%s", string(output))
	}
	return nil
}

// RemoteInfo holds information about a git remote.
type RemoteInfo struct {
	Name string
	URL  string
}

// GetRemotes returns the remotes configured for a repo.
func (o *Operations) GetRemotes(ctx context.Context, repoPath string) ([]RemoteInfo, error) {
	r, err := git.PlainOpen(repoPath)
	if err != nil {
		return o.getRemotesCLI(ctx, repoPath)
	}

	remotes, err := r.Remotes()
	if err != nil {
		return nil, err
	}

	var result []RemoteInfo
	for _, remote := range remotes {
		urls := remote.Config().URLs
		if len(urls) > 0 {
			result = append(result, RemoteInfo{
				Name: remote.Config().Name,
				URL:  urls[0],
			})
		}
	}
	return result, nil
}

func (o *Operations) getRemotesCLI(ctx context.Context, repoPath string) ([]RemoteInfo, error) {
	output, err := o.runGit(ctx, repoPath, "remote", "-v")
	if err != nil {
		return nil, err
	}

	seen := make(map[string]struct{})
	var result []RemoteInfo
	for _, line := range strings.Split(string(output), "\n") {
		parts := strings.Fields(line)
		if len(parts) >= 2 && strings.HasSuffix(parts[len(parts)-1], "(fetch)") {
			name := parts[0]
			remoteURL := parts[1]
			if _, ok := seen[name]; !ok {
				seen[name] = struct{}{}
				result = append(result, RemoteInfo{Name: name, URL: remoteURL})
			}
		}
	}
	return result, nil
}

// FindRemoteURL returns the URL of the specified remote, or empty string if not found.
func (o *Operations) FindRemoteURL(ctx context.Context, repoPath, remoteName string) string {
	remotes, err := o.GetRemotes(ctx, repoPath)
	if err != nil {
		return ""
	}
	for _, r := range remotes {
		if r.Name == remoteName {
			return r.URL
		}
	}
	return ""
}

// GetLocalBranches returns a list of local branch names
func (o *Operations) GetLocalBranches(ctx context.Context, repoPath string) ([]string, error) {
	output, err := o.runGit(ctx, repoPath, "branch", "--format=%(refname:short)")
	if err != nil {
		return nil, fmt.Errorf("get local branches: %w", err)
	}

	var branches []string
	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			branches = append(branches, line)
		}
	}
	return branches, nil
}

// BranchInfo contains branch information with commit time
type BranchInfo struct {
	Name       string
	CommitTime time.Time
}

// GetRemoteBranches returns a list of remote branch names, sorted by most recent commit first
func (o *Operations) GetRemoteBranches(ctx context.Context, repoPath string, remoteName string) ([]string, error) {
	// Use for-each-ref to get remote branches with their latest commit time
	// Format: %(refname:short)|%(committerdate:iso8601)
	output, err := o.runGit(ctx, repoPath, "for-each-ref",
		"--format=%(refname:short)|%(committerdate:iso8601)",
		"--sort=-committerdate",
		fmt.Sprintf("refs/remotes/%s/", remoteName))
	if err != nil {
		// Fallback to ls-remote if for-each-ref fails (e.g., remote not fetched)
		return o.getRemoteBranchesViaLsRemote(ctx, repoPath, remoteName)
	}

	var branches []string
	prefix := remoteName + "/"
	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Split(line, "|")
		if len(parts) >= 1 {
			branchName := strings.TrimPrefix(parts[0], prefix)
			if branchName != "" && branchName != "HEAD" {
				branches = append(branches, branchName)
			}
		}
	}
	return branches, nil
}

// GetRemoteBranchesFromURL fetches branch names from a remote URL via ls-remote.
// This bypasses local tracking branches and fetches directly from the remote.
func (o *Operations) GetRemoteBranchesFromURL(ctx context.Context, repoPath string, remoteURL string) ([]string, error) {
	return o.getRemoteBranchesViaLsRemote(ctx, repoPath, remoteURL)
}

// getRemoteBranchesViaLsRemote fetches remote branches via ls-remote as a fallback
func (o *Operations) getRemoteBranchesViaLsRemote(ctx context.Context, repoPath string, remoteName string) ([]string, error) {
	output, err := o.runGit(ctx, repoPath, "ls-remote", "--heads", remoteName)
	if err != nil {
		return nil, fmt.Errorf("get remote branches: %w", err)
	}

	var branches []string
	prefix := "refs/heads/"
	for _, line := range strings.Split(string(output), "\n") {
		parts := strings.Split(line, "\t")
		if len(parts) == 2 && strings.HasPrefix(parts[1], prefix) {
			branchName := strings.TrimPrefix(parts[1], prefix)
			branches = append(branches, branchName)
		}
	}
	return branches, nil
}

// CommitInfo represents a single git commit.
type CommitInfo struct {
	Hash    string
	Message string
}

// GetCommitLog returns commits between oldHEAD and upstreamRef (oldHEAD..upstreamRef).
func (o *Operations) GetCommitLog(ctx context.Context, repoPath, oldHEAD, upstreamRef string) ([]CommitInfo, error) {
	if oldHEAD == "" || upstreamRef == "" {
		return nil, nil
	}
	output, err := o.runGit(ctx, repoPath, "log",
		fmt.Sprintf("%s..%s", oldHEAD, upstreamRef),
		"--pretty=format:%h%x09%s")
	if err != nil {
		return nil, err
	}

	var commits []CommitInfo
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) == 2 {
			commits = append(commits, CommitInfo{
				Hash:    parts[0],
				Message: parts[1],
			})
		}
	}
	return commits, nil
}

// GetCurrentBranch returns the current branch name of the repo.
func (o *Operations) GetCurrentBranch(ctx context.Context, repoPath string) (string, error) {
	output, err := o.runGit(ctx, repoPath, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

// GetHEAD returns the current HEAD commit hash.
func (o *Operations) GetHEAD(ctx context.Context, repoPath string) (string, error) {
	output, err := o.runGit(ctx, repoPath, "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

// StageFile stages a single file.
func (o *Operations) StageFile(ctx context.Context, repoPath, file string) error {
	_, err := o.runGitCombined(ctx, repoPath, "add", file)
	return err
}

// StageAll stages all changes.
func (o *Operations) StageAll(ctx context.Context, repoPath string) error {
	_, err := o.runGitCombined(ctx, repoPath, "add", "-u")
	return err
}

// Commit creates a new commit with the given message, skipping pre-commit hooks.
func (o *Operations) Commit(ctx context.Context, repoPath, message string) error {
	logger.Debug("git: Commit", "path", repoPath, "message", message)
	_, err := o.runGitCombined(ctx, repoPath, "commit", "-m", message, "--no-verify")
	return err
}

// CommitWithVerify creates a new commit with the given message, without --no-verify.
func (o *Operations) CommitWithVerify(ctx context.Context, repoPath, message string) error {
	logger.Debug("git: CommitWithVerify", "path", repoPath, "message", message)
	_, err := o.runGitCombined(ctx, repoPath, "commit", "-m", message)
	return err
}

// CommitNoEdit creates a commit using the default merge message, skipping pre-commit hooks.
func (o *Operations) CommitNoEdit(ctx context.Context, repoPath string) error {
	logger.Debug("git: CommitNoEdit", "path", repoPath)
	_, err := o.runGitCombined(ctx, repoPath, "commit", "--no-edit", "--no-verify")
	return err
}

// CommitNoEditWithVerify creates a commit using the default merge message, without --no-verify.
func (o *Operations) CommitNoEditWithVerify(ctx context.Context, repoPath string) error {
	logger.Debug("git: CommitNoEditWithVerify", "path", repoPath)
	_, err := o.runGitCombined(ctx, repoPath, "commit", "--no-edit")
	return err
}

// CheckoutFile restores a file to the HEAD state.
func (o *Operations) CheckoutFile(ctx context.Context, repoPath, file string) error {
	_, err := o.runGitCombined(ctx, repoPath, "checkout", "--", file)
	return err
}

// Diff returns all changes relative to HEAD (staged + unstaged combined).
// The "resolved diff" view is shown after an agent resolves conflicts and
// typically `git add`s the result; a plain `git diff` (unstaged only) would
// then show nothing. `git diff HEAD` captures both working-tree and index.
func (o *Operations) Diff(ctx context.Context, repoPath string) ([]byte, error) {
	return o.runGit(ctx, repoPath, "diff", "HEAD")
}

// DiffStaged returns the staged diff output for the repository.
func (o *Operations) DiffStaged(ctx context.Context, repoPath string) ([]byte, error) {
	return o.runGit(ctx, repoPath, "diff", "--staged")
}

// GetPreMergeHEAD returns the first parent of the most recent merge commit,
// which corresponds to the HEAD before the merge. Used to recover OldHEAD
// when the workflow was created before the OldHEAD field was added.
func (o *Operations) GetPreMergeHEAD(ctx context.Context, repoPath string) (string, error) {
	output, err := o.runGit(ctx, repoPath, "log", "--merges", "-1", "--format=%P")
	if err != nil {
		return "", err
	}
	parents := strings.Fields(strings.TrimSpace(string(output)))
	if len(parents) == 0 {
		return "", fmt.Errorf("no merge commit found")
	}
	return parents[0], nil
}

// ResolveUpstreamRef returns the upstream remote/branch ref for a repo (e.g. "upstream/main").
func (o *Operations) ResolveUpstreamRef(ctx context.Context, r types.Repo) string {
	remoteName := r.RemoteName()
	branch := r.Branch
	if branch == "" {
		if b, err := o.GetCurrentBranch(ctx, r.Path); err == nil {
			branch = b
		}
	}
	if branch == "" {
		branch = types.DefaultBranch
	}
	return fmt.Sprintf("%s/%s", remoteName, r.GetRemoteBranchForLocal(branch))
}

// HasConflictMarkers checks if content contains unresolved conflict markers.
func HasConflictMarkers(content string) bool {
	return strings.Contains(content, "<<<<<<< ") &&
		strings.Contains(content, "=======") &&
		strings.Contains(content, ">>>>>>> ")
}
