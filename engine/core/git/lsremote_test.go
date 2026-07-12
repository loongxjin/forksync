package git

import (
	"context"
	"os/exec"
	"testing"

	"github.com/loongxjin/forksync/engine/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestShouldSkipFetchWhenHashesMatch verifies the core preflight decision:
// when the remote HEAD hash equals the locally-tracked upstream ref hash, the
// fetch can be skipped (no new upstream commits).
func TestShouldSkipFetchWhenHashesMatch(t *testing.T) {
	assert.True(t, shouldSkipFetch("abc123", "abc123"),
		"identical hashes → skip fetch")
}

func TestShouldNotSkipWhenHashesDiffer(t *testing.T) {
	assert.False(t, shouldSkipFetch("abc123", "def456"),
		"different hashes → must fetch")
}

// TestShouldNotSkipWhenLocalHashUnknown verifies that when the local tracking
// ref has never been set (empty hash — repo never fetched), we must fetch.
func TestShouldNotSkipWhenLocalHashUnknown(t *testing.T) {
	assert.False(t, shouldSkipFetch("abc123", ""),
		"unknown local hash → must fetch")
	assert.False(t, shouldSkipFetch("", ""),
		"both unknown → must fetch")
}

// TestShouldNotSkipWhenRemoteHashUnknown verifies that a failed/inconclusive
// ls-remote (empty remote hash) never suppresses a real fetch.
func TestShouldNotSkipWhenRemoteHashUnknown(t *testing.T) {
	assert.False(t, shouldSkipFetch("", "abc123"),
		"unknown remote hash → must fetch (ls-remote failed, don't trust it)")
}

// TestFetchSkipsWhenUpstreamUnchanged is an integration test: after a real
// fetch, a second fetch against an unchanged upstream is skipped by the
// ls-remote preflight (remote HEAD hash == local tracking ref hash).
func TestFetchSkipsWhenUpstreamUnchanged(t *testing.T) {
	// upstream: a real repo with one commit, acting as the remote source.
	upstreamDir := t.TempDir()
	upRun := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = upstreamDir
		require.NoError(t, cmd.Run(), args)
	}
	upRun("init", "-q", "-b", "main")
	upRun("config", "user.email", "t@t")
	upRun("config", "user.name", "t")
	upRun("commit", "-q", "--allow-empty", "-m", "init")

	// downstream: clones from upstream so its tracking ref is populated.
	workDir := t.TempDir()
	require.NoError(t, exec.Command("git", "clone", "-q", upstreamDir, workDir).Run(), "clone")
	downRun := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = workDir
		require.NoError(t, cmd.Run(), args)
	}
	// Clone names the remote "origin"; add "upstream" pointing at the same place
	// so fetchGoGit (which uses repo.RemoteName() == "upstream") works.
	downRun("remote", "rename", "origin", "upstream")

	repo := types.Repo{Path: workDir, Name: "downstream", Branch: "main", Upstream: upstreamDir}
	o := NewOperations()
	ctx := context.Background()

	// Local tracking ref exists from clone. ls-remote preflight should match it.
	remoteHash := o.remoteHeadHash(ctx, repo)
	require.NotEmpty(t, remoteHash, "ls-remote must return the upstream hash")
	localHash := o.localTrackingHash(repo)
	require.NotEmpty(t, localHash, "local tracking ref must exist after clone")
	assert.Equal(t, remoteHash, localHash, "clone should align hashes")

	// Fetch on unchanged upstream: preflight skips the object transfer.
	assert.True(t, o.upstreamUnchanged(ctx, repo),
		"preflight should detect unchanged upstream and allow skipping")
	require.NoError(t, o.Fetch(ctx, repo), "fetch on unchanged upstream should succeed (skip)")
}

// TestFetchRunsWhenUpstreamAdvances verifies that when upstream gains a new
// commit, the preflight detects the hash mismatch and a real fetch runs.
func TestFetchRunsWhenUpstreamAdvances(t *testing.T) {
	upstreamDir := t.TempDir()
	upRun := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = upstreamDir
		require.NoError(t, cmd.Run(), args)
	}
	upRun("init", "-q", "-b", "main")
	upRun("config", "user.email", "t@t")
	upRun("config", "user.name", "t")
	upRun("commit", "-q", "--allow-empty", "-m", "init")

	workDir := t.TempDir()
	require.NoError(t, exec.Command("git", "clone", "-q", upstreamDir, workDir).Run(), "clone")
	downRun := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = workDir
		require.NoError(t, cmd.Run(), args)
	}
	downRun("remote", "rename", "origin", "upstream")

	repo := types.Repo{Path: workDir, Name: "downstream", Branch: "main", Upstream: upstreamDir}
	o := NewOperations()
	ctx := context.Background()

	// Upstream advances with a new commit.
	upRun("commit", "-q", "--allow-empty", "-m", "second")

	assert.False(t, o.upstreamUnchanged(ctx, repo),
		"preflight should detect the new upstream commit and force a fetch")

	// Real fetch pulls the new commit; tracking ref updates.
	require.NoError(t, o.Fetch(ctx, repo), "fetch should pull the new commit")
	// After fetch, hashes realign.
	assert.True(t, o.upstreamUnchanged(ctx, repo),
		"after fetch, preflight should see matching hashes again")
}
