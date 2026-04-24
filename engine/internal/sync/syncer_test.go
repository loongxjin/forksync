package sync

import (
	"context"
	"sync"
	"testing"

	"github.com/loongxjin/forksync/engine/internal/notify"
	"github.com/loongxjin/forksync/engine/pkg/types"
)

// ---------------------------------------------------------------------------
// mockStore implements repo.Store for testing.
// ---------------------------------------------------------------------------

type mockStore struct {
	mu    sync.RWMutex
	repos map[string]types.Repo
}

func newMockStore(repos ...types.Repo) *mockStore {
	s := &mockStore{repos: make(map[string]types.Repo)}
	for _, r := range repos {
		s.repos[r.ID] = r
	}
	return s
}

func (m *mockStore) List() ([]types.Repo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]types.Repo, 0, len(m.repos))
	for _, r := range m.repos {
		out = append(out, r)
	}
	return out, nil
}

func (m *mockStore) Get(id string) (types.Repo, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	r, ok := m.repos[id]
	return r, ok
}

func (m *mockStore) GetByName(name string) (types.Repo, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, r := range m.repos {
		if r.Name == name {
			return r, true
		}
	}
	return types.Repo{}, false
}

func (m *mockStore) Add(repo types.Repo) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.repos[repo.ID] = repo
	return nil
}

func (m *mockStore) Update(repo types.Repo) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.repos[repo.ID] = repo
	return nil
}

func (m *mockStore) Remove(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.repos, id)
	return nil
}

// ---------------------------------------------------------------------------
// Result.ToSyncResult
// ---------------------------------------------------------------------------

func TestResult_ToSyncResult(t *testing.T) {
	r := &Result{
		RepoID:         "repo-1",
		RepoName:       "my-repo",
		Status:         "up_to_date",
		CommitsPulled:  5,
		ConflictFiles:  []string{"a.go", "b.go"},
		ErrorMessage:   "",
		AgentUsed:      "claude",
		ConflictsFound: 2,
		AutoResolved:   2,
		PendingConfirm: []string{},
	}
	got := r.ToSyncResult()

	if got.RepoID != "repo-1" {
		t.Errorf("RepoID = %q, want %q", got.RepoID, "repo-1")
	}
	if got.RepoName != "my-repo" {
		t.Errorf("RepoName = %q, want %q", got.RepoName, "my-repo")
	}
	if got.Status != types.RepoStatus("up_to_date") {
		t.Errorf("Status = %q, want %q", got.Status, "up_to_date")
	}
	if got.CommitsPulled != 5 {
		t.Errorf("CommitsPulled = %d, want 5", got.CommitsPulled)
	}
	if len(got.ConflictFiles) != 2 || got.ConflictFiles[0] != "a.go" || got.ConflictFiles[1] != "b.go" {
		t.Errorf("ConflictFiles = %v, want [a.go b.go]", got.ConflictFiles)
	}
	if got.ErrorMessage != "" {
		t.Errorf("ErrorMessage = %q, want empty", got.ErrorMessage)
	}
	if got.AgentUsed != "claude" {
		t.Errorf("AgentUsed = %q, want %q", got.AgentUsed, "claude")
	}
	if got.ConflictsFound != 2 {
		t.Errorf("ConflictsFound = %d, want 2", got.ConflictsFound)
	}
	if got.AutoResolved != 2 {
		t.Errorf("AutoResolved = %d, want 2", got.AutoResolved)
	}
}

func TestResult_ToSyncResult_Error(t *testing.T) {
	r := &Result{
		RepoID:       "repo-2",
		RepoName:     "broken",
		Status:       "error",
		ErrorMessage: "fetch failed: timeout",
	}
	got := r.ToSyncResult()

	if got.Status != types.RepoStatusError {
		t.Errorf("Status = %q, want %q", got.Status, types.RepoStatusError)
	}
	if got.ErrorMessage != "fetch failed: timeout" {
		t.Errorf("ErrorMessage = %q, want %q", got.ErrorMessage, "fetch failed: timeout")
	}
	if got.CommitsPulled != 0 {
		t.Errorf("CommitsPulled = %d, want 0", got.CommitsPulled)
	}
	if got.ConflictFiles != nil {
		t.Errorf("ConflictFiles = %v, want nil", got.ConflictFiles)
	}
	if got.AgentUsed != "" {
		t.Errorf("AgentUsed = %q, want empty", got.AgentUsed)
	}
}

