package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/loongxjin/forksync/engine/core/agent"
	"github.com/loongxjin/forksync/engine/core/agent/session"
	"github.com/loongxjin/forksync/engine/core/app"
	"github.com/loongxjin/forksync/engine/core/config"
	"github.com/loongxjin/forksync/engine/core/eventbus"
	"github.com/loongxjin/forksync/engine/core/git"
	"github.com/loongxjin/forksync/engine/core/github"
	"github.com/loongxjin/forksync/engine/core/history"
	"github.com/loongxjin/forksync/engine/core/logger"
	respkg "github.com/loongxjin/forksync/engine/core/resolve"
	syncpkg "github.com/loongxjin/forksync/engine/core/sync"
	"github.com/loongxjin/forksync/engine/core/workflow"
	"github.com/loongxjin/forksync/engine/pkg/types"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App is the Wails application struct.
type App struct {
	ctx    context.Context
	deps   *app.Deps
	cfgMgr *config.Manager
}

func NewApp() *App { return &App{} }

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	cfgMgr := config.NewManager()
	a.cfgMgr = cfgMgr
	logDir := cfgMgr.ConfigDir() + "/logs"
	if err := logger.Init(logDir); err != nil {
		logger.Warn("app: logger init failed", "error", err)
	}
	deps, err := app.BuildDeps()
	if err != nil {
		logger.Error("app: failed to build dependencies", "error", err)
		return
	}
	a.deps = deps
	if deps.Cfg != nil && deps.Cfg.Sync.SyncOnStartup {
		deps.StartScheduler(ctx)
	}
	if deps.Bus != nil {
		ch, cancel := deps.Bus.Subscribe()
		go func() {
			defer cancel()
			for ev := range ch {
				runtime.EventsEmit(ctx, "engine:event", string(ev.Type))
			}
		}()
	}
	logger.Info("app: wails started")
}

func (a *App) shutdown(_ context.Context) {
	if a.deps != nil {
		a.deps.Close()
	}
	logger.Close()
}

// ---------------------------------------------------------------------------
// Status
// ---------------------------------------------------------------------------

const statusTimeout = 30 * time.Second

func (a *App) Status(exclude []string) (types.StatusData, error) {
	if a.deps == nil {
		return types.StatusData{}, nil
	}
	ctx, cancel := context.WithTimeout(a.ctx, statusTimeout)
	defer cancel()
	repos, err := a.deps.Store.List()
	if err != nil {
		return types.StatusData{}, err
	}
	refresher := syncpkg.NewStatusRefresher(a.deps.GitOps, a.deps.RawStore(), a.deps.Cfg)
	repos, err = refresher.RefreshAll(ctx, repos, exclude)
	if err != nil {
		return types.StatusData{}, err
	}
	agents := a.deps.AgentRegistry.Discover()
	preferredAgent := ""
	if len(agents) > 0 {
		preferredAgent = agents[0].Name
	}
	return types.StatusData{Repos: repos, Agents: agents, PreferredAgent: preferredAgent}, nil
}

func (a *App) Greet(name string) string {
	if a.deps == nil {
		return "engine not ready"
	}
	return "ForkSync engine ready, " + name
}

// ---------------------------------------------------------------------------
// Sync
// ---------------------------------------------------------------------------

func (a *App) SyncAll() (types.SyncData, error) {
	if a.deps == nil {
		return types.SyncData{}, nil
	}
	results := a.deps.Syncer.SyncAll(a.ctx)
	syncResults := make([]types.SyncResult, 0, len(results))
	for _, res := range results {
		syncResults = append(syncResults, res.ToSyncResult())
	}
	return types.SyncData{Results: syncResults}, nil
}

func (a *App) SyncRepo(name string) (types.SyncData, error) {
	if a.deps == nil {
		return types.SyncData{}, nil
	}
	r2, ok := a.deps.Store.GetByName(name)
	if !ok {
		return types.SyncData{}, fmt.Errorf("repository %q not found", name)
	}
	result := a.deps.Syncer.SyncRepo(a.ctx, r2)
	return types.SyncData{Results: []types.SyncResult{result.ToSyncResult()}}, nil
}

// ---------------------------------------------------------------------------
// Scan / Add / Remove / Diff
// ---------------------------------------------------------------------------

