package app

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/loongxjin/forksync/engine/core/agent"
	"github.com/loongxjin/forksync/engine/core/agent/session"
	"github.com/loongxjin/forksync/engine/core/config"
	"github.com/loongxjin/forksync/engine/core/eventbus"
	"github.com/loongxjin/forksync/engine/core/history"
	"github.com/loongxjin/forksync/engine/core/logger"
	"github.com/loongxjin/forksync/engine/core/summarizer"
	"github.com/loongxjin/forksync/engine/pkg/types"
)

// registerMiscRoutes wires agent / history / config / post-sync / summarize.
func (s *Server) registerMiscRoutes(mux *http.ServeMux) {
	// agent
	mux.HandleFunc("GET /agents", s.handleAgentList)
	mux.HandleFunc("GET /agents/sessions", s.handleAgentSessions)
	mux.HandleFunc("POST /agents/cleanup", s.handleAgentCleanup)
	mux.HandleFunc("POST /agents/{name}/reset", s.handleAgentReset)

	// history
	mux.HandleFunc("GET /history", s.handleHistory)
	mux.HandleFunc("POST /history/cleanup", s.handleHistoryCleanup)

	// config
	mux.HandleFunc("GET /config", s.handleConfigGet)
	mux.HandleFunc("PUT /config", s.handleConfigSet)

	// post-sync commands
	mux.HandleFunc("GET /repos/{name}/post-sync", s.handlePostSyncList)
	mux.HandleFunc("POST /repos/{name}/post-sync", s.handlePostSyncAdd)
	mux.HandleFunc("DELETE /repos/{name}/post-sync", s.handlePostSyncRemove)

	// summarize
	mux.HandleFunc("POST /repos/{name}/summarize", s.handleSummarize)
}

// ---------------------------------------------------------------------------
// agent
// ---------------------------------------------------------------------------

func (s *Server) handleAgentList(w http.ResponseWriter, r *http.Request) {
	preferredCfg := ""
	if s.deps.Cfg != nil {
		preferredCfg = s.deps.Cfg.Agent.Preferred
	}
	registry := agent.NewRegistry(preferredCfg)
	agents := registry.Discover()
	allAgents := registry.ListAll()

	// Preferred resolution logic mirrors cmd/agent.go runAgentList.
	preferred := preferredCfg
	if preferred != "" {
		found := false
		for _, a := range agents {
			if a.Name == preferred {
				found = true
				break
			}
		}
		if !found {
			preferred = ""
		}
	}
	if preferred == "" {
		for _, a := range agents {
			if a.Installed {
				preferred = a.Name
				break
			}
		}
	}

	writeOK(w, types.AgentListData{Agents: allAgents, Preferred: preferred})
}

func (s *Server) handleAgentSessions(w http.ResponseWriter, r *http.Request) {
	store := session.NewSessionStore(s.deps.SessionsDir())
	mgr := session.NewManager(store, nil) // provider not needed for listing
	infos, err := mgr.ListSessionsAsInfo()
	if err != nil {
		writeErr[types.AgentSessionsData](w, fmt.Errorf("list sessions: %w", err))
		return
	}

	// Enrich each session with its repository name so the UI can show
	// "which repo" instead of an opaque repoId. Falls back to the repoId
	// when the repo is no longer registered (e.g. removed but session kept).
	for i := range infos {
		if repo, ok := s.deps.Store.Get(infos[i].RepoID); ok {
			infos[i].RepoName = repo.Name
		} else {
			infos[i].RepoName = infos[i].RepoID
		}
	}

	writeOK(w, types.AgentSessionsData{Sessions: infos})
}

// agentCleanupResult mirrors map[string]int{"removed": n}.
type agentCleanupResult struct {
	Removed int `json:"removed"`
}

func (s *Server) handleAgentCleanup(w http.ResponseWriter, r *http.Request) {
	store := session.NewSessionStore(s.deps.SessionsDir())
	mgr := session.NewManager(store, nil)
	count, err := mgr.CleanupFailed()
	if err != nil {
		writeErr[agentCleanupResult](w, fmt.Errorf("cleanup sessions: %w", err))
		return
	}
	writeOK(w, agentCleanupResult{Removed: count})
}

func (s *Server) handleAgentReset(w http.ResponseWriter, r *http.Request) {
	repoID := r.PathValue("name")
	store := session.NewSessionStore(s.deps.SessionsDir())
	mgr := session.NewManager(store, nil)
	cleared, err := mgr.ResetSession(r.Context(), repoID)
	if err != nil {
		writeErr[types.AgentResetData](w, fmt.Errorf("reset session: %w", err))
		return
	}
	writeOK(w, types.AgentResetData{RepoID: repoID, Cleared: cleared})
}

// ---------------------------------------------------------------------------
// history
// ---------------------------------------------------------------------------

