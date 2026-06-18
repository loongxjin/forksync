package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/loongxjin/forksync/engine/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupTempGitRepo creates a temporary git repo with an initial commit.
// Returns the repo path and a cleanup function.
func setupTempGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	cmd := exec.Command("git", "init")
	cmd.Dir = dir
	require.NoError(t, cmd.Run(), "git init should succeed")

	cmd = exec.Command("git", "config", "user.email", "test@example.com")
	cmd.Dir = dir
	require.NoError(t, cmd.Run(), "git config email should succeed")

	cmd = exec.Command("git", "config", "user.name", "Test User")
	cmd.Dir = dir
	require.NoError(t, cmd.Run(), "git config name should succeed")

	// Create initial commit
	readme := filepath.Join(dir, "README.md")
	require.NoError(t, os.WriteFile(readme, []byte("# Test\n"), 0644), "write README should succeed")

	cmd = exec.Command("git", "add", "README.md")
	cmd.Dir = dir
	require.NoError(t, cmd.Run(), "git add should succeed")

	cmd = exec.Command("git", "commit", "-m", "initial commit")
	cmd.Dir = dir
	require.NoError(t, cmd.Run(), "git commit should succeed")

	return dir
}

// setupTempGitRepoWithRemote creates a temp repo with an origin remote pointing to another temp repo.
func setupTempGitRepoWithRemote(t *testing.T) (string, string) {
	t.Helper()
	// Create the "remote" repo
	remoteDir := t.TempDir()
	cmd := exec.Command("git", "init", "--bare")
	cmd.Dir = remoteDir
	require.NoError(t, cmd.Run(), "git init --bare should succeed")

	// Create the local repo
	localDir := setupTempGitRepo(t)

	// Add origin remote
	cmd = exec.Command("git", "remote", "add", "origin", remoteDir)
	cmd.Dir = localDir
	require.NoError(t, cmd.Run(), "git remote add should succeed")

	// Push to set up tracking
	cmd = exec.Command("git", "push", "-u", "origin", "master")
	cmd.Dir = localDir
	require.NoError(t, cmd.Run(), "git push should succeed")

	return localDir, remoteDir
}

func TestIsGitRepo_ValidGitRepo(t *testing.T) {
	dir := setupTempGitRepo(t)
	ops := NewOperations()

	result := ops.IsGitRepo(context.Background(), dir)
	assert.True(t, result, "should recognize valid git repo")
}

func TestIsGitRepo_NonGitDir(t *testing.T) {
	dir := t.TempDir()
	ops := NewOperations()

	result := ops.IsGitRepo(context.Background(), dir)
	assert.False(t, result, "should not recognize non-git dir as git repo")
}

func TestIsGitRepo_NonexistentPath(t *testing.T) {
	ops := NewOperations()

	result := ops.IsGitRepo(context.Background(), "/nonexistent/path/that/does/not/exist")
	assert.False(t, result, "should not recognize nonexistent path as git repo")
}

func TestGetRemotes_NoRemotes(t *testing.T) {
	dir := setupTempGitRepo(t)
	ops := NewOperations()

	remotes, err := ops.GetRemotes(context.Background(), dir)
	require.NoError(t, err, "GetRemotes should not error")
	assert.Empty(t, remotes, "fresh repo should have no remotes")
}

func TestGetRemotes_WithOrigin(t *testing.T) {
	localDir, _ := setupTempGitRepoWithRemote(t)
	ops := NewOperations()

	remotes, err := ops.GetRemotes(context.Background(), localDir)
	require.NoError(t, err, "GetRemotes should not error")
	require.Len(t, remotes, 1, "should have one remote")
	assert.Equal(t, "origin", remotes[0].Name)
	assert.NotEmpty(t, remotes[0].URL)
}

// TestStatus_FreshRepo verifies that a repo with no remote/upstream configured
// returns an error rather than silently reporting {0,0}. The old behavior of
// returning a silent 0/0 caused callers to mistake "cannot compare" for
// "up to date", masking fetch failures and missing upstream config.
func TestStatus_FreshRepo(t *testing.T) {
	dir := setupTempGitRepo(t)
	ops := NewOperations()

	repo := types.Repo{Path: dir}
	result, err := ops.Status(context.Background(), repo)
	require.Error(t, err, "Status on repo without upstream ref should error")
	assert.Nil(t, result, "no result should be returned on error")
	// Error may originate from statusGoGit ("remote ref") or the statusCLI
	// fallback ("upstream ref") — both are acceptable, the contract is that
	// ref resolution failure surfaces as an error.
	errMsg := err.Error()
	assert.True(t,
		strings.Contains(errMsg, "remote ref") || strings.Contains(errMsg, "upstream ref"),
		"error should indicate ref resolution failure, got: %s", errMsg,
	)
}