func (a *App) Scan(dir string) (types.ScanData, error) {
	if a.deps == nil {
		return types.ScanData{}, nil
	}
	info, err := os.Stat(dir)
	if err != nil {
		return types.ScanData{}, fmt.Errorf("directory not found: %w", err)
	}
	if !info.IsDir() {
		return types.ScanData{}, fmt.Errorf("%s is not a directory", dir)
	}
	ghToken := ""
	if a.deps.Cfg != nil {
		ghToken = a.deps.Cfg.GitHub.Token
	}
	ghClient := github.NewClient(ghToken)
	gitOps := a.deps.GitOps
	scanned := make([]types.ScannedRepo, 0)
	skipDirs := []string{"node_modules", "vendor"}
	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if len(name) > 0 && name[0] == '.' {
				return filepath.SkipDir
			}
			for _, sk := range skipDirs {
				if name == sk {
					return filepath.SkipDir
				}
			}
		}
		if !d.IsDir() {
			return nil
		}
		if !gitOps.IsGitRepo(a.ctx, path) {
			return nil
		}
		if s := scanRepo(a.ctx, path, gitOps, ghClient); s != nil {
			scanned = append(scanned, *s)
		}
		return filepath.SkipDir
	})
	return types.ScanData{Repos: scanned}, nil
}

func scanRepo(ctx context.Context, dir string, gitOps git.OperationsProvider, ghClient *github.Client) *types.ScannedRepo {
	remotes, _ := gitOps.GetRemotes(ctx, dir)
	originURL := findRemoteURL(remotes, "origin")
	s := types.ScannedRepo{Path: dir, Name: filepath.Base(dir), Origin: originURL}
	if originURL != "" && github.IsGitHubURL(originURL) {
		owner, repo, err := github.ParseRepoURL(originURL)
		if err == nil {
			r, err := ghClient.DetectFork(ctx, owner, repo)
			if err == nil {
				s.IsFork = r.IsFork
				if r.UpstreamURL != "" {
					s.SuggestedUpstream = r.UpstreamURL
				}
			}
		}
	}
	s.LocalBranches, _ = gitOps.GetLocalBranches(ctx, dir)
	s.RemoteBranches, _ = gitOps.GetRemoteBranches(ctx, dir, "origin")
	for _, r := range remotes {
		if r.Name == "upstream" {
			b, _ := gitOps.GetRemoteBranches(ctx, dir, "upstream")
			seen := map[string]bool{}
			for _, x := range s.RemoteBranches {
				seen[x] = true
			}
			for _, x := range b {
				if !seen[x] {
					s.RemoteBranches = append(s.RemoteBranches, x)
					seen[x] = true
				}
			}
			break
		}
	}
	return &s
}

func findRemoteURL(remotes []git.RemoteInfo, name string) string {
	for _, r := range remotes {
		if r.Name == name {
			return r.URL
		}
	}
	return ""
}

// AddRepoParams mirrors POST /repos body.
type AddRepoParams struct {
	Path          string               `json:"path"`
	Upstream      string               `json:"upstream,omitempty"`
	BranchMapping *types.BranchMapping `json:"branchMapping,omitempty"`
}

func (a *App) AddRepo(params AddRepoParams) (types.AddData, error) {
	if a.deps == nil {
		return types.AddData{}, fmt.Errorf("engine not ready")
	}
	repoPath, err := filepath.Abs(params.Path)
	if err != nil {
		return types.AddData{}, fmt.Errorf("invalid path: %w", err)
	}
	if !a.deps.GitOps.IsGitRepo(a.ctx, repoPath) {
		return types.AddData{}, fmt.Errorf("%s is not a git repository", repoPath)
	}
	originURL := a.deps.GitOps.FindRemoteURL(a.ctx, repoPath, "origin")
	upstream := params.Upstream
	if upstream == "" {
		upstream = autoDetectUpstream(a.ctx, a.deps.Cfg, originURL)
	}
	statusResult, _ := a.deps.GitOps.Status(a.ctx, types.Repo{Path: repoPath, Upstream: upstream})
	branch := types.DefaultBranch
	if statusResult != nil && statusResult.Branch != "" {
		branch = statusResult.Branch
	}
	var bm *types.BranchMapping
	if params.BranchMapping != nil && params.BranchMapping.LocalBranch != "" && params.BranchMapping.RemoteBranch != "" {
		cp := *params.BranchMapping
		bm = &cp
	}
	newRepo := types.Repo{
		Name: filepath.Base(repoPath), Path: repoPath, Origin: originURL,
		Upstream: upstream, Branch: branch, BranchMapping: bm,
		Status: types.RepoStatusUnconfigured,
	}
	if err := a.deps.Store.Add(newRepo); err != nil {
		return types.AddData{}, fmt.Errorf("add repo: %w", err)
	}
	saved, ok := a.deps.Store.GetByName(newRepo.Name)
	if !ok {
		return types.AddData{}, fmt.Errorf("add: failed to retrieve saved repo %q", newRepo.Name)
	}
	return types.AddData{Repo: saved}, nil
}

