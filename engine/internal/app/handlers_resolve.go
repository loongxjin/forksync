package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/loongxjin/forksync/engine/internal/agent"
	"github.com/loongxjin/forksync/engine/internal/agent/session"
	"github.com/loongxjin/forksync/engine/internal/config"
	"github.com/loongxjin/forksync/engine/internal/git"
	"github.com/loongxjin/forksync/engine/internal/logger"
	"github.com/loongxjin/forksync/engine/internal/repo"
	respkg "github.com/loongxjin/forksync/engine/internal/resolve"
	"github.com/loongxjin/forksync/engine/internal/workflow"
	"github.com/loongxjin/forksync/engine/pkg/types"
)

// registerResolveRoutes wires the resolve + agent-log endpoints.
func (s *Server) registerResolveRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /repos/{name}/resolve", s.handleResolve)
	mux.HandleFunc("GET /repos/{name}/agent-log", s.handleReadAgentLog)
	// resolveStream is a WebSocket upgrade handled by gorilla/websocket —
	// registered in handlers_stream.go.
}

// resolveRequest is the body for POST /repos/{name}/resolve.
type resolveRequest struct {
	Mode      string `json:"mode,omitempty"`      // prepare|accept|reject|agent (default agent)
	Agent     string `json:"agent,omitempty"`     // --agent
	NoConfirm bool   `json:"noConfirm,omitempty"` // --no-confirm
	Manual    bool   `json:"manual,omitempty"`    // --accept --manual
	Retry     bool   `json:"retry,omitempty"`     // --accept --retry
}

func (s *Server) handleResolve(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	req, ok := decodeJSON[resolveRequest](w, r)
	if !ok {
		return
	}

	r2, ok := s.deps.Store.GetByName(name)
	if !ok {
		writeErr[types.ResolveData](w, fmt.Errorf("repository %q not found", name))
		return
	}

	switch req.Mode {
	case "prepare":
		s.resolvePrepare(w, r, r2)
	case "accept":
		s.resolveAccept(w, r, r2, req.Manual, req.Retry)
	case "reject":
		s.resolveReject(w, r, r2)
	default:
		s.resolveAgent(w, r, r2, req)
	}
}

// workflowContinueResult mirrors cmd/workflow.go workflowContinueResult.
type workflowContinueResult struct {
	RepoID   string              `json:"repoId"`
	RepoName string              `json:"repoName"`
	Status   types.RepoStatus    `json:"status"`
	Workflow *types.SyncWorkflow `json:"workflow,omitempty"`
}

func (s *Server) resolvePrepare(w http.ResponseWriter, r *http.Request, r2 types.Repo) {
	repo, err := s.deps.Resolve.Prepare(r2)
	if err != nil {
		writeErr[workflowContinueResult](w, err)
		return
	}
	writeOK(w, workflowContinueResult{
		RepoID:   repo.ID,
		RepoName: repo.Name,
		Status:   repo.Status,
		Workflow: repo.Workflow,
	})
}

func (s *Server) resolveAccept(w http.ResponseWriter, r *http.Request, r2 types.Repo, manual, retry bool) {
	repo, result, err := workflow.AcceptCommit(r.Context(), r2, s.deps.Store, s.deps.GitOps, s.deps.Cfg, s.deps.ConfigDir(), manual, retry)

	// Conflicts still unresolved — mirrors cmd/resolve.go runResolveAccept.
	if err != nil && !result.Success {
		writeOK(w, types.AcceptData{RepoID: repo.ID, Resolved: false})
		return
	}

	if result.Success && err == nil {
		// accept-no-merge short-circuit (no MERGE_HEAD, already up to date).
		mergeHead := filepath.Join(repo.Path, ".git", "MERGE_HEAD")
		if _, serr := os.Stat(mergeHead); serr != nil && repo.Status == types.RepoStatusUpToDate {
			writeOK(w, types.AcceptData{RepoID: repo.ID, Resolved: true})
			return
		}
	}

	writeOK(w, types.AcceptData{RepoID: repo.ID, Resolved: true})
}

func (s *Server) resolveReject(w http.ResponseWriter, r *http.Request, r2 types.Repo) {
	repo, err := s.deps.Resolve.Reject(r.Context(), r2)
	if err != nil {
		writeErr[types.RejectData](w, err)
		return
	}
	writeOK(w, types.RejectData{RepoID: repo.ID, RolledBack: true})
}

