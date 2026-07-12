package git

import (
	"context"
	"fmt"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/loongxjin/forksync/engine/core/logger"
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
func (o *Operations) remoteHeadHash(ctx context.Context, repo types.Repo) string {
	hash, err := o.lsRemoteGoGit(ctx, repo)
	if err != nil {
		logger.Debug("git: ls-remote preflight failed, will fetch normally",
			"repo", repo.Name, "error", err)
		return ""
	}
	return hash
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