func autoDetectUpstream(ctx context.Context, cfg *config.Config, originURL string) string {
	if originURL == "" || !github.IsGitHubURL(originURL) {
		return ""
	}
	tok := ""
	if cfg != nil {
		tok = cfg.GitHub.Token
	}
	c := github.NewClient(tok)
	o, r, err := github.ParseRepoURL(originURL)
	if err != nil {
		return ""
	}
	res, err := c.DetectFork(ctx, o, r)
	if err != nil || !res.IsFork || res.UpstreamURL == "" {
		return ""
	}
	return res.UpstreamURL
}

// RemoveResult mirrors the {removed string} shape.
type RemoveResult struct {
	Removed string `json:"removed"`
}

func (a *App) RemoveRepo(name string) (RemoveResult, error) {
	if a.deps == nil {
		return RemoveResult{}, nil
	}
	r2, ok := a.deps.Store.GetByName(name)
	if !ok {
		return RemoveResult{}, fmt.Errorf("repository %q not found", name)
	}
	if err := a.deps.Store.Remove(r2.ID); err != nil {
		return RemoveResult{}, fmt.Errorf("remove repo: %w", err)
	}
	// Clean up history records.
	h := a.deps.HistStore
	closeH := false
	if h == nil {
		hs, err := history.NewStore(a.deps.ConfigDir())
		if err == nil {
			h = hs
			closeH = true
		}
	}
	if h != nil {
		h.ClearByRepo(r2.ID)
		if closeH {
			h.Close()
		}
	}
	return RemoveResult{Removed: r2.Name}, nil
}

// DiffResult mirrors the {success,diff?,error?} shape.
type DiffResult struct {
	Success bool   `json:"success"`
	Diff    string `json:"diff,omitempty"`
	Error   string `json:"error,omitempty"`
}

func (a *App) RepoDiff(name string) (DiffResult, error) {
	if a.deps == nil {
		return DiffResult{}, nil
	}
	r2, ok := a.deps.Store.GetByName(name)
	if !ok {
		return DiffResult{}, fmt.Errorf("repo %q not found", name)
	}
	diff, err := a.deps.GitOps.Diff(a.ctx, r2.Path)
	if err != nil {
		return DiffResult{Success: false, Error: err.Error()}, nil
	}
	return DiffResult{Success: true, Diff: string(diff)}, nil
}

// ---------------------------------------------------------------------------
// Resolve
// ---------------------------------------------------------------------------

type ResolveRequest struct {
	Mode      string `json:"mode,omitempty"`
	Agent     string `json:"agent,omitempty"`
	NoConfirm bool   `json:"noConfirm,omitempty"`
	Manual    bool   `json:"manual,omitempty"`
	Retry     bool   `json:"retry,omitempty"`
}

func (a *App) Resolve(name string, req ResolveRequest) (types.ResolveData, error) {
	if a.deps == nil {
		return types.ResolveData{}, nil
	}
	r2, ok := a.deps.Store.GetByName(name)
	if !ok {
		return types.ResolveData{}, fmt.Errorf("repository %q not found", name)
	}
	switch req.Mode {
	case "prepare":
		repo, err := a.deps.Resolve.Prepare(r2)
		if err != nil {
			return types.ResolveData{}, err
		}
		return types.ResolveData{RepoID: repo.ID}, nil
	case "accept":
		repo, result, err := workflow.AcceptCommit(a.ctx, r2, a.deps.Store, a.deps.GitOps, a.deps.Cfg, a.deps.ConfigDir(), a.deps.HistStore, req.Manual, req.Retry)
		_ = result
		if err != nil && !result.Success {
			return types.ResolveData{RepoID: repo.ID}, nil
		}
		return types.ResolveData{RepoID: repo.ID}, nil
	case "reject":
		repo, err := a.deps.Resolve.Reject(a.ctx, r2)
		if err != nil {
			return types.ResolveData{}, err
		}
		return types.ResolveData{RepoID: repo.ID}, nil
	default:
		return a.resolveAgent(r2, req)
	}
}