// resolveAgent is the non-streaming agent resolve path (mode == "agent",
// without a WebSocket). It mirrors cmd/resolve.go resolveWithAgent but uses
// request-context cancellation in place of the SIGINT/SIGTERM signal guard.
func (s *Server) resolveAgent(w http.ResponseWriter, r *http.Request, r2 types.Repo, req resolveRequest) {
	// Not in a conflict-related state — short-circuit (cmd/resolve.go guard).
	if r2.Status != types.RepoStatusConflict && r2.Status != types.RepoStatusResolved &&
		r2.Status != types.RepoStatusResolving && r2.Status != types.RepoStatusWaiting {
		writeOK(w, types.AcceptData{RepoID: r2.ID, Resolved: true})
		return
	}

	conflictPaths := s.deps.GitOps.DetectConflicts(r.Context(), r2.Path)
	if len(conflictPaths) == 0 {
		writeOK(w, types.AcceptData{RepoID: r2.ID, Resolved: true})
		return
	}

	outcome, err := s.runResolveWithAgent(r.Context(), r2, req, nil /* no stream writer */)
	if err != nil {
		writeErr[types.ResolveData](w, err)
		return
	}
	writeOK(w, outcome.data)
}

// resolveOutcome bundles the typed response + final repo state from an agent run.
type resolveOutcome struct {
	data types.ResolveData
	repo types.Repo
}

