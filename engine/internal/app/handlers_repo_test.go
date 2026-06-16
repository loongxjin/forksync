package app

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/loongxjin/forksync/engine/pkg/types"
)

// initTempGitRepo creates a real git repo in a temp dir with one initial
// commit, returns its path.
func initTempGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "Test User")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Test\n"), 0644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	run("add", "README.md")
	run("commit", "-m", "initial commit")
	return dir
}

// TestHandleRepoDiff_OK covers the happy path and guards the diff range:
// after an agent resolves and `git add`s a change, a plain `git diff` would
// be empty. The handler must return staged changes via `git diff HEAD`.
func TestHandleRepoDiff_OK(t *testing.T) {
	dir := initTempGitRepo(t)

	// Stage a change (simulates post-resolve `git add`).
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Changed\n"), 0644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	if out, err := exec.Command("git", "-C", dir, "add", "README.md").CombinedOutput(); err != nil {
		t.Fatalf("git add: %v\n%s", err, out)
	}

	s := newTestServer(t)
	if err := s.deps.Store.Add(types.Repo{Name: "demo", Path: dir}); err != nil {
		t.Fatalf("store add: %v", err)
	}

	rec := do(t, s, http.MethodGet, "/repos/demo/diff", "")
	var bare repoDiffResult
	if err := json.Unmarshal(rec.Body.Bytes(), &bare); err != nil {
		t.Fatalf("diff body not valid bare JSON: %v", err)
	}
	if !bare.Success {
		t.Fatalf("diff should succeed, error=%q", bare.Error)
	}
	if bare.Diff == "" {
		t.Fatal("diff should include the staged change relative to HEAD")
	}

	// Sanity: the underlying Operations.Diff uses HEAD too.
	ops := s.deps.GitOps
	out, err := ops.Diff(context.Background(), dir)
	if err != nil {
		t.Fatalf("Operations.Diff: %v", err)
	}
	if len(out) == 0 {
		t.Fatal("Operations.Diff should return staged change vs HEAD")
	}
}