// TestStatus_WithUpstreamRef covers the normal path: a repo with a valid
// origin remote and tracking ref. Should report 0/0 and no error when the
// local branch is at parity with the remote.
func TestStatus_WithUpstreamRef(t *testing.T) {
	localDir, _ := setupTempGitRepoWithRemote(t)
	ops := NewOperations()

	repo := types.Repo{Path: localDir, Branch: "master"}
	result, err := ops.Status(context.Background(), repo)
	require.NoError(t, err, "Status on repo with upstream ref should not error")
	require.NotNil(t, result)
	assert.Equal(t, 0, result.AheadBy, "repo at parity should have 0 ahead")
	assert.Equal(t, 0, result.BehindBy, "repo at parity should have 0 behind")
	assert.NotEmpty(t, result.Branch, "branch name should not be empty")
}

func TestStatus_NonGitDir(t *testing.T) {
	dir := t.TempDir()
	ops := NewOperations()

	repo := types.Repo{Path: dir}
	_, err := ops.Status(context.Background(), repo)
	assert.Error(t, err, "Status on non-git dir should return error")
}

func TestFetch_NoRemote(t *testing.T) {
	dir := setupTempGitRepo(t)
	ops := NewOperations()

	repo := types.Repo{Path: dir}
	err := ops.Fetch(context.Background(), repo)
	assert.Error(t, err, "Fetch with no remote should return error")
}

func TestFetch_WithOrigin(t *testing.T) {
	localDir, _ := setupTempGitRepoWithRemote(t)
	ops := NewOperations()

	repo := types.Repo{Path: localDir, Origin: "origin"}
	err := ops.Fetch(context.Background(), repo)
	assert.NoError(t, err, "Fetch with valid origin should succeed")
}

func TestGetFileContent(t *testing.T) {
	dir := setupTempGitRepo(t)
	ops := NewOperations()

	content, err := ops.GetFileContent(context.Background(), dir, "HEAD", "README.md")
	require.NoError(t, err, "GetFileContent should not error")
	assert.Equal(t, "# Test\n", content, "file content should match")
}

func TestGetFileContent_NonexistentFile(t *testing.T) {
	dir := setupTempGitRepo(t)
	ops := NewOperations()

	_, err := ops.GetFileContent(context.Background(), dir, "HEAD", "nonexistent.txt")
	assert.Error(t, err, "GetFileContent for nonexistent file should return error")
}

func TestNewOperations(t *testing.T) {
	ops := NewOperations()
	assert.NotNil(t, ops, "NewOperations should return non-nil instance")
}

func TestGetRemotes_NonGitDir(t *testing.T) {
	ops := NewOperations()
	_, err := ops.GetRemotes(context.Background(), t.TempDir())
	// Should fall back to CLI which will also fail
	assert.Error(t, err, "GetRemotes on non-git dir should return error")
}

func TestMerge_NoUpstream(t *testing.T) {
	dir := setupTempGitRepo(t)
	ops := NewOperations()

	// When no upstream is configured, RemoteName() returns "origin".
	// The temp repo has an origin remote, so merge should succeed.
	repo := types.Repo{Path: dir}
	result, err := ops.Merge(context.Background(), repo)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.False(t, result.HasConflicts)
}

func TestHasConflictMarkers(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    bool
	}{
		{"conflict markers", "<<<<<<< HEAD\nfoo\n=======\nbar\n>>>>>>> upstream", true},
		{"only open marker", "<<<<<<< HEAD\nfoo", false},
		{"no markers", "hello world", false},
		{"empty", "", false},
		{"equals and close only", "=======\n>>>>>>> upstream", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := HasConflictMarkers(tt.content)
			if got != tt.want {
				t.Errorf("HasConflictMarkers() = %v; want %v", got, tt.want)
			}
		})
	}
}