// runResolveWithAgent executes the agent resolve flow. streamSink, when
// non-nil, receives every agent StreamEvent (used by the WebSocket handler).
// It replicates cmd/resolve.go resolveWithAgent:
//   - resolve timeout from cfg.Agent.Timeout (default 10m)
//   - resolved atomic guard: if the agent did not finish, defer Reject to roll
//     the repo back out of RepoStatusResolving
//   - ctx cancellation (client disconnect / WS close) replaces the SIGINT guard
//   - auto-commit when --no-confirm OR cfg.Agent.ConfirmBeforeCommit==false
func (s *Server) runResolveWithAgent(ctx context.Context, r types.Repo, req resolveRequest, streamSink func(agent.StreamEvent)) (*resolveOutcome, error) {
	cfg := s.deps.Cfg
	store := s.deps.Store

	provider, err := resolveAgentProvider(cfg, req.Agent)
	if err != nil {
		return nil, err
	}

	// Fresh session manager scoped to this request (matches cmd/resolve.go
	// which builds its own instead of reusing the shared one).
	cfgMgr := s.deps.CfgMgr
	sessionStore := session.NewSessionStore(filepath.Join(cfgMgr.ConfigDir(), "sessions"))
	sessionMgr := session.NewManager(sessionStore, provider)

	resolver := respkg.NewResolver(s.deps.GitOps, store, cfg, cfgMgr, sessionMgr)

	timeout := config.AgentTimeout(cfg)
	resolveStrategy := config.ResolveStrategyOrDefault(cfg)

	var resolved atomic.Bool

	// Rollback guard: if the agent never finished (ctx cancel or error), roll
	// the repo out of RepoStatusResolving so it isn't stuck.
	//
	// IMPORTANT: this defer runs AFTER ctx has been cancelled (by the timeout
	// below or by a WS client disconnect). gitOps.AbortMerge honors the passed
	// context, so we must run Reject on a fresh background context with its own
	// generous deadline — otherwise the rollback itself is cancelled and the
	// repo is left stuck in RepoStatusResolving forever. This mirrors the old
	// cmd/resolve.go which used the parent cmd.Context() (process lifetime)
	// for the rollback, NOT the resolve timeout ctx.
	defer func() {
		if resolved.Load() {
			return
		}
		rollbackCtx, rollbackCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer rollbackCancel()
		if _, rerr := resolver.Reject(rollbackCtx, r); rerr != nil {
			logger.Warn("resolve: rollback failed", "repo", r.Name, "error", rerr)
		}
	}()

	r.Status = types.RepoStatusResolving
	updateRepoWithLog(r, store, "resolving")

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Build the stream writer fan-out: disk log (+ optional live sink).
	// Name the log by the resolve session id stamped on the agent_resolve step
	// (by Prepare) so the frontend can locate it precisely by session.
	resolveSessionID := ""
	if step := workflow.FindStep(r.Workflow, types.StepAgentResolve); step != nil {
		resolveSessionID = step.ResolveSessionID
	}
	streamWriter, closeLogWriter := buildResolveStreamWriter(cfgMgr.ConfigDir(), r.Name, resolveSessionID, streamSink)
	defer closeLogWriter()

	res, err := resolver.ResolveWithAgent(ctx, r, resolveStrategy, streamWriter)
	if err != nil {
		if streamWriter != nil {
			_ = streamWriter.WriteEvent(agent.StreamEvent{
				Type:      agent.StreamEventError,
				Data:      fmt.Sprintf("agent resolve failed: %v", err),
				Timestamp: time.Now().UTC(),
				Success:   false,
			})
		}
		resolved.Store(true)
		return nil, fmt.Errorf("agent resolve: %w", err)
	}

	if len(res.Unresolved) > 0 {
		resolved.Store(true)
		// Emit a final done=false event so stream consumers stop, mirroring
		// cmd/resolve.go handleUnresolvedConflicts behavior.
		if streamWriter != nil {
			_ = streamWriter.WriteEvent(agent.StreamEvent{
				Type:      agent.StreamEventDone,
				Data:      fmt.Sprintf("agent left %d unresolved conflicts", len(res.Unresolved)),
				Success:   false,
				Timestamp: time.Now().UTC(),
			})
		}
		return &resolveOutcome{
			data: types.ResolveData{
				RepoID:      res.Repo.ID,
				Conflicts:   toConflictFiles(res.Unresolved),
				AgentResult: agentResultToTypes(res.AgentResult),
			},
			repo: res.Repo,
		}, nil
	}

	r = res.Repo
	resolved.Store(true)

	confirmBeforeCommit := true
	if cfg != nil {
		confirmBeforeCommit = cfg.Agent.ConfirmBeforeCommit
	}

	if req.NoConfirm || !confirmBeforeCommit {
		// Auto-commit path (mirrors finalizeCommitWithWorkflow).
		if commitErr := finalizeCommitSilent(ctx, r, store, s.deps.GitOps, cfg, cfgMgr); commitErr != nil {
			logger.Warn("resolve: auto-commit failed", "repo", r.Name, "error", commitErr)
		}
		// Refresh repo from store (FinalizeCommit updated it).
		if updated, ok := store.Get(r.ID); ok {
			r = updated
		}
		// Emit a terminal done event for stream consumers.
		if streamWriter != nil {
			_ = streamWriter.WriteEvent(agent.StreamEvent{
				Type:      agent.StreamEventDone,
				Success:   true,
				Timestamp: time.Now().UTC(),
			})
		}
		return &resolveOutcome{
			data: types.ResolveData{
				RepoID:      r.ID,
				AgentResult: agentResultToTypes(res.AgentResult),
			},
			repo: r,
		}, nil
	}

	// Wait-for-confirmation path: transition repo to resolved/waiting state
	// so the frontend can show the diff and offer Accept/Reject.
	if r.Workflow == nil {
		r.Workflow = workflow.NewWorkflowFromRepo(r)
	}
	workflow.TransitionAgentResolved(r.Workflow, res.AgentResult.AgentName)
	r.Status = types.RepoStatusResolved
	r.ErrorMessage = ""
	if storeErr := store.Update(r); storeErr != nil {
		logger.Error("resolve: failed to update repo after agent resolution", "repo", r.Name, "error", storeErr)
	}

	if streamWriter != nil {
		_ = streamWriter.WriteEvent(doneEventFromResult(res.AgentResult))
	}

	return &resolveOutcome{
		data: types.ResolveData{
			RepoID:      res.Repo.ID,
			AgentResult: agentResultToTypes(res.AgentResult),
		},
		repo: r,
	}, nil
}

// doneEventFromResult delegates to the shared agent.DoneEventFromResult.
func doneEventFromResult(r *agent.AgentResult) agent.StreamEvent {
	return agent.DoneEventFromResult(r)
}

// resolveAgentProvider extracts cfg.Agent.Preferred and delegates to the shared
// agent.ResolveProvider (single source of truth for agent selection, also used
// by the auto-sync path).
func resolveAgentProvider(cfg *config.Config, requested string) (agent.AgentProvider, error) {
	preferred := ""
	if cfg != nil {
		preferred = cfg.Agent.Preferred
	}
	return agent.ResolveProvider(preferred, requested)
}

// toConflictFiles mirrors cmd/resolve.go toConflictFiles.
func toConflictFiles(paths []string) []types.ConflictFile {
	files := make([]types.ConflictFile, len(paths))
	for i, p := range paths {
		files[i] = types.ConflictFile{Path: p}
	}
	return files
}

