package sync

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/loongxjin/forksync/engine/core/git"
	"github.com/loongxjin/forksync/engine/pkg/types"
	"github.com/stretchr/testify/assert"
)

// slowFakeGitOps lets Fetch block for a configurable duration per repo path,
// so tests can simulate a slow upstream fetch. Status returns instantly.
type slowFakeGitOps struct {
	mu        sync.Mutex
	delays    map[string]time.Duration
	fetchCnt  int32
	statusCnt int32
}

func newSlowFakeGitOps() *slowFakeGitOps {
	return &slowFakeGitOps{delays: make(map[string]time.Duration)}
}

func (f *slowFakeGitOps) Fetch(ctx context.Context, repo types.Repo) error {
	atomic.AddInt32(&f.fetchCnt, 1)
	f.mu.Lock()
	d := f.delays[repo.Path]
	f.mu.Unlock()
	if d > 0 {
		select {
		case <-time.After(d):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

func (f *slowFakeGitOps) Status(ctx context.Context, repo types.Repo) (*git.StatusResult, error) {
	// Mirror real git ops: if the per-repo ctx already expired (because the
	// fetch ate the whole budget), Status fails instead of producing a result.
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	atomic.AddInt32(&f.statusCnt, 1)
	return &git.StatusResult{AheadBy: 1, BehindBy: 2}, nil
}

// The following methods are not exercised by RefreshAll's normal path.
func (f *slowFakeGitOps) Merge(context.Context, types.Repo) (*git.MergeResult, error) {
	panic("not used")
}
func (f *slowFakeGitOps) ResolveUpstreamRef(context.Context, types.Repo) string { return "" }
func (f *slowFakeGitOps) IsGitRepo(context.Context, string) bool                { return true }
func (f *slowFakeGitOps) IsMergingState(context.Context, string) (bool, []string, error) {
	return false, nil, nil
}
func (f *slowFakeGitOps) DetectConflicts(context.Context, string) []string { return nil }
func (f *slowFakeGitOps) GetConflictedContent(context.Context, string, string) (string, error) {
	return "", nil
}
func (f *slowFakeGitOps) FilterResolvedFiles(context.Context, string, []string) []string {
	return nil
}
func (f *slowFakeGitOps) Diff(context.Context, string) ([]byte, error)       { return nil, nil }
func (f *slowFakeGitOps) DiffStaged(context.Context, string) ([]byte, error) { return nil, nil }
func (f *slowFakeGitOps) GetHEAD(context.Context, string) (string, error)    { return "", nil }
func (f *slowFakeGitOps) GetPreMergeHEAD(context.Context, string) (string, error) {
	return "", nil
}
func (f *slowFakeGitOps) StageFile(context.Context, string, string) error { return nil }
func (f *slowFakeGitOps) StageAll(context.Context, string) error          { return nil }
func (f *slowFakeGitOps) Commit(context.Context, string, string) error    { return nil }
func (f *slowFakeGitOps) CommitNoEdit(context.Context, string) error      { return nil }
func (f *slowFakeGitOps) AbortMerge(context.Context, string) error        { return nil }
func (f *slowFakeGitOps) CheckStaged(context.Context, string) error       { return nil }
func (f *slowFakeGitOps) GetRemotes(context.Context, string) ([]git.RemoteInfo, error) {
	return nil, nil
}
func (f *slowFakeGitOps) FindRemoteURL(context.Context, string, string) string { return "" }
func (f *slowFakeGitOps) GetLocalBranches(context.Context, string) ([]string, error) {
	return nil, nil
}
func (f *slowFakeGitOps) GetRemoteBranches(context.Context, string, string) ([]string, error) {
	return nil, nil
}
func (f *slowFakeGitOps) GetRemoteBranchesFromURL(context.Context, string, string) ([]string, error) {
	return nil, nil
}
func (f *slowFakeGitOps) GetCommitLog(context.Context, string, string, string) ([]git.CommitInfo, error) {
	return nil, nil
}

// TestRefreshAllSlowRepoDoesNotStarveOthers verifies that a slow repo's fetch
// timeout does not cancel other repos' status checks. Each repo gets its own
// per-repo timeout budget derived from the parent context.
func TestRefreshAllSlowRepoDoesNotStarveOthers(t *testing.T) {
	gitOps := newSlowFakeGitOps()
	// /slow fetches for 800ms; others are instant.
	gitOps.delays["/slow"] = 800 * time.Millisecond

	repos := []types.Repo{
		{ID: "1", Name: "slow", Path: "/slow"},
		{ID: "2", Name: "fast-a", Path: "/fast-a"},
		{ID: "3", Name: "fast-b", Path: "/fast-b"},
	}
	store := newMockStore(repos...)

	sf := NewStatusRefresher(gitOps, store, nil)

	// Parent context generous enough that, under the old shared-timeout bug,
	// the slow repo would still consume most of the budget. We give a parent
	// deadline of 2s but a per-repo deadline of 100ms: the slow repo must be
	// cancelled at ~100ms while the fast repos complete.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	start := time.Now()
	out, err := sf.RefreshAllWithPerRepoTimeout(ctx, repos, nil, 100*time.Millisecond)
	elapsed := time.Since(start)

	assertNoErr(t, err)

	// All three repos attempted a fetch.
	assert.Equal(t, int32(3), atomic.LoadInt32(&gitOps.fetchCnt), "all repos should attempt fetch")

	// The fast repos must have their ahead/behind computed.
	fastA := findRepoByName(t, out, "fast-a")
	assert.Equal(t, 2, fastA.BehindBy, "fast repo must complete despite slow repo")

	// The slow repo's per-repo ctx expired during its 800ms fetch, so its
	// Status() call ran under an already-dead context and did not contribute
	// a fresh result. Only the 2 fast repos successfully reached Status.
	assert.Equal(t, int32(2), atomic.LoadInt32(&gitOps.statusCnt),
		"only the fast repos should complete Status; the slow repo's ctx expired")

	// The whole batch must return well under the slow repo's 800ms, proving
	// the slow repo did not block the others (it was cut off at 100ms).
	if elapsed > 500*time.Millisecond {
		t.Fatalf("RefreshAll took %v; slow repo should not block the batch", elapsed)
	}
}

// TestRefreshAllParentCancelPropagates verifies that cancelling the parent
// context still aborts all per-repo work.
func TestRefreshAllParentCancelPropagates(t *testing.T) {
	gitOps := newSlowFakeGitOps()
	gitOps.delays["/r1"] = 5 * time.Second

	repos := []types.Repo{{ID: "1", Name: "r1", Path: "/r1"}}
	store := newMockStore(repos...)
	sf := NewStatusRefresher(gitOps, store, nil)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	_, _ = sf.RefreshAllWithPerRepoTimeout(ctx, repos, nil, 10*time.Second)
	elapsed := time.Since(start)

	if elapsed > 500*time.Millisecond {
		t.Fatalf("parent cancel should abort quickly, took %v", elapsed)
	}
}

func assertNoErr(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func findRepoByName(t *testing.T, repos []types.Repo, name string) types.Repo {
	t.Helper()
	for _, r := range repos {
		if r.Name == name {
			return r
		}
	}
	t.Fatalf("repo %q not found", name)
	return types.Repo{}
}
