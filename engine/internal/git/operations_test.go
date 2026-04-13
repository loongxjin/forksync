package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
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

func TestStatus_FreshRepo(t *testing.T) {
	dir := setupTempGitRepo(t)
	ops := NewOperations()

	repo := types.Repo{Path: dir}
	result, err := ops.Status(context.Background(), repo)
	require.NoError(t, err, "Status should not error")
	assert.NotNil(t, result)
	assert.Equal(t, 0, result.AheadBy, "fresh repo should have 0 ahead")
	assert.Equal(t, 0, result.BehindBy, "fresh repo should have 0 behind")
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

	repo := types.Repo{Path: dir}
	_, err := ops.Merge(context.Background(), repo)
	assert.Error(t, err, "Merge with no upstream should return error")
}