func (a *App) resolveAgent(r2 types.Repo, req ResolveRequest) (types.ResolveData, error) {
	if !isConflictRelated(r2.Status) {
		return types.ResolveData{RepoID: r2.ID}, nil
	}
	paths := a.deps.GitOps.DetectConflicts(a.ctx, r2.Path)
	if len(paths) == 0 {
		return types.ResolveData{RepoID: r2.ID}, nil
	}
	cfg := a.deps.Cfg
	provider, err := resolveAgentProvider(cfg, req.Agent)
	if err != nil {
		return types.ResolveData{}, err
	}
	sessStore := session.NewSessionStore(a.cfgMgr.ConfigDir() + "/sessions")
	sessMgr := session.NewManager(sessStore, provider)
	resolver := respkg.NewResolver(a.deps.GitOps, a.deps.Store, cfg, a.cfgMgr, sessMgr)
	timeout := config.AgentTimeout(cfg)
	strategy := config.ResolveStrategyOrDefault(cfg)
	ctx, cancel := context.WithTimeout(a.ctx, timeout)
	defer cancel()
	res, err := resolver.ResolveWithAgent(ctx, r2, strategy, nil)
	if err != nil {
		return types.ResolveData{}, err
	}
	if len(res.Unresolved) > 0 {
		cf := make([]types.ConflictFile, len(res.Unresolved))
		for i, p := range res.Unresolved {
			cf[i] = types.ConflictFile{Path: p}
		}
		return types.ResolveData{RepoID: res.Repo.ID, Conflicts: cf, AgentResult: agentResultToTypes(res.AgentResult)}, nil
	}
	r2 = res.Repo
	if req.NoConfirm || (cfg != nil && !cfg.Agent.ConfirmBeforeCommit) {
		if _, err := workflow.FinalizeCommit(ctx, r2, a.deps.Store, a.deps.GitOps, cfg, a.cfgMgr.ConfigDir(), a.deps.HistStore, workflow.CommitParams{
			CommitMsg: types.CommitMsgAgentResolved, RecordHistory: true, SilentOutput: true,
		}); err != nil {
			logger.Warn("resolve: auto-commit failed", "repo", r2.Name, "error", err)
		}
	}
	return types.ResolveData{RepoID: r2.ID, AgentResult: agentResultToTypes(res.AgentResult)}, nil
}

func isConflictRelated(s types.RepoStatus) bool {
	return s == types.RepoStatusConflict || s == types.RepoStatusResolved ||
		s == types.RepoStatusResolving || s == types.RepoStatusWaiting
}

func resolveAgentProvider(cfg *config.Config, requested string) (agent.AgentProvider, error) {
	preferred := ""
	if cfg != nil {
		preferred = cfg.Agent.Preferred
	}
	return agent.ResolveProvider(preferred, requested)
}

// ---------------------------------------------------------------------------
// Resolve streaming via Wails Events
// ---------------------------------------------------------------------------

// ResolveStreamStart kicks off the agent resolve for a repo in a background
// goroutine, emitting Wails Events as the agent produces output:
//   - "resolve:tick:<name>" — agent is alive, frontend should re-read disk log
//   - "resolve:done:<name>" — resolve finished (carries ResolveData payload)
//   - "resolve:error:<name>" — resolve failed (carries error string)
//
// The disk NDJSON log remains the single source of truth for event content;
// the Wails Events are push notifications that trigger the frontend to
// re-read the disk log (same architecture as the old WebSocket path).
func (a *App) ResolveStreamStart(name string, agentName string, noConfirm bool) {
	if a.deps == nil {
		runtime.EventsEmit(a.ctx, "resolve:error:"+name, "engine not ready")
		return
	}
	go a.runResolveStream(name, agentName, noConfirm)
}

