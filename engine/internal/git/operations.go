package git

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/loongxjin/forksync/engine/pkg/types"
	git "github.com/go-git/go-git/v5"
	gitConfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// errStop is a sentinel error used to break out of iterator ForEach loops.
var errStop = errors.New("stop")

// Operations provides git operations with go-git primary and CLI fallback.
type Operations struct{}

// NewOperations creates a new Operations instance.
func NewOperations() *Operations {
	return &Operations{}
}

// IsGitRepo checks if the given path is a valid git repository.
func (o *Operations) IsGitRepo(_ context.Context, path string) bool {
	_, err := git.PlainOpen(path)
	return err == nil
}

// Fetch fetches from the specified remote for the given repo.
func (o *Operations) Fetch(ctx context.Context, repo types.Repo) error {
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
		cmd := exec.CommandContext(ctx, "git", "remote", "add", remoteName, repo.Upstream)
		cmd.Dir = repo.Path
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("git remote add %s: %w", remoteName, err)
		}
	}

	cmd := exec.CommandContext(ctx, "git", "fetch", remoteName)
	cmd.Dir = repo.Path
	output, err := cmd.CombinedOutput()
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
	remoteBranch := fmt.Sprintf("refs/remotes/upstream/%s", branch)
	if repo.Upstream == "" {
		remoteBranch = fmt.Sprintf("refs/remotes/origin/%s", branch)
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
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = repo.Path
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("get branch: %w", err)
	}
	branch := strings.TrimSpace(string(output))

	remoteName := "upstream"
	if repo.Upstream == "" {
		remoteName = "origin"
	}
	upstreamRef := fmt.Sprintf("%s/%s", remoteName, branch)

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
	cmd := exec.CommandContext(ctx, "git", "rev-list", "--count", fmt.Sprintf("%s..%s", exclude, include))
	cmd.Dir = dir
	output, err := cmd.Output()
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
		cmd := exec.CommandContext(ctx, "git", "rev-parse", "--abbrev-ref", "HEAD")
		cmd.Dir = repo.Path
		output, err := cmd.Output()
		if err != nil {
			return nil, fmt.Errorf("get branch: %w", err)
		}
		branch = strings.TrimSpace(string(output))
	}

	upstreamRef := fmt.Sprintf("%s/%s", remoteName, branch)
	cmd := exec.CommandContext(ctx, "git", "merge", upstreamRef)
	cmd.Dir = repo.Path
	output, err := cmd.CombinedOutput()
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
	cmd := exec.CommandContext(ctx, "git", "diff", "--name-only", "--diff-filter=U")
	cmd.Dir = repoPath
	output, err := cmd.Output()
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
	cmd := exec.CommandContext(ctx, "git", "show", fmt.Sprintf("%s:%s", ref, filePath))
	cmd.Dir = repoPath
	output, err := cmd.Output()
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

// AbortMerge aborts an in-progress merge.
func (o *Operations) AbortMerge(ctx context.Context, repoPath string) error {
	cmd := exec.CommandContext(ctx, "git", "merge", "--abort")
	cmd.Dir = repoPath
	return cmd.Run()
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
	cmd := exec.CommandContext(ctx, "git", "remote", "-v")
	cmd.Dir = repoPath
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	seen := make(map[string]bool)
	var result []RemoteInfo
	for _, line := range strings.Split(string(output), "\n") {
		parts := strings.Fields(line)
		if len(parts) >= 2 && strings.HasSuffix(parts[len(parts)-1], "(fetch)") {
			name := parts[0]
			url := parts[1]
			if !seen[name] {
				seen[name] = true
				result = append(result, RemoteInfo{Name: name, URL: url})
			}
		}
	}
	return result, nil
}