// agentResultToTypes mirrors cmd/resolve.go agentResultToTypes.
func agentResultToTypes(r *agent.AgentResult) *types.AgentResolveResult {
	if r == nil {
		return nil
	}
	return &types.AgentResolveResult{
		Success:       r.Success,
		ResolvedFiles: r.ResolvedFiles,
		Diff:          r.Diff,
		Summary:       r.Summary,
		SessionID:     r.SessionID,
		AgentName:     r.AgentName,
	}
}

// buildResolveStreamWriter constructs a stream writer fan-out. When streamSink
// is nil (non-streaming path) it still writes the disk log so readAgentLog can
// replay later — mirroring cmd/resolve.go setupResolveStreamWriter's disk arm.
// When streamSink is non-nil (WebSocket path) events are also pushed live.
//
// The returned cleanup is always safe to defer even if the disk log failed to
// open (lw == nil).
func buildResolveStreamWriter(configDir, repoName, sessionID string, streamSink func(agent.StreamEvent)) (*agent.StreamWriter, func()) {
	// Disk-log arm — shared with the auto-sync path via agent.NewResolveLogWriter.
	// It always returns a non-nil writer (no-op on failure) and a safe cleanup.
	diskWriter, closeLog := agent.NewResolveLogWriter(configDir, repoName, sessionID)

	if streamSink == nil {
		// Non-streaming path: disk log only.
		return diskWriter, closeLog
	}

	// Streaming path: fan out to the live WS sink AND the disk log.
	writers := []*agent.StreamWriter{
		agent.NewStreamWriter(&sinkWriter{fn: streamSink}),
		diskWriter,
	}
	msw := agent.NewMultiStreamWriter(writers...)
	return msw.StreamWriter(), closeLog
}

// sinkWriter is an io.Writer that forwards each full NDJSON line to a callback.
// agent.StreamWriter writes one JSON object + newline per WriteEvent, then
// flushes its internal bufio.Writer — which drives Write() here line-by-line.
type sinkWriter struct {
	fn func(agent.StreamEvent)
}

func (s *sinkWriter) Write(p []byte) (int, error) {
	// Each Write corresponds to one encoded StreamEvent line.
	var ev agent.StreamEvent
	if err := json.Unmarshal(p, &ev); err != nil {
		// Couldn't decode as a StreamEvent — forward the raw bytes as a
		// stdout event so the WS client isn't silently dropped a frame.
		// (Matches the old engine.ts readline path which emitted
		// {t:'stdout', d:line} for non-JSON lines.)
		s.fn(agent.StreamEvent{
			Type:      agent.StreamEventStdout,
			Data:      strings.TrimRight(string(p), "\n"),
			Timestamp: time.Now().UTC(),
		})
		return len(p), nil
	}
	s.fn(ev)
	return len(p), nil
}

// finalizeCommitSilent wraps workflow.FinalizeCommit with the params used by
// the --no-confirm resolve path (RecordHistory + SilentOutput), matching
// cmd/workflow.go finalizeCommitWithWorkflow.
func finalizeCommitSilent(ctx context.Context, r types.Repo, store repo.Store, gitOps git.OperationsProvider, cfg *config.Config, cfgMgr *config.Manager) error {
	_, err := workflow.FinalizeCommit(ctx, r, store, gitOps, cfg, cfgMgr.ConfigDir(), workflow.CommitParams{
		CommitMsg:     types.CommitMsgAgentResolved,
		RecordHistory: true,
		SilentOutput:  true,
	})
	return err
}

// agentLogResult mirrors the {events,isRunning} shape from engine.ts readAgentLog.
type agentLogResult struct {
	Events    []agent.StreamEvent `json:"events"`
	IsRunning bool                `json:"isRunning"`
}

// handleReadAgentLog replays the on-disk agent log for a repo.
// The sessionId query param identifies the exact resolve run's log.
func (s *Server) handleReadAgentLog(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	sid := r.URL.Query().Get("session")
	logPath, logErr := agent.LogFile(s.deps.ConfigDir(), name, sid)

	if logErr != nil || logPath == "" {
		writeBare(w, agentLogResult{Events: []agent.StreamEvent{}, IsRunning: false})
		return
	}
	events, err := agent.ReadLogFile(logPath)
	if err != nil {
		writeBare(w, agentLogResult{Events: []agent.StreamEvent{}, IsRunning: false})
		return
	}
	if events == nil {
		events = []agent.StreamEvent{}
	}
	isRunning := false
	if len(events) > 0 {
		last := events[len(events)-1]
		isRunning = last.Type != agent.StreamEventDone && last.Type != agent.StreamEventError
	}
	writeBare(w, agentLogResult{Events: events, IsRunning: isRunning})
}