// ---------------------------------------------------------------------------
// NewSyncer
// ---------------------------------------------------------------------------

func TestNewSyncer(t *testing.T) {
	store := newMockStore()
	s := NewSyncer(store)
	if s == nil {
		t.Fatal("NewSyncer returned nil")
	}
	if s.store == nil {
		t.Error("store is nil")
	}
	if s.gitOps == nil {
		t.Error("gitOps is nil")
	}
	if s.active == nil {
		t.Error("active map is nil")
	}
	if len(s.active) != 0 {
		t.Errorf("active map = %v, want empty", s.active)
	}
}

// ---------------------------------------------------------------------------
// SyncRepo — "already syncing" guard
// ---------------------------------------------------------------------------

func TestSyncRepo_AlreadySyncing(t *testing.T) {
	repo := types.Repo{ID: "r1", Name: "test-repo"}
	store := newMockStore(repo)
	s := NewSyncer(store)

	// Manually mark the repo as active.
	s.active["r1"] = true

	result := s.SyncRepo(context.Background(), repo)
	if result.Status != "error" {
		t.Errorf("Status = %q, want %q", result.Status, "error")
	}
	if result.ErrorMessage != "sync already in progress" {
		t.Errorf("ErrorMessage = %q, want %q", result.ErrorMessage, "sync already in progress")
	}
}

// ---------------------------------------------------------------------------
// SyncRepo — concurrent access safety
// ---------------------------------------------------------------------------

func TestSyncRepo_ConcurrentAccess(t *testing.T) {
	repo := types.Repo{ID: "r1", Name: "concurrent-repo"}
	store := newMockStore(repo)
	s := NewSyncer(store)

	var wg sync.WaitGroup
	errCount := 0
	successCount := 0
	var mu sync.Mutex

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result := s.SyncRepo(context.Background(), repo)
			mu.Lock()
			if result.Status == "error" && result.ErrorMessage == "sync already in progress" {
				errCount++
			} else {
				successCount++
			}
			mu.Unlock()
		}()
	}
	wg.Wait()

	// At most 1 goroutine should have passed the active guard.
	if successCount > 1 {
		t.Errorf("expected at most 1 sync attempt, got %d", successCount)
	}
	// The rest should have been rejected.
	if errCount < 9 {
		t.Errorf("expected at least 9 rejections, got %d", errCount)
	}
}

// ---------------------------------------------------------------------------
// SyncAll — repos without upstream are skipped
// ---------------------------------------------------------------------------

func TestSyncAll_SkipsReposWithoutUpstream(t *testing.T) {
	r1 := types.Repo{ID: "a", Name: "with-upstream", Upstream: "origin/main"}
	r2 := types.Repo{ID: "b", Name: "no-upstream", Upstream: ""}
	r3 := types.Repo{ID: "c", Name: "also-no-upstream", Upstream: ""}
	store := newMockStore(r1, r2, r3)
	s := NewSyncer(store)

	results := s.SyncAll(context.Background())

	// Only the repo with upstream should produce a result.
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].RepoID != "a" {
		t.Errorf("result RepoID = %q, want %q", results[0].RepoID, "a")
	}
	// The result itself will be an error (fetch failed) which is expected.
	if results[0].Status != "error" {
		t.Errorf("result Status = %q, want %q", results[0].Status, "error")
	}
}

func TestSyncAll_ListError(t *testing.T) {
	s := NewSyncer(&listErrorStore{})
	results := s.SyncAll(context.Background())

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Status != "error" {
		t.Errorf("Status = %q, want %q", results[0].Status, "error")
	}
}

// listErrorStore is a repo.Store whose List always returns an error.
type listErrorStore struct {
	mockStore
}

func (l *listErrorStore) List() ([]types.Repo, error) {
	return nil, errTestList
}

var errTestList = func() error {
	return &testListErr{}
}()

type testListErr struct{}

func (testListErr) Error() string { return "test list error" }

// ---------------------------------------------------------------------------
// notifyResult — tests that the notifier methods are called for each status
// ---------------------------------------------------------------------------