func (a *App) runResolveStream(name, agentName string, noConfirm bool) {
	r2, ok := a.deps.Store.GetByName(name)
	if !ok {
		runtime.EventsEmit(a.ctx, "resolve:error:"+name, fmt.Sprintf("repository %q not found", name))
		return
	}
	if !isConflictRelated(r2.Status) {
		runtime.EventsEmit(a.ctx, "resolve:done:"+name, types.ResolveData{RepoID: r2.ID})
		return
	}
	paths := a.deps.GitOps.DetectConflicts(a.ctx, r2.Path)
	if len(paths) == 0 {
		runtime.EventsEmit(a.ctx, "resolve:done:"+name, types.ResolveData{RepoID: r2.ID})
		return
	}

	cfg := a.deps.Cfg
	provider, err := resolveAgentProvider(cfg, agentName)
	if err != nil {
		runtime.EventsEmit(a.ctx, "resolve:error:"+name, err.Error())
		return
	}
	sessStore := session.NewSessionStore(a.cfgMgr.ConfigDir() + "/sessions")
	sessMgr := session.NewManager(sessStore, provider)
	resolver := respkg.NewResolver(a.deps.GitOps, a.deps.Store, cfg, a.cfgMgr, sessMgr)
	timeout := config.AgentTimeout(cfg)
	strategy := config.ResolveStrategyOrDefault(cfg)

	// Emit tick on every agent event so the frontend re-reads the disk log.
	tickSink := func(_ agent.StreamEvent) {
		runtime.EventsEmit(a.ctx, "resolve:tick:"+name, name)
	}

	// Build stream writer that fans out to disk log + tick sink.
	resolveSessionID := ""
	if step := findAgentResolveStep(r2); step != nil {
		resolveSessionID = step.ResolveSessionID
	}
	streamWriter, closeLog := buildStreamEmitter(a.cfgMgr.ConfigDir(), name, resolveSessionID, tickSink)
	defer closeLog()

	// Set repo to resolving state.
	r2.Status = types.RepoStatusResolving
	if err := a.deps.Store.Update(r2); err != nil {
		logger.Warn("resolve stream: failed to update repo", "repo", r2.Name, "error", err)
	}

	ctx, cancel := context.WithTimeout(a.ctx, timeout)
	defer cancel()

	res, err := resolver.ResolveWithAgent(ctx, r2, strategy, streamWriter)
	if err != nil {
		if streamWriter != nil {
			_ = streamWriter.WriteEvent(agent.StreamEvent{
				Type: agent.StreamEventError, Data: fmt.Sprintf("agent resolve failed: %v", err),
				Timestamp: time.Now().UTC(), Success: false,
			})
		}
		runtime.EventsEmit(a.ctx, "resolve:error:"+name, err.Error())
		return
	}

	r2 = res.Repo
	data := types.ResolveData{RepoID: r2.ID, AgentResult: agentResultToTypes(res.AgentResult)}

	if len(res.Unresolved) > 0 {
		cf := make([]types.ConflictFile, len(res.Unresolved))
		for i, p := range res.Unresolved {
			cf[i] = types.ConflictFile{Path: p}
		}
		data.Conflicts = cf
		if streamWriter != nil {
			_ = streamWriter.WriteEvent(agent.StreamEvent{
				Type: agent.StreamEventDone,
				Data: fmt.Sprintf("agent left %d unresolved conflicts", len(res.Unresolved)),
				Success: false, Timestamp: time.Now().UTC(),
			})
		}
		runtime.EventsEmit(a.ctx, "resolve:done:"+name, data)
		return
	}

	if noConfirm || (cfg != nil && !cfg.Agent.ConfirmBeforeCommit) {
		if _, err := workflow.FinalizeCommit(ctx, r2, a.deps.Store, a.deps.GitOps, cfg, a.cfgMgr.ConfigDir(), a.deps.HistStore, workflow.CommitParams{
			CommitMsg: types.CommitMsgAgentResolved, RecordHistory: true, SilentOutput: true,
		}); err != nil {
			logger.Warn("resolve stream: auto-commit failed", "repo", r2.Name, "error", err)
		}
		if streamWriter != nil {
			_ = streamWriter.WriteEvent(agent.StreamEvent{
				Type: agent.StreamEventDone, Success: true, Timestamp: time.Now().UTC(),
			})
		}
	} else {
		// Wait-for-confirmation: transition to resolved/waiting.
		if r2.Workflow == nil {
			r2.Workflow = workflow.NewWorkflowFromRepo(r2)
		}
		workflow.TransitionAgentResolved(r2.Workflow, res.AgentResult.AgentName)
		r2.Status = types.RepoStatusResolved
		r2.ErrorMessage = ""
		if err := a.deps.Store.Update(r2); err != nil {
			logger.Error("resolve stream: failed to update repo after resolution", "repo", r2.Name, "error", err)
		}
		if streamWriter != nil {
			_ = streamWriter.WriteEvent(agent.DoneEventFromResult(res.AgentResult))
		}
	}

	runtime.EventsEmit(a.ctx, "resolve:done:"+name, data)
}

// findAgentResolveStep extracts the resolveSessionID from the workflow.
func findAgentResolveStep(r types.Repo) *types.WorkflowStepRecord {
	if r.Workflow == nil {
		return nil
	}
	for i := range r.Workflow.Steps {
		if r.Workflow.Steps[i].Step == types.StepAgentResolve {
			return &r.Workflow.Steps[i]
		}
	}
	return nil
}

