package git

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/loongxjin/forksync/engine/pkg/types"
	git "github.com/go-git/go-git/v5"
	gitConfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// errStop is a sentinel error used to break out of iterator ForEach loops.
var errStop = errors.New("stop")

// Operations provides git operations with go-git primary and CLI fallback.
type Operations struct {
	proxyURL string
}

// NewOperations creates a new Operations instance.
func NewOperations() *Operations {
	return &Operations{}
}

// NewOperationsWithProxy creates a new Operations instance with proxy support.
// The proxyURL is applied to both go-git (via environment) and CLI git commands.
func NewOperationsWithProxy(proxyURL string) *Operations {
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
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", repoPath}, args...)...)
	cmd.Env = o.proxyEnv()
	return cmd.Output()
}

// runGitCombined runs a git command and returns combined stdout+stderr.
func (o *Operations) runGitCombined(ctx context.Context, repoPath string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", repoPath}, args...)...)
	cmd.Env = o.proxyEnv()
	return cmd.CombinedOutput()
}

// IsGitRepo checks if the given path is a valid git repository.
func (o *Operations) IsGitRepo(_ context.Context, path string) bool {
	_, err := git.PlainOpen(path)
	return err == nil
}

// Fetch fetches from the specified remote for the given repo.
func (o *Operations) Fetch(ctx context.Context, repo types.Repo) error {
	// Set proxy env for go-git (it uses Go's http client which respects proxy env)
	if o.proxyURL != "" {
		os.Setenv("HTTP_PROXY", o.proxyURL)
		os.Setenv("HTTPS_PROXY", o.proxyURL)
		defer func() {
			os.Unsetenv("HTTP_PROXY")
			os.Unsetenv("HTTPS_PROXY")
		}()
	}

	// Try go-git first
	err := o.fetchGoGit(ctx, repo)
	if err == nil {
		return nil
	}
	// Fallback to CLI
	return o.fetchCLI(ctx, repo)
}