func TestNotifyResult_NilNotifier(t *testing.T) {
	s := &Syncer{notifier: nil}
	// Should not panic.
	s.notifyResult("repo", &Result{Status: "up_to_date", CommitsPulled: 3})
	s.notifyResult("repo", &Result{Status: "conflict", ConflictFiles: []string{"a.go"}})
	s.notifyResult("repo", &Result{Status: "error", ErrorMessage: "boom"})
}

func TestNotifyResult_Synced(t *testing.T) {
	s := &Syncer{
		notifier: &notify.Notifier{},
	}
	// Test with commits > 0 — should call NotifySyncSuccess path.
	s.notifyResult("my-repo", &Result{Status: "up_to_date", CommitsPulled: 5})
}

func TestNotifyResult_SyncedZeroCommits(t *testing.T) {
	s := &Syncer{notifier: notify.Disabled()}
	s.notifyResult("my-repo", &Result{Status: "up_to_date", CommitsPulled: 0})
}

func TestNotifyResult_Conflict(t *testing.T) {
	s := &Syncer{notifier: notify.Disabled()}
	s.notifyResult("my-repo", &Result{
		Status:        "conflict",
		ConflictFiles: []string{"a.go", "b.go"},
	})
}

func TestNotifyResult_Error(t *testing.T) {
	s := &Syncer{notifier: notify.Disabled()}
	s.notifyResult("my-repo", &Result{
		Status:       "error",
		ErrorMessage: "fetch failed",
	})
}

func TestNotifyResult_UnknownStatus(t *testing.T) {
	s := &Syncer{notifier: notify.Disabled()}
	// Unknown status — none of the notifier methods should be called.
	s.notifyResult("my-repo", &Result{Status: "unknown"})
}

// ---------------------------------------------------------------------------
// updateRepoStatus
// ---------------------------------------------------------------------------

func TestUpdateRepoStatus_RepoNotFound(t *testing.T) {
	store := newMockStore() // empty
	s := NewSyncer(store)
	// Should not panic when repo is not found.
	s.updateRepoStatus("nonexistent", types.RepoStatusUpToDate, "")
}

func TestUpdateRepoStatus_SetsUpToDate(t *testing.T) {
	r := types.Repo{ID: "r1", Name: "test", Status: types.RepoStatusSyncing}
	store := newMockStore(r)
	s := NewSyncer(store)

	s.updateRepoStatus("r1", types.RepoStatusUpToDate, "")

	updated, ok := store.Get("r1")
	if !ok {
		t.Fatal("repo not found")
	}
	if updated.Status != types.RepoStatusUpToDate {
		t.Errorf("Status = %q, want %q", updated.Status, types.RepoStatusUpToDate)
	}
	if updated.LastSync == nil {
		t.Error("LastSync should be set for up_to_date status")
	}
}

func TestUpdateRepoStatus_SetsError(t *testing.T) {
	r := types.Repo{ID: "r1", Name: "test", Status: types.RepoStatusSyncing}
	store := newMockStore(r)
	s := NewSyncer(store)

	s.updateRepoStatus("r1", types.RepoStatusError, "fetch failed")

	updated, ok := store.Get("r1")
	if !ok {
		t.Fatal("repo not found")
	}
	if updated.Status != types.RepoStatusError {
		t.Errorf("Status = %q, want %q", updated.Status, types.RepoStatusError)
	}
	if updated.ErrorMessage != "fetch failed" {
		t.Errorf("ErrorMessage = %q, want %q", updated.ErrorMessage, "fetch failed")
	}
	if updated.LastSync != nil {
		t.Error("LastSync should be nil for error status")
	}
}

// ---------------------------------------------------------------------------
// SetNotifier
// ---------------------------------------------------------------------------

func TestSetNotifier(t *testing.T) {
	s := NewSyncer(newMockStore())
	if s.notifier != nil {
		t.Error("expected nil notifier initially")
	}
	n := notify.New()
	s.SetNotifier(n)
	if s.notifier != n {
		t.Error("SetNotifier did not set the notifier")
	}
}

// ---------------------------------------------------------------------------
// SetSessionManager
// ---------------------------------------------------------------------------

func TestSetSessionManager(t *testing.T) {
	s := NewSyncer(newMockStore())
	if s.sessionMgr != nil {
		t.Error("expected nil session manager initially")
	}
	s.SetSessionManager(nil)
	// Setting nil is valid — means no auto-resolve
	if s.sessionMgr != nil {
		t.Error("expected nil session manager after SetSessionManager(nil)")
	}
}
