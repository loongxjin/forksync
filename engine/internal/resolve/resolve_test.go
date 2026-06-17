package resolve

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/loongxjin/forksync/engine/internal/agent"
	"github.com/loongxjin/forksync/engine/internal/agent/session"
	"github.com/loongxjin/forksync/engine/internal/config"
	"github.com/loongxjin/forksync/engine/internal/git"
	"github.com/loongxjin/forksync/engine/pkg/types"
)

// These tests characterize the Resolver's current behavior so the upcoming
// refactor (verify-and-stage convergence + tryAgentResolve convergence onto
// the Resolver core) has a safety net. They mock the git seam and the agent
// provider — no real CLI or repo is touched.

// ---------------------------------------------------------------------------
// fakes
// ---------------------------------------------------------------------------

// fakeGitOps implements git.OperationsProvider with controllable answers.
// Only the methods Resolver touches are meaningfully exercised; the rest panic
// if called, which would surface an unexpected dependency rather than hide it.
type fakeGitOps struct {
	mu sync.Mutex

	// inputs / return knobs
	conflicts      []string          // returned by DetectConflicts
	fileContent    map[string]string // returned by GetConflictedContent (per file)
	contentErr     map[string]error  // optional read error per file
	diffBytes      []byte            // returned by Diff
	stagedFiles    []string          // records every StageFile call
	stageErr       map[string]error  // optional stage error per file
	abortCalled    bool
	commitCalled   bool
	stageAllCalled bool
}

func newFakeGitOps() *fakeGitOps {
	return &fakeGitOps{
		fileContent: make(map[string]string),
		contentErr:  make(map[string]error),
		stageErr:    make(map[string]error),
	}
}

func (f *fakeGitOps) DetectConflicts(_ context.Context, _ string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.conflicts...)
}

func (f *fakeGitOps) GetConflictedContent(_ context.Context, _ string, file string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err, ok := f.contentErr[file]; ok {
		return "", err
	}
	return f.fileContent[file], nil
}

func (f *fakeGitOps) StageFile(_ context.Context, _ string, file string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stagedFiles = append(f.stagedFiles, file)
	return f.stageErr[file]
}

func (f *fakeGitOps) Diff(_ context.Context, _ string) ([]byte, error) { return f.diffBytes, nil }

// FilterResolvedFiles mirrors the real *Operations implementation so the
// characterization tests exercise the same verify-and-stage semantics through
// the git seam, using the fake's content/stage knobs.
func (f *fakeGitOps) FilterResolvedFiles(ctx context.Context, _ string, files []string) []string {
	var still []string
	for _, file := range files {
		content, err := f.GetConflictedContent(ctx, "", file)
		if err != nil {
			still = append(still, file)
			continue
		}
		if git.HasConflictMarkers(content) {
			still = append(still, file)
			continue
		}
		if err := f.StageFile(ctx, "", file); err != nil {
			still = append(still, file)
		}
	}
	return still
}

func (f *fakeGitOps) AbortMerge(_ context.Context, _ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.abortCalled = true
	return nil
}

func (f *fakeGitOps) Commit(_ context.Context, _ string, _ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.commitCalled = true
	return nil
}

func (f *fakeGitOps) StageAll(_ context.Context, _ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stageAllCalled = true
	return nil
}

// The remaining OperationsProvider methods are not exercised by Resolver.
func (f *fakeGitOps) Fetch(context.Context, types.Repo) error { panic("not used") }
func (f *fakeGitOps) Status(context.Context, types.Repo) (*git.StatusResult, error) {
	panic("not used")
}
func (f *fakeGitOps) Merge(context.Context, types.Repo) (*git.MergeResult, error) { panic("not used") }
func (f *fakeGitOps) ResolveUpstreamRef(context.Context, types.Repo) string       { panic("not used") }
func (f *fakeGitOps) IsGitRepo(context.Context, string) bool                      { panic("not used") }
func (f *fakeGitOps) IsMergingState(context.Context, string) (bool, []string, error) {
	panic("not used")
}
func (f *fakeGitOps) DiffStaged(context.Context, string) ([]byte, error) { panic("not used") }
func (f *fakeGitOps) GetHEAD(context.Context, string) (string, error)    { panic("not used") }
func (f *fakeGitOps) GetPreMergeHEAD(context.Context, string) (string, error) {
	panic("not used")
}
func (f *fakeGitOps) CommitNoEdit(context.Context, string) error { panic("not used") }
func (f *fakeGitOps) CheckStaged(context.Context, string) error  { panic("not used") }
func (f *fakeGitOps) GetRemotes(context.Context, string) ([]git.RemoteInfo, error) {
	panic("not used")
}
func (f *fakeGitOps) FindRemoteURL(context.Context, string, string) string { panic("not used") }
func (f *fakeGitOps) GetLocalBranches(context.Context, string) ([]string, error) {
	panic("not used")
}
func (f *fakeGitOps) GetRemoteBranches(context.Context, string, string) ([]string, error) {
	panic("not used")
}
func (f *fakeGitOps) GetCommitLog(context.Context, string, string, string) ([]git.CommitInfo, error) {
	panic("not used")
}