// buildStreamEmitter constructs a stream writer that fans events to the disk
// log and a live tick callback. Mirrors buildResolveStreamWriter from the old
// handlers_resolve.go.
func buildStreamEmitter(configDir, repoName, sessionID string, tick func(agent.StreamEvent)) (*agent.StreamWriter, func()) {
	diskWriter, closeLog := agent.NewResolveLogWriter(configDir, repoName, sessionID)
	if tick == nil {
		return diskWriter, closeLog
	}
	writers := []*agent.StreamWriter{
		agent.NewStreamWriter(&tickIoWriter{fn: tick}),
		diskWriter,
	}
	msw := agent.NewMultiStreamWriter(writers...)
	return msw.StreamWriter(), closeLog
}

// tickIoWriter forwards each encoded StreamEvent line to the tick callback.
type tickIoWriter struct{ fn func(agent.StreamEvent) }

func (t *tickIoWriter) Write(p []byte) (int, error) {
	var ev agent.StreamEvent
	if err := json.Unmarshal(p, &ev); err != nil {
		t.fn(agent.StreamEvent{Type: agent.StreamEventStdout, Data: trimRight(p), Timestamp: time.Now().UTC()})
		return len(p), nil
	}
	t.fn(ev)
	return len(p), nil
}

func trimRight(p []byte) string {
	s := string(p)
	for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == '\r') {
		s = s[:len(s)-1]
	}
	return s
}

func agentResultToTypes(r *agent.AgentResult) *types.AgentResolveResult {
	if r == nil {
		return nil
	}
	return &types.AgentResolveResult{
		Success: r.Success, ResolvedFiles: r.ResolvedFiles, Diff: r.Diff,
		Summary: r.Summary, SessionID: r.SessionID, AgentName: r.AgentName,
	}
}

// ---------------------------------------------------------------------------
// Agent / History / Config / Post-sync / Summarize reader
// ---------------------------------------------------------------------------

type AgentLogResult struct {
	Events    []agent.StreamEvent `json:"events"`
	IsRunning bool                `json:"isRunning"`
}

func (a *App) ReadAgentLog(repoName, sessionID string) (AgentLogResult, error) {
	if a.deps == nil {
		return AgentLogResult{Events: []agent.StreamEvent{}, IsRunning: false}, nil
	}
	p, err := agent.LogFile(a.deps.ConfigDir(), repoName, sessionID)
	if err != nil || p == "" {
		return AgentLogResult{Events: []agent.StreamEvent{}, IsRunning: false}, nil
	}
	events, err := agent.ReadLogFile(p)
	if err != nil || events == nil {
		events = []agent.StreamEvent{}
	}
	run := false
	if len(events) > 0 {
		last := events[len(events)-1]
		run = last.Type != agent.StreamEventDone && last.Type != agent.StreamEventError
	}
	return AgentLogResult{Events: events, IsRunning: run}, nil
}

func (a *App) AgentList() (types.AgentListData, error) {
	if a.deps == nil {
		return types.AgentListData{}, nil
	}
	return agentListFromDeps(a.deps), nil
}

func agentListFromDeps(d *app.Deps) types.AgentListData {
	reg := agent.NewRegistry("")
	agents := reg.Discover()
	all := reg.ListAll()
	pref := ""
	if d.Cfg != nil {
		pref = d.Cfg.Agent.Preferred
	}
	if pref != "" {
		found := false
		for _, a := range agents {
			if a.Name == pref {
				found = true
				break
			}
		}
		if !found {
			pref = ""
		}
	}
	if pref == "" {
		for _, a := range agents {
			if a.Installed {
				pref = a.Name
				break
			}
		}
	}
	return types.AgentListData{Agents: all, Preferred: pref}
}

func (a *App) History(repoName string, limit int) (types.HistoryData, error) {
	if a.deps == nil || a.deps.HistStore == nil {
		return types.HistoryData{}, nil
	}
	if limit <= 0 {
		limit = 20
	}
	h := a.deps.HistStore
	var records []history.Record
	var err error
	if repoName != "" {
		r2, ok := a.deps.Store.GetByName(repoName)
		if !ok {
			return types.HistoryData{}, fmt.Errorf("repository %q not found", repoName)
		}
		records, err = h.ByRepo(r2.ID, limit)
	} else {
		records, err = h.Recent(limit)
	}
	if err != nil {
		return types.HistoryData{}, err
	}
	result := make([]types.SyncHistoryRecord, 0, len(records))
	for _, r := range records {
		result = append(result, types.SyncHistoryRecord{
			ID: r.ID, RepoID: r.RepoID, RepoName: r.RepoName, Status: r.Status,
			CommitsPulled: r.CommitsPulled, ConflictFiles: r.ConflictFiles,
			AgentUsed: r.AgentUsed, ConflictsFound: r.ConflictsFound,
			AutoResolved: r.AutoResolved, ErrorMessage: r.ErrorMessage,
			Summary: r.Summary, SummaryStatus: r.SummaryStatus,
			CreatedAt: r.CreatedAt.Format(time.RFC3339),
		})
	}
	return types.HistoryData{Records: result}, nil
}

