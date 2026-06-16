package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/loongxjin/forksync/engine/internal/config"
	"github.com/loongxjin/forksync/engine/internal/git"
	"github.com/loongxjin/forksync/engine/internal/repo"
	"github.com/loongxjin/forksync/engine/pkg/types"
)

// newTestServer builds a Server with a real config manager + repo store in a
// temp dir, so the config/repo/post-sync/diff handlers can be exercised without
// touching the user's real ~/.forksync. gitOps/syncer/resolver are nil — only
// handlers that don't need them are tested here.
func newTestServer(t *testing.T) *Server {
	t.Helper()
	dir := t.TempDir()
	cfgMgr := config.NewManagerWithDir(dir)
	cfg, _ := cfgMgr.Load() // empty config is fine
	store := repo.NewJSONStore(dir)
	if err := store.Load(); err != nil {
		t.Fatalf("load store: %v", err)
	}
	deps := &Deps{
		Cfg:       cfg,
		CfgMgr:    cfgMgr,
		Store:     store,
		GitOps:    git.NewOperations(),
		configDir: dir,
	}
	mux := http.NewServeMux()
	s := &Server{deps: deps}
	s.registerMiscRoutes(mux)
	s.registerRepoRoutes(mux)
	return s
}

func do(t *testing.T, s *Server, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	var r *http.Request
	if body != "" {
		r = httptest.NewRequest(method, target, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	} else {
		r = httptest.NewRequest(method, target, nil)
	}
	// ServeMux needs the handler; build a fresh mux each call via the server.
	mux := http.NewServeMux()
	s.registerMiscRoutes(mux)
	s.registerRepoRoutes(mux)
	// Re-register all so path patterns match.
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, r)
	return rec
}

// --- config ---

func TestHandleConfigGet(t *testing.T) {
	s := newTestServer(t)
	rec := do(t, s, http.MethodGet, "/config", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	env := recJSON(t, rec)
	if !env.Success {
		t.Fatalf("config get should succeed, error=%q", env.Error)
	}
}

func TestHandleConfigSet(t *testing.T) {
	s := newTestServer(t)
	rec := do(t, s, http.MethodPut, "/config", `{"key":"agent.preferred","value":"claude"}`)
	env := recJSON(t, rec)
	if !env.Success {
		t.Fatalf("config set should succeed, error=%q", env.Error)
	}
	// Verify it persisted.
	cfg, _ := s.deps.CfgMgr.Load()
	if cfg.Agent.Preferred != "claude" {
		t.Fatalf("preferred = %q, want claude", cfg.Agent.Preferred)
	}
}

// --- repo CRUD ---

func TestHandleAddRepo_NotGitRepo(t *testing.T) {
	s := newTestServer(t)
	dir := t.TempDir()
	rec := do(t, s, http.MethodPost, "/repos", `{"path":"`+dir+`"}`)
	env := recJSON(t, rec)
	// A plain temp dir is not a git repo → error envelope.
	if env.Success {
		t.Fatal("add on non-git dir should fail")
	}
}

func TestHandleRemoveRepo_NotFound(t *testing.T) {
	s := newTestServer(t)
	rec := do(t, s, http.MethodDelete, "/repos/does-not-exist", "")
	env := recJSON(t, rec)
	if env.Success {
		t.Fatal("remove on missing repo should fail")
	}
}

func TestHandleRepoDiff_NotFound(t *testing.T) {
	s := newTestServer(t)
	rec := do(t, s, http.MethodGet, "/repos/missing/diff", "")
	// repoDiff returns a bare {success:false,error} object, NOT an envelope.
	var bare repoDiffResult
	if err := json.Unmarshal(rec.Body.Bytes(), &bare); err != nil {
		t.Fatalf("diff body not valid bare JSON: %v", err)
	}
	if bare.Success {
		t.Fatal("diff on missing repo should return success=false")
	}
	if bare.Error == "" {
		t.Fatal("diff error message should be non-empty")
	}
}

// --- post-sync ---

func TestHandlePostSyncRoundTrip(t *testing.T) {
	s := newTestServer(t)
	// Seed a repo.
	r := types.Repo{ID: "r1", Name: "myrepo", Path: filepath.Join(t.TempDir(), "myrepo")}
	if err := s.deps.Store.Add(r); err != nil {
		t.Fatal(err)
	}

	// List (empty).
	rec := do(t, s, http.MethodGet, "/repos/myrepo/post-sync", "")
	env := recJSON(t, rec)
	if !env.Success {
		t.Fatalf("list should succeed, error=%q", env.Error)
	}

	// Add a command.
	rec = do(t, s, http.MethodPost, "/repos/myrepo/post-sync", `{"name":"build","cmd":"npm run build"}`)
	env = recJSON(t, rec)
	if !env.Success {
		t.Fatalf("add should succeed, error=%q", env.Error)
	}

	// Remove it (need the ID; re-read from store).
	got, ok := s.deps.Store.GetByName("myrepo")
	if !ok || len(got.PostSyncCommands) != 1 {
		t.Fatalf("expected 1 post-sync command after add, got %d", len(got.PostSyncCommands))
	}
	cmdID := got.PostSyncCommands[0].ID
	rec = do(t, s, http.MethodDelete, "/repos/myrepo/post-sync", `{"id":"`+cmdID+`"}`)
	env = recJSON(t, rec)
	if !env.Success {
		t.Fatalf("remove should succeed, error=%q", env.Error)
	}
	got, _ = s.deps.Store.GetByName("myrepo")
	if len(got.PostSyncCommands) != 0 {
		t.Fatalf("expected 0 commands after remove, got %d", len(got.PostSyncCommands))
	}
}

// --- history (shared store nil fallback path) ---

func TestHandleHistory_EmptyReturnsRecords(t *testing.T) {
	s := newTestServer(t)
	// HistStore is nil → handler opens a one-shot store in the temp dir.
	rec := do(t, s, http.MethodGet, "/history?limit=5", "")
	env := recJSON(t, rec)
	if !env.Success {
		t.Fatalf("history should succeed, error=%q", env.Error)
	}
}

func TestHandleHistoryCleanup_All(t *testing.T) {
	s := newTestServer(t)
	rec := do(t, s, http.MethodPost, "/history/cleanup", `{}`)
	env := recJSON(t, rec)
	if !env.Success {
		t.Fatalf("cleanup should succeed, error=%q", env.Error)
	}
}

// keep context import referenced (used by future store-backed tests).
var _ = context.Background