func (s *Server) handleHistory(w http.ResponseWriter, r *http.Request) {
	repoName := r.URL.Query().Get("repo")
	limit := 20
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}

	// Reuse the long-lived history store from Deps instead of opening a fresh
	// SQLite connection per request. The frontend polls /history heavily during
	// sync (see HomePage.tsx loadHistory), so per-request NewStore would open and
	// close the DB ~80x/second. Fall back to a one-shot store only if the shared
	// one failed to initialize at boot.
	hStore := s.deps.HistStore
	if hStore == nil {
		hs, err := history.NewStore(s.deps.ConfigDir())
		if err != nil {
			writeErr[types.HistoryData](w, fmt.Errorf("open history store: %w", err))
			return
		}
		defer hs.Close()
		hStore = hs
	}

	var (
		dbRecords []history.Record
		err       error
	)
	if repoName != "" {
		r2, ok := s.deps.Store.GetByName(repoName)
		if !ok {
			writeErr[types.HistoryData](w, fmt.Errorf("repository %q not found", repoName))
			return
		}
		dbRecords, err = hStore.ByRepo(r2.ID, limit)
	} else {
		dbRecords, err = hStore.Recent(limit)
	}
	if err != nil {
		writeErr[types.HistoryData](w, fmt.Errorf("query history: %w", err))
		return
	}

	records := make([]types.SyncHistoryRecord, 0, len(dbRecords))
	for _, rec := range dbRecords {
		records = append(records, types.SyncHistoryRecord{
			ID:             rec.ID,
			RepoID:         rec.RepoID,
			RepoName:       rec.RepoName,
			Status:         rec.Status,
			CommitsPulled:  rec.CommitsPulled,
			ConflictFiles:  rec.ConflictFiles,
			AgentUsed:      rec.AgentUsed,
			ConflictsFound: rec.ConflictsFound,
			AutoResolved:   rec.AutoResolved,
			ErrorMessage:   rec.ErrorMessage,
			Summary:        rec.Summary,
			SummaryStatus:  rec.SummaryStatus,
			CreatedAt:      rec.CreatedAt.Format(time.RFC3339),
		})
	}
	writeOK(w, types.HistoryData{Records: records})
}

type historyCleanupRequest struct {
	Repo     string `json:"repo,omitempty"`
	KeepDays int    `json:"keepDays,omitempty"`
}

// historyCleanupResult mirrors map[string]string{"message": ...}.
type historyCleanupResult struct {
	Message string `json:"message"`
}

func (s *Server) handleHistoryCleanup(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeJSON[historyCleanupRequest](w, r)
	if !ok {
		return
	}

	// Reuse the shared history store; only open a one-shot store if boot init failed.
	hStore := s.deps.HistStore
	if hStore == nil {
		hs, err := history.NewStore(s.deps.ConfigDir())
		if err != nil {
			writeErr[historyCleanupResult](w, fmt.Errorf("open history store: %w", err))
			return
		}
		defer hs.Close()
		hStore = hs
	}

	var (
		n   int64
		err error
		msg string
	)
	if req.Repo != "" {
		r2, ok := s.deps.Store.GetByName(req.Repo)
		if !ok {
			writeErr[historyCleanupResult](w, fmt.Errorf("repository %q not found", req.Repo))
			return
		}
		n, err = hStore.ClearByRepo(r2.ID)
		msg = fmt.Sprintf("Cleared %d history record(s) for repository %q", n, req.Repo)
	} else if req.KeepDays > 0 {
		n, err = hStore.ClearBefore(time.Now().AddDate(0, 0, -req.KeepDays))
		msg = fmt.Sprintf("Cleared %d history record(s) older than %d days", n, req.KeepDays)
	} else {
		n, err = hStore.ClearAll()
		msg = fmt.Sprintf("Cleared %d history record(s)", n)
	}
	if err != nil {
		writeErr[historyCleanupResult](w, fmt.Errorf("cleanup failed: %w", err))
		return
	}
	// History changed — notify /stream/events subscribers so the renderer can
	// refresh its history view without polling.
	if s.deps.Bus != nil {
		s.deps.Bus.Publish(eventbus.Event{Type: eventbus.EventHistoryChanged})
	}
	writeOK(w, historyCleanupResult{Message: msg})
}

// ---------------------------------------------------------------------------
// config
// ---------------------------------------------------------------------------

func (s *Server) handleConfigGet(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.deps.CfgMgr.Load()
	if err != nil {
		writeErr[config.Config](w, fmt.Errorf("load config: %w", err))
		return
	}
	writeOK(w, cfg)
}

type configSetRequest struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// configSetResult mirrors map[string]any{"key","value"}.
type configSetResult struct {
	Key   string      `json:"key"`
	Value interface{} `json:"value"`
}