// ConfigResult mirrors the engine config for the frontend.
func (a *App) ConfigGet() (config.Config, error) {
	if a.deps == nil || a.deps.Cfg == nil {
		return config.Config{}, nil
	}
	return *a.deps.Cfg, nil
}

type ConfigSetReq struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

func (a *App) ConfigSet(key string, value string) (map[string]interface{}, error) {
	if a.deps == nil {
		return nil, fmt.Errorf("engine not ready")
	}
	if err := a.deps.CfgMgr.Set(key, value); err != nil {
		return nil, err
	}
	v, err := a.deps.CfgMgr.Get(key)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"key": key, "value": v}, nil
}

// ---------------------------------------------------------------------------
// Agent sessions / cleanup / reset
// ---------------------------------------------------------------------------

func (a *App) AgentSessions() (types.AgentSessionsData, error) {
	if a.deps == nil {
		return types.AgentSessionsData{}, nil
	}
	store := session.NewSessionStore(a.deps.SessionsDir())
	mgr := session.NewManager(store, nil)
	infos, err := mgr.ListSessionsAsInfo()
	if err != nil {
		return types.AgentSessionsData{}, fmt.Errorf("list sessions: %w", err)
	}
	for i := range infos {
		if repo, ok := a.deps.Store.Get(infos[i].RepoID); ok {
			infos[i].RepoName = repo.Name
		} else {
			infos[i].RepoName = infos[i].RepoID
		}
	}
	return types.AgentSessionsData{Sessions: infos}, nil
}

type AgentCleanupResult struct {
	Removed int `json:"removed"`
}

func (a *App) AgentCleanup() (AgentCleanupResult, error) {
	if a.deps == nil {
		return AgentCleanupResult{}, nil
	}
	store := session.NewSessionStore(a.deps.SessionsDir())
	mgr := session.NewManager(store, nil)
	count, err := mgr.CleanupFailed()
	if err != nil {
		return AgentCleanupResult{}, fmt.Errorf("cleanup sessions: %w", err)
	}
	return AgentCleanupResult{Removed: count}, nil
}

func (a *App) AgentReset(repoID string) (types.AgentResetData, error) {
	if a.deps == nil {
		return types.AgentResetData{}, nil
	}
	store := session.NewSessionStore(a.deps.SessionsDir())
	mgr := session.NewManager(store, nil)
	cleared, err := mgr.ResetSession(a.ctx, repoID)
	if err != nil {
		return types.AgentResetData{}, fmt.Errorf("reset session: %w", err)
	}
	return types.AgentResetData{RepoID: repoID, Cleared: cleared}, nil
}

// ---------------------------------------------------------------------------
// History cleanup
// ---------------------------------------------------------------------------

type HistoryCleanupReq struct {
	Repo     string `json:"repo,omitempty"`
	KeepDays int    `json:"keepDays,omitempty"`
}

type HistoryCleanupResult struct {
	Message string `json:"message"`
}

func (a *App) HistoryCleanup(req HistoryCleanupReq) (HistoryCleanupResult, error) {
	if a.deps == nil || a.deps.HistStore == nil {
		return HistoryCleanupResult{}, fmt.Errorf("history store unavailable")
	}
	h := a.deps.HistStore
	var n int64
	var err error
	var msg string
	if req.Repo != "" {
		r2, ok := a.deps.Store.GetByName(req.Repo)
		if !ok {
			return HistoryCleanupResult{}, fmt.Errorf("repository %q not found", req.Repo)
		}
		n, err = h.ClearByRepo(r2.ID)
		msg = fmt.Sprintf("Cleared %d history record(s) for repository %q", n, req.Repo)
	} else if req.KeepDays > 0 {
		n, err = h.ClearBefore(time.Now().AddDate(0, 0, -req.KeepDays))
		msg = fmt.Sprintf("Cleared %d history record(s) older than %d days", n, req.KeepDays)
	} else {
		n, err = h.ClearAll()
		msg = fmt.Sprintf("Cleared %d history record(s)", n)
	}
	if err != nil {
		return HistoryCleanupResult{}, fmt.Errorf("cleanup failed: %w", err)
	}
	if a.deps.Bus != nil {
		a.deps.Bus.Publish(eventbus.Event{Type: eventbus.EventHistoryChanged})
	}
	return HistoryCleanupResult{Message: msg}, nil
}