// fakeAgentProvider implements agent.AgentProvider with a controllable result.
type fakeAgentProvider struct {
	resolveResult *agent.AgentResult
	resolveErr    error
}

func (p *fakeAgentProvider) Name() string      { return "fake" }
func (p *fakeAgentProvider) IsAvailable() bool { return true }
func (p *fakeAgentProvider) StartSession(_ context.Context, _ agent.SessionOptions) (*agent.Session, error) {
	return &agent.Session{ID: "sess-1", Provider: "fake", StartedAt: time.Now(), IsNew: true}, nil
}
func (p *fakeAgentProvider) ResolveConflicts(_ context.Context, _ *agent.Session, _ string) (*agent.AgentResult, error) {
	return p.resolveResult, p.resolveErr
}
func (p *fakeAgentProvider) EndSession(context.Context, string) error { return nil }

// mockStore implements repo.Store.
type mockStore struct {
	mu    sync.Mutex
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
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]types.Repo, 0, len(m.repos))
	for _, r := range m.repos {
		out = append(out, r)
	}
	return out, nil
}
func (m *mockStore) Get(id string) (types.Repo, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.repos[id]
	return r, ok
}
func (m *mockStore) GetByName(name string) (types.Repo, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
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

// newTestResolver wires a Resolver against fakes. The session.Manager is real
// (wrapping a fake provider) so the full ResolveWithAgent path is exercised.
func newTestResolver(t *testing.T, gitOps *fakeGitOps, provider *fakeAgentProvider, repo types.Repo) (*Resolver, *mockStore) {
	t.Helper()
	store := newMockStore(repo)

	sessionDir := filepath.Join(t.TempDir(), "sessions")
	sessionStore := session.NewSessionStore(sessionDir)
	if err := sessionStore.Init(); err != nil {
		t.Fatalf("init session store: %v", err)
	}
	sessionMgr := session.NewManager(sessionStore, provider)

	cfgMgr := config.NewManagerWithDir(t.TempDir())
	cfg, err := cfgMgr.Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	return NewResolver(gitOps, store, cfg, cfgMgr, sessionMgr), store
}

// loadCfg builds a fresh config.Manager against a temp dir and returns the
// manager plus its loaded Config — for tests that build a Resolver inline
// (e.g. with a nil session manager) instead of via newTestResolver.
func loadCfg(t *testing.T) (*config.Config, *config.Manager) {
	t.Helper()
	cfgMgr := config.NewManagerWithDir(t.TempDir())
	cfg, err := cfgMgr.Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	return cfg, cfgMgr
}

// conflictMarkers is a string with git conflict markers.
const conflictMarkers = "<<<<<<< HEAD\na\n=======\nb\n>>>>>>> upstream\n"

// ---------------------------------------------------------------------------
// ResolveWithAgent
// ---------------------------------------------------------------------------

// Happy path: agent resolves all conflicts, markers gone → files staged,
// NO commit, NO state transitions (caller owns the state machine).
func TestResolveWithAgent_SuccessStagesNoCommitNoStateTransition(t *testing.T) {
	repo := types.Repo{ID: "r1", Name: "demo", Path: "/repo", Status: types.RepoStatusConflict}
	gitOps := newFakeGitOps()
	gitOps.conflicts = []string{"a.go", "b.go"}
	// Agent's edits left the files clean (no markers).
	gitOps.fileContent = map[string]string{"a.go": "package a", "b.go": "package b"}
	gitOps.diffBytes = []byte("diff --git a/a.go b/a.go")

	provider := &fakeAgentProvider{resolveResult: &agent.AgentResult{Success: true}}

	r, store := newTestResolver(t, gitOps, provider, repo)

	out, err := r.ResolveWithAgent(context.Background(), repo, "overwrite", nil)
	if err != nil {
		t.Fatalf("ResolveWithAgent: unexpected error %v", err)
	}
	if !out.Success {
		t.Fatalf("out.Success = false, want true")
	}
	if len(out.Unresolved) != 0 {
		t.Errorf("Unresolved = %v, want empty", out.Unresolved)
	}
	if len(gitOps.stagedFiles) != 2 {
		t.Errorf("staged files = %v, want both conflict files staged", gitOps.stagedFiles)
	}
	if gitOps.commitCalled {
		t.Error("Resolver committed; it must NOT commit (CONTEXT: Resolve is separate from Resolve Commit)")
	}
	// Resolver must NOT transition state — caller owns the state machine.
	if got := out.Repo.Status; got != types.RepoStatusConflict {
		t.Errorf("repo status = %q, want %q (unchanged — caller transitions state)", got, types.RepoStatusConflict)
	}
	// Resolver must NOT persist — caller owns persistence.
	stored, ok := store.Get("r1")
	if !ok {
		t.Fatal("repo not found in store")
	}
	if stored.Status != types.RepoStatusConflict {
		t.Errorf("stored status = %q, want %q (unchanged — caller persists)", stored.Status, types.RepoStatusConflict)
	}
}

// Markers remain after the agent run → reported as Unresolved, nothing staged.
func TestResolveWithAgent_MarkersRemainReportUnresolved(t *testing.T) {
	repo := types.Repo{ID: "r1", Name: "demo", Path: "/repo", Status: types.RepoStatusConflict}
	gitOps := newFakeGitOps()
	gitOps.conflicts = []string{"a.go"}
	gitOps.fileContent = map[string]string{"a.go": conflictMarkers} // still has markers

	provider := &fakeAgentProvider{resolveResult: &agent.AgentResult{Success: true}}
	r, _ := newTestResolver(t, gitOps, provider, repo)

	out, err := r.ResolveWithAgent(context.Background(), repo, "overwrite", nil)
	if err != nil {
		t.Fatalf("unexpected error %v", err)
	}
	if out.Success {
		t.Error("out.Success = true, want false (markers remain)")
	}
	if len(out.Unresolved) != 1 || out.Unresolved[0] != "a.go" {
		t.Errorf("Unresolved = %v, want [a.go]", out.Unresolved)
	}
	if len(gitOps.stagedFiles) != 0 {
		t.Errorf("staged files = %v, want none when markers remain", gitOps.stagedFiles)
	}
	if gitOps.commitCalled {
		t.Error("Resolver committed on unresolved; it must NOT commit")
	}
}

// No conflicts detected → short-circuit success with no agent/stage work.
func TestResolveWithAgent_NoConflictsShortCircuits(t *testing.T) {
	repo := types.Repo{ID: "r1", Name: "demo", Path: "/repo", Status: types.RepoStatusConflict}
	gitOps := newFakeGitOps()
	gitOps.conflicts = nil // nothing to resolve
	provider := &fakeAgentProvider{resolveResult: &agent.AgentResult{Success: true}}
	r, _ := newTestResolver(t, gitOps, provider, repo)

	out, err := r.ResolveWithAgent(context.Background(), repo, "overwrite", nil)
	if err != nil {
		t.Fatalf("unexpected error %v", err)
	}
	if !out.Success {
		t.Fatal("out.Success = false, want true (no conflicts)")
	}
	if len(gitOps.stagedFiles) != 0 {
		t.Errorf("staged files = %v, want none when there were no conflicts", gitOps.stagedFiles)
	}
}

// nil session manager → error, no panic.
func TestResolveWithAgent_NilSessionManagerErrors(t *testing.T) {
	repo := types.Repo{ID: "r1", Name: "demo", Path: "/repo", Status: types.RepoStatusConflict}
	gitOps := newFakeGitOps()
	store := newMockStore(repo)
	cfgMgr := config.NewManagerWithDir(t.TempDir())
	cfg, err := cfgMgr.Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	r := NewResolver(gitOps, store, cfg, cfgMgr, nil)

	if _, err := r.ResolveWithAgent(context.Background(), repo, "overwrite", nil); err == nil {
		t.Error("expected error when sessionMgr is nil")
	}
}

// ---------------------------------------------------------------------------
// Reject
// ---------------------------------------------------------------------------

// Reject aborts the merge, marks the workflow failed, sets SyncNeeded, no commit.
func TestReject_AbortsAndSetsSyncNeeded(t *testing.T) {
	repo := types.Repo{
		ID:     "r1",
		Name:   "demo",
		Path:   "/repo",
		Status: types.RepoStatusConflict,
		Workflow: &types.SyncWorkflow{
			Status: types.WorkflowRunning,
			Steps: []types.WorkflowStepRecord{
				{Step: types.StepResolveStrategy, Status: types.StepStatusPending},
				{Step: types.StepAgentResolve, Status: types.StepStatusRunning},
			},
		},
	}
	gitOps := newFakeGitOps()
	store := newMockStore(repo)
	cfg, cfgMgr := loadCfg(t)
	r := NewResolver(gitOps, store, cfg, cfgMgr, nil)

	out, err := r.Reject(context.Background(), repo)
	if err != nil {
		t.Fatalf("Reject: unexpected error %v", err)
	}
	if !gitOps.abortCalled {
		t.Error("AbortMerge was not called")
	}
	if gitOps.commitCalled {
		t.Error("Reject committed; it must NOT commit")
	}
	if out.Status != types.RepoStatusSyncNeeded {
		t.Errorf("status = %q, want %q", out.Status, types.RepoStatusSyncNeeded)
	}
	if out.ErrorMessage != "" {
		t.Errorf("ErrorMessage = %q, want empty", out.ErrorMessage)
	}
	// Pending steps → skipped; workflow done-failed.
	if wf := out.Workflow; wf == nil {
		t.Error("workflow nil after reject")
	} else if wf.Status != types.WorkflowFailed {
		t.Errorf("workflow status = %q, want %q", wf.Status, types.WorkflowFailed)
	}
	// Persisted.
	stored, ok := store.Get("r1")
	if !ok {
		t.Fatal("repo not persisted")
	}
	if stored.Status != types.RepoStatusSyncNeeded {
		t.Errorf("stored status = %q, want %q", stored.Status, types.RepoStatusSyncNeeded)
	}
}

// ---------------------------------------------------------------------------
// Prepare
// ---------------------------------------------------------------------------

// Prepare positions the workflow at AgentResolve without invoking any agent.
func TestPrepare_PositionsWorkflowForAgent(t *testing.T) {
	repo := types.Repo{
		ID:     "r1",
		Name:   "demo",
		Path:   "/repo",
		Status: types.RepoStatusConflict,
	}
	gitOps := newFakeGitOps()
	store := newMockStore(repo)
	cfg, cfgMgr := loadCfg(t)
	r := NewResolver(gitOps, store, cfg, cfgMgr, nil)

	out, err := r.Prepare(repo)
	if err != nil {
		t.Fatalf("Prepare: unexpected error %v", err)
	}
	if out.Status != types.RepoStatusConflict {
		t.Errorf("status = %q, want %q", out.Status, types.RepoStatusConflict)
	}
	wf := out.Workflow
	if wf == nil {
		t.Fatal("workflow not built")
	}
	if wf.Status != types.WorkflowRunning {
		t.Errorf("workflow status = %q, want %q", wf.Status, types.WorkflowRunning)
	}
	resolveStrategy := workflowStep(wf, types.StepResolveStrategy)
	if resolveStrategy == nil || resolveStrategy.Status != types.StepStatusSuccess {
		t.Error("ResolveStrategy step not advanced to Success")
	}
	agentResolve := workflowStep(wf, types.StepAgentResolve)
	if agentResolve == nil || agentResolve.Status != types.StepStatusRunning {
		t.Error("AgentResolve step not advanced to Running")
	}
	// Prepare stamps a resolve session id so the frontend can locate the log.
	if agentResolve.ResolveSessionID == "" {
		t.Error("AgentResolve step has empty ResolveSessionID after Prepare")
	}
	// No git work done during Prepare.
	if gitOps.commitCalled || gitOps.stageAllCalled {
		t.Error("Prepare performed git work; it must only position the workflow")
	}
}

func workflowStep(wf *types.SyncWorkflow, step types.WorkflowStep) *types.WorkflowStepRecord {
	for i := range wf.Steps {
		if wf.Steps[i].Step == step {
			return &wf.Steps[i]
		}
	}
	return nil
}