func (o *Operations) fetchGoGit(ctx context.Context, repo types.Repo) error {
	r, err := git.PlainOpen(repo.Path)
	if err != nil {
		return fmt.Errorf("open repo: %w", err)
	}

	remoteName := "upstream"
	if repo.Upstream == "" {
		remoteName = "origin"
	}

	remote, err := r.Remote(remoteName)
	if err != nil {
		// If upstream remote doesn't exist, try to add it
		if repo.Upstream != "" {
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

	err = remote.FetchContext(ctx, &git.FetchOptions{})
	if err != nil && err != git.NoErrAlreadyUpToDate {
		return fmt.Errorf("fetch: %w", err)
	}
	return nil
}

func (o *Operations) fetchCLI(ctx context.Context, repo types.Repo) error {
	remoteName := "upstream"
	if repo.Upstream == "" {
		remoteName = "origin"
	}

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
	// Use CLI directly for reliability and performance
	return o.statusCLI(ctx, repo)
}

func (o *Operations) statusGoGit(_ context.Context, repo types.Repo) (*StatusResult, error) {
	r, err := git.PlainOpen(repo.Path)
	if err != nil {
		return nil, fmt.Errorf("open repo: %w", err)
	}

	head, err := r.Head()
	if err != nil {
		return nil, fmt.Errorf("get HEAD: %w", err)
	}

	branch := head.Name().Short()
	remoteBranchName := repo.GetRemoteBranchForLocal(branch)
	remoteBranch := fmt.Sprintf("refs/remotes/upstream/%s", remoteBranchName)
	if repo.Upstream == "" {
		remoteBranch = fmt.Sprintf("refs/remotes/origin/%s", remoteBranchName)
	}

	remoteRef, err := r.Reference(plumbing.ReferenceName(remoteBranch), true)
	if err != nil {
		return &StatusResult{AheadBy: 0, BehindBy: 0, Branch: branch}, nil
	}

	ahead, behind, err := o.countDivergence(r, head.Hash(), remoteRef.Hash())
	if err != nil {
		return nil, err
	}

	return &StatusResult{AheadBy: ahead, BehindBy: behind, Branch: branch}, nil
}

func (o *Operations) countDivergence(r *git.Repository, local, remote plumbing.Hash) (ahead, behind int, err error) {
	// Build set of remote ancestors with bounded iteration
	remoteAncestors := make(map[plumbing.Hash]bool)
	remoteIter, err := r.Log(&git.LogOptions{From: remote})
	if err != nil {
		return 0, 0, fmt.Errorf("remote log: %w", err)
	}
	defer remoteIter.Close()
	_ = remoteIter.ForEach(func(c *object.Commit) error {
		remoteAncestors[c.Hash] = true
		return nil
	})

	// Count ahead: commits reachable from local but not remote
	localIter, err := r.Log(&git.LogOptions{From: local})
	if err != nil {
		return 0, 0, fmt.Errorf("local log: %w", err)
	}
	defer localIter.Close()
	err = localIter.ForEach(func(c *object.Commit) error {
		if c.Hash == remote || remoteAncestors[c.Hash] {
			return errStop
		}
		ahead++
		return nil
	})
	if err != nil && !errors.Is(err, errStop) {
		return 0, 0, fmt.Errorf("count ahead: %w", err)
	}

	// Build set of local ancestors
	localAncestors := make(map[plumbing.Hash]bool)
	localIter2, err := r.Log(&git.LogOptions{From: local})
	if err != nil {
		return 0, 0, fmt.Errorf("local log 2: %w", err)
	}
	defer localIter2.Close()
	_ = localIter2.ForEach(func(c *object.Commit) error {
		localAncestors[c.Hash] = true
		return nil
	})

	// Count behind: commits reachable from remote but not local
	remoteIter2, err := r.Log(&git.LogOptions{From: remote})
	if err != nil {
		return 0, 0, fmt.Errorf("remote log 2: %w", err)
	}
	defer remoteIter2.Close()
	err = remoteIter2.ForEach(func(c *object.Commit) error {
		if c.Hash == local || localAncestors[c.Hash] {
			return errStop
		}
		behind++
		return nil
	})
	if err != nil && !errors.Is(err, errStop) {
		return 0, 0, fmt.Errorf("count behind: %w", err)
	}

	return ahead, behind, nil
}

func (o *Operations) statusCLI(ctx context.Context, repo types.Repo) (*StatusResult, error) {
	// Get current branch
	output, err := o.runGit(ctx, repo.Path, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return nil, fmt.Errorf("get branch: %w", err)
	}
	branch := strings.TrimSpace(string(output))

	remoteName := "upstream"
	if repo.Upstream == "" {
		remoteName = "origin"
	}
	remoteBranchName := repo.GetRemoteBranchForLocal(branch)
	upstreamRef := fmt.Sprintf("%s/%s", remoteName, remoteBranchName)

	// Get ahead count
	ahead, err := o.revListCount(ctx, repo.Path, upstreamRef, "HEAD")
	if err != nil {
		// Upstream ref may not exist yet
		ahead = 0
	}

	// Get behind count
	behind, err := o.revListCount(ctx, repo.Path, "HEAD", upstreamRef)
	if err != nil {
		behind = 0
	}

	return &StatusResult{AheadBy: ahead, BehindBy: behind, Branch: branch}, nil
}

func (o *Operations) revListCount(ctx context.Context, dir, exclude, include string) (int, error) {
	output, err := o.runGit(ctx, dir, "rev-list", "--count", fmt.Sprintf("%s..%s", exclude, include))
	if err != nil {
		return 0, err
	}
	var count int
	fmt.Sscanf(strings.TrimSpace(string(output)), "%d", &count)
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
	remoteName := "upstream"
	if repo.Upstream == "" {
		remoteName = "origin"
	}

	branch := repo.Branch
	if branch == "" {
		// Get current branch
		output, err := o.runGit(ctx, repo.Path, "rev-parse", "--abbrev-ref", "HEAD")
		if err != nil {
			return nil, fmt.Errorf("get branch: %w", err)
		}
		branch = strings.TrimSpace(string(output))
	}

	remoteBranchName := repo.GetRemoteBranchForLocal(branch)
	upstreamRef := fmt.Sprintf("%s/%s", remoteName, remoteBranchName)
	output, err := o.runGitCombined(ctx, repo.Path, "merge", upstreamRef)
	outputStr := string(output)

	if err != nil {
		if strings.Contains(outputStr, "CONFLICT") {
			conflicts := o.detectConflicts(ctx, repo.Path)
			return &MergeResult{HasConflicts: true, Conflicts: conflicts}, nil
		}
		return nil, fmt.Errorf("merge %s: %s: %w", upstreamRef, outputStr, err)
	}

	return &MergeResult{HasConflicts: false}, nil
}

func (o *Operations) detectConflicts(ctx context.Context, repoPath string) []string {
	output, err := o.runGit(ctx, repoPath, "diff", "--name-only", "--diff-filter=U")
	if err != nil {
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
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return "", fmt.Errorf("read conflicted file: %w", err)
	}
	return string(data), nil
}

// IsMergingState checks if the repository has an in-progress merge (unmerged files).
// It runs `git ls-files --unmerge` to check for unmerged entries.
func (o *Operations) IsMergingState(ctx context.Context, repoPath string) (bool, []string, error) {
	// Check for MERGE_HEAD which indicates a merge is in progress
	mergeHead := filepath.Join(repoPath, ".git", "MERGE_HEAD")
	if _, err := os.Stat(mergeHead); os.IsNotExist(err) {
		return false, nil, nil
	}

	// MERGE_HEAD exists — check for unmerged files
	unmergedFiles := o.detectConflicts(ctx, repoPath)
	return true, unmergedFiles, nil
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

	seen := make(map[string]bool)
	var result []RemoteInfo
	for _, line := range strings.Split(string(output), "\n") {
		parts := strings.Fields(line)
		if len(parts) >= 2 && strings.HasSuffix(parts[len(parts)-1], "(fetch)") {
			name := parts[0]
			remoteURL := parts[1]
			if !seen[name] {
				seen[name] = true
				result = append(result, RemoteInfo{Name: name, URL: remoteURL})
			}
		}
	}
	return result, nil
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