// ---------------------------------------------------------------------------
// Post-sync commands
// ---------------------------------------------------------------------------

type PostSyncAddReq struct {
	Name string `json:"name"`
	Cmd  string `json:"cmd"`
}

type PostSyncRemoveReq struct {
	ID string `json:"id"`
}

func (a *App) PostSyncList(repoName string) (types.PostSyncCommandsData, error) {
	if a.deps == nil {
		return types.PostSyncCommandsData{Commands: []types.PostSyncCommand{}}, nil
	}
	r2, ok := a.deps.Store.GetByName(repoName)
	if !ok {
		return types.PostSyncCommandsData{}, fmt.Errorf("repo %q not found", repoName)
	}
	cmds := r2.PostSyncCommands
	if cmds == nil {
		cmds = []types.PostSyncCommand{}
	}
	return types.PostSyncCommandsData{Commands: cmds}, nil
}

func (a *App) PostSyncAdd(repoName string, req PostSyncAddReq) (types.PostSyncCommandsData, error) {
	if a.deps == nil {
		return types.PostSyncCommandsData{}, fmt.Errorf("engine not ready")
	}
	r2, ok := a.deps.Store.GetByName(repoName)
	if !ok {
		return types.PostSyncCommandsData{}, fmt.Errorf("repo %q not found", repoName)
	}
	r2.PostSyncCommands = append(r2.PostSyncCommands, types.PostSyncCommand{
		ID: uuid(), Name: req.Name, Cmd: req.Cmd,
	})
	if err := a.deps.Store.Update(r2); err != nil {
		return types.PostSyncCommandsData{}, err
	}
	return types.PostSyncCommandsData{Commands: r2.PostSyncCommands}, nil
}

func (a *App) PostSyncRemove(repoName string, req PostSyncRemoveReq) (types.PostSyncCommandsData, error) {
	if a.deps == nil {
		return types.PostSyncCommandsData{}, nil
	}
	r2, ok := a.deps.Store.GetByName(repoName)
	if !ok {
		return types.PostSyncCommandsData{}, fmt.Errorf("repo %q not found", repoName)
	}
	filtered := make([]types.PostSyncCommand, 0, len(r2.PostSyncCommands))
	for _, c := range r2.PostSyncCommands {
		if c.ID != req.ID {
			filtered = append(filtered, c)
		}
	}
	r2.PostSyncCommands = filtered
	if err := a.deps.Store.Update(r2); err != nil {
		return types.PostSyncCommandsData{}, err
	}
	return types.PostSyncCommandsData{Commands: filtered}, nil
}

// ---------------------------------------------------------------------------
// Summarize
// ---------------------------------------------------------------------------

type SummarizeReq struct {
	Retry bool `json:"retry,omitempty"`
}

type SummarizeResult struct {
	HistoryID     int64  `json:"historyId"`
	RepoName      string `json:"repoName"`
	Summary       string `json:"summary"`
	SummaryStatus string `json:"summaryStatus"`
}

func (a *App) Summarize(repoName string, req SummarizeReq) (SummarizeResult, error) {
	if a.deps == nil || a.deps.HistStore == nil {
		return SummarizeResult{}, fmt.Errorf("history store unavailable")
	}
	r2, ok := a.deps.Store.GetByName(repoName)
	if !ok {
		return SummarizeResult{}, fmt.Errorf("repository %q not found", repoName)
	}
	record, err := a.deps.HistStore.LatestByRepo(r2.ID)
	if err != nil {
		return SummarizeResult{}, fmt.Errorf("no sync history found for %q", repoName)
	}
	if !req.Retry && record.SummaryStatus == string(types.SummaryStatusDone) {
		return SummarizeResult{
			HistoryID: record.ID, RepoName: record.RepoName,
			Summary: record.Summary, SummaryStatus: record.SummaryStatus,
		}, nil
	}
	summary, err := app.GenerateSummary(a.ctx, a.deps.Cfg, a.deps.HistStore, a.deps.Bus, record, r2)
	if err != nil {
		return SummarizeResult{}, err
	}
	return SummarizeResult{
		HistoryID: record.ID, RepoName: record.RepoName,
		Summary: summary, SummaryStatus: string(types.SummaryStatusDone),
	}, nil
}

func (a *App) SummarizeRetry(repoName string) (SummarizeResult, error) {
	return a.Summarize(repoName, SummarizeReq{Retry: true})
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func uuid() string {
	b := make([]byte, 16)
	for i := range b {
		b[i] = byte(time.Now().UnixNano() >> uint(i*8))
	}
	return fmt.Sprintf("%x", b)
}