func (s *Server) handleConfigSet(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeJSON[configSetRequest](w, r)
	if !ok {
		return
	}
	if err := s.deps.CfgMgr.Set(req.Key, req.Value); err != nil {
		writeErr[configSetResult](w, err)
		return
	}
	newValue, err := s.deps.CfgMgr.Get(req.Key)
	if err != nil {
		writeErr[configSetResult](w, err)
		return
	}
	writeOK(w, configSetResult{Key: req.Key, Value: newValue})
}

// ---------------------------------------------------------------------------
// post-sync
// ---------------------------------------------------------------------------

type postSyncAddRequest struct {
	Name string `json:"name"`
	Cmd  string `json:"cmd"`
}

type postSyncRemoveRequest struct {
	ID string `json:"id"`
}

func (s *Server) handlePostSyncList(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	r2, ok := s.deps.Store.GetByName(name)
	if !ok {
		writePostSyncErr(w, fmt.Errorf("repo %q not found", name))
		return
	}
	writeOK(w, normalizePostSync(r2.PostSyncCommands))
}

func (s *Server) handlePostSyncAdd(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	req, ok := decodeJSON[postSyncAddRequest](w, r)
	if !ok {
		return
	}
	r2, ok := s.deps.Store.GetByName(name)
	if !ok {
		writePostSyncErr(w, fmt.Errorf("repo %q not found", name))
		return
	}
	newCmd := types.PostSyncCommand{ID: uuid.New().String(), Name: req.Name, Cmd: req.Cmd}
	r2.PostSyncCommands = append(r2.PostSyncCommands, newCmd)
	if err := s.deps.Store.Update(r2); err != nil {
		writePostSyncErr(w, err)
		return
	}
	writeOK(w, normalizePostSync(r2.PostSyncCommands))
}

func (s *Server) handlePostSyncRemove(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	req, ok := decodeJSON[postSyncRemoveRequest](w, r)
	if !ok {
		return
	}
	r2, ok := s.deps.Store.GetByName(name)
	if !ok {
		writePostSyncErr(w, fmt.Errorf("repo %q not found", name))
		return
	}
	found := false
	filtered := make([]types.PostSyncCommand, 0, len(r2.PostSyncCommands))
	for _, c := range r2.PostSyncCommands {
		if c.ID == req.ID {
			found = true
			continue
		}
		filtered = append(filtered, c)
	}
	if !found {
		writePostSyncErr(w, fmt.Errorf("post-sync command with ID %q not found", req.ID))
		return
	}
	r2.PostSyncCommands = filtered
	if err := s.deps.Store.Update(r2); err != nil {
		writePostSyncErr(w, err)
		return
	}
	writeOK(w, normalizePostSync(r2.PostSyncCommands))
}

// writePostSyncErr mirrors cmd/repo_postsync.go handlePostSyncError: emit a
// PostSyncCommandsData envelope carrying the error so the frontend's existing
// success/error parsing is preserved.
func writePostSyncErr(w http.ResponseWriter, err error) {
	writeErr[types.PostSyncCommandsData](w, err)
}

// normalizePostSync guarantees a non-nil slice (frontend expects an array).
func normalizePostSync(cmds []types.PostSyncCommand) types.PostSyncCommandsData {
	if cmds == nil {
		cmds = []types.PostSyncCommand{}
	}
	return types.PostSyncCommandsData{Commands: cmds}
}

// ---------------------------------------------------------------------------
// summarize
// ---------------------------------------------------------------------------

// summarizeData mirrors cmd/summarize.go SummarizeData.
type summarizeData struct {
	HistoryID     int64  `json:"historyId"`
	RepoName      string `json:"repoName"`
	Summary       string `json:"summary"`
	SummaryStatus string `json:"summaryStatus"`
}

type summarizeRequest struct {
	Retry bool `json:"retry,omitempty"`
}

