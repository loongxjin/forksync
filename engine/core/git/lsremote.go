package git

import (
	"context"
	"fmt"
	"strings"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/loongxjin/forksync/engine/pkg/types"
)

// shouldSkipFetch reports whether a full fetch can be skipped because the
// upstream's current HEAD hash matches the locally-tracked upstream ref hash.
// Both hashes must be known and non-empty; an unknown hash on either side
// forces a fetch (never suppress a real fetch on inconclusive evidence).
func shouldSkipFetch(remoteHash, localHash string) bool {
	if remoteHash == "" || localHash == "" {
		return false
	}
	return remoteHash == localHash
}

// remoteHeadHash queries the upstream (via ls-remote, no object transfer) for
// the current hash of the repo's target branch. Returns "" if the query fails
// or the ref is absent — callers must treat "" as "unknown, proceed to fetch".
//
// The CLI is tried first: git reads the user's global http.proxy config and
// libcurl manages the connection more robustly than go-git's HTTP transport,
// which is unreliable on large ref advertisements and does not read git config.
// go-git ls-remote is the fallback for environments where the CLI is unavailable.
func (o *Operations) remoteHeadHash(ctx context.Context, repo types.Repo) string {
	if hash, err := o.lsRemoteCLI(ctx, repo); err == nil {
		return hash
	}
	if hash, err := o.lsRemoteGoGit(ctx, repo); err == nil {
		return hash
	}
	return ""
}

// lsRemoteGoGit performs a go-git ls-remote against the upstream and returns
// the hash of the repo's target branch. Only refs are listed — no objects are
// transferred, so this is far cheaper than a full fetch when the upstream is
// unchanged.
func (o *Operations) lsRemoteGoGit(ctx context.Context, repo types.Repo) (string, error) {
	r, err := git.PlainOpen(repo.Path)
	if err != nil {
		return "", fmt.Errorf("open repo: %w", err)
	}

	remoteName := repo.RemoteName()
	remote, err := r.Remote(remoteName)
	if err != nil {
		// If the remote isn't configured locally but we have an upstream URL,
		// the caller's fetch path will create it; for the preflight we just
		// report "unknown" so a real fetch runs.
		return "", fmt.Errorf("get remote %s: %w", remoteName, err)
	}

	refs, err := remote.ListContext(ctx, &git.ListOptions{
		ProxyOptions: o.proxyOptions(),
	})
	if err != nil {
		return "", fmt.Errorf("ls-remote: %w", err)
	}

	// Resolve the branch we care about among the advertised refs.
	branch := repo.Branch
	if branch == "" {
		// Fall back to the local HEAD branch.
		head, headErr := r.Head()
		if headErr != nil {
			return "", fmt.Errorf("get HEAD for branch inference: %w", headErr)
		}
		branch = head.Name().Short()
	}
	branchRef := plumbing.NewBranchReferenceName(branch).String()

	for _, ref := range refs {
		if ref.Name().String() == branchRef {
			return ref.Hash().String(), nil
		}
	}
	return "", fmt.Errorf("branch %s not advertised by upstream", branch)
}

// localTrackingHash returns the hash of the locally-tracked upstream ref for
// the repo's branch (e.g. refs/remotes/upstream/main). Returns "" if the ref
// does not exist (never fetched) so callers force a real fetch.
func (o *Operations) localTrackingHash(repo types.Repo) string {
	r, err := git.PlainOpen(repo.Path)
	if err != nil {
		return ""
	}
	branch := repo.Branch
	if branch == "" {
		head, headErr := r.Head()
		if headErr != nil {
			return ""
		}
		branch = head.Name().Short()
	}
	remoteBranch := repo.GetRemoteBranchForLocal(branch)
	refName := fmt.Sprintf("refs/remotes/%s/%s", repo.RemoteName(), remoteBranch)
	ref, err := r.Reference(plumbing.ReferenceName(refName), true)
	if err != nil {
		return ""
	}
	return ref.Hash().String()
}

// lsRemoteCLI runs `git ls-remote <remote> refs/heads/<branch>` and returns the
// advertised hash. Used as a fallback when go-git's ls-remote fails.
func (o *Operations) lsRemoteCLI(ctx context.Context, repo types.Repo) (string, error) {
	r, err := git.PlainOpen(repo.Path)
	if err != nil {
		return "", fmt.Errorf("open repo: %w", err)
	}
	branch := repo.Branch
	if branch == "" {
		head, headErr := r.Head()
		if headErr != nil {
			return "", fmt.Errorf("get HEAD for branch inference: %w", headErr)
		}
		branch = head.Name().Short()
	}
	remoteName := repo.RemoteName()
	refSpec := "refs/heads/" + branch

	output, err := o.runGit(ctx, repo.Path, "ls-remote", remoteName, refSpec)
	if err != nil {
		return "", fmt.Errorf("git ls-remote: %w", err)
	}
	// Output format: "<40-hex-hash>\trefs/heads/<branch>\n". Take the first
	// field of the first line.
	for _, line := range strings.Split(string(output), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[1] == refSpec {
			return fields[0], nil
		}
	}
	return "", fmt.Errorf("branch %s not advertised by %s", branch, remoteName)
}