func (s *Server) handleSummarize(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	req, ok := decodeJSON[summarizeRequest](w, r)
	if !ok {
		// allow no-body POST (retry defaults to false)
		req = summarizeRequest{}
	}

	r2, ok := s.deps.Store.GetByName(name)
	if !ok {
		writeErr[summarizeData](w, fmt.Errorf("repository %q not found", name))
		return
	}

	// Reuse the long-lived history store from Deps instead of opening a fresh
	// SQLite connection per request. The summarizer writes summary_status to
	// the same DB that the syncer concurrently writes records to, so a
	// per-request connection races for the write lock (SQLITE_BUSY). Fall back
	// to a one-shot store only if the shared one failed to initialize at boot.
	hStore := s.deps.HistStore
	if hStore == nil {
		hs, err := history.NewStore(s.deps.ConfigDir())
		if err != nil {
			writeErr[summarizeData](w, fmt.Errorf("open history store: %w", err))
			return
		}
		defer hs.Close()
		hStore = hs
	}

	record, err := hStore.LatestByRepo(r2.ID)
	if err != nil {
		writeErr[summarizeData](w, fmt.Errorf("no sync history found for %q", name))
		return
	}

	if req.Retry && record.SummaryStatus != string(types.SummaryStatusFailed) {
		writeErr[summarizeData](w, fmt.Errorf("latest sync for %q is not in failed state (current: %s)", name, record.SummaryStatus))
		return
	}

	if !req.Retry && record.SummaryStatus == string(types.SummaryStatusDone) {
		writeOK(w, summarizeData{
			HistoryID:     record.ID,
			RepoName:      record.RepoName,
			Summary:       record.Summary,
			SummaryStatus: record.SummaryStatus,
		})
		return
	}

	summary, err := generateSummary(r.Context(), s.deps.Cfg, hStore, s.deps.Bus, record, r2)
	if err != nil {
		writeErr[summarizeData](w, err)
		return
	}
	writeOK(w, summarizeData{
		HistoryID:     record.ID,
		RepoName:      record.RepoName,
		Summary:       summary,
		SummaryStatus: string(types.SummaryStatusDone),
	})
}

// GenerateSummary is the exported wrapper for the Wails App layer.
func GenerateSummary(ctx context.Context, cfg *config.Config, histStore *history.Store, bus *eventbus.Bus, record *history.Record, r types.Repo) (string, error) {
	return generateSummary(ctx, cfg, histStore, bus, record, r)
}

// generateSummary mirrors cmd/summarize.go generateSummary verbatim.
func generateSummary(ctx context.Context, cfg *config.Config, histStore *history.Store, bus *eventbus.Bus, record *history.Record, r types.Repo) (string, error) {
	agentName := ""
	if cfg != nil {
		agentName = cfg.Sync.SummaryAgent
	}
	if agentName == "" {
		if prov, err := agent.ResolveProvider("", ""); err == nil {
			agentName = prov.Name()
		}
	}
	if agentName == "" {
		return "", fmt.Errorf("no agent available. Install Claude Code, OpenCode, or Codex, or configure sync.summary_agent")
	}

	if !summarizer.IsAgentAvailable(agentName) {
		return "", fmt.Errorf("agent %q is not installed", agentName)
	}

	if record.OldHEAD == "" {
		if updateErr := histStore.UpdateSummary(record.ID, "", string(types.SummaryStatusFailed)); updateErr != nil {
			logger.Error("summarize: failed to set failed status (no old HEAD)", "error", updateErr)
		}
		return "", fmt.Errorf("no old HEAD recorded for %q, cannot determine pulled commits", r.Name)
	}

	if updateErr := histStore.UpdateSummary(record.ID, "", string(types.SummaryStatusGenerating)); updateErr != nil {
		logger.Error("summarize: failed to set generating status", "error", updateErr)
	}

	gitOps := newGitOps(cfg)
	upstreamRef := gitOps.ResolveUpstreamRef(ctx, r)
	gitCommits, err := gitOps.GetCommitLog(ctx, r.Path, record.OldHEAD, upstreamRef)
	if err != nil || len(gitCommits) == 0 {
		if updateErr := histStore.UpdateSummary(record.ID, "", string(types.SummaryStatusFailed)); updateErr != nil {
			logger.Error("summarize: failed to set failed status (no commits)", "error", updateErr)
		}
		return "", fmt.Errorf("no commits found for summarization")
	}

	var commits []summarizer.CommitInfo
	for _, c := range gitCommits {
		commits = append(commits, summarizer.CommitInfo{Hash: c.Hash, Message: c.Message})
	}

	lang := types.DefaultSummaryLanguage
	if cfg != nil && cfg.Sync.SummaryLanguage != "" {
		lang = cfg.Sync.SummaryLanguage
	}

	executor := summarizer.NewExecutor()
	summary, err := executor.Summarize(ctx, commits, lang, agentName)
	if err != nil {
		if updateErr := histStore.UpdateSummary(record.ID, "", string(types.SummaryStatusFailed)); updateErr != nil {
			logger.Error("summarize: failed to set failed status after error", "error", updateErr)
		}
		return "", fmt.Errorf("summarization failed: %w", err)
	}

	if updateErr := histStore.UpdateSummary(record.ID, summary, string(types.SummaryStatusDone)); updateErr != nil {
		logger.Error("summarize: failed to save summary result", "error", updateErr)
	}
	// History changed (summary complete) — notify subscribers so the renderer
	// stops its summary-generation poll and shows the new summary.
	if bus != nil {
		bus.Publish(eventbus.Event{Type: eventbus.EventHistoryChanged})
	}
	return summary, nil
}
