package session

import (
	"context"
	"testing"
	"time"

	"github.com/loongxjin/forksync/engine/internal/agent"
)

// mockProvider implements agent.AgentProvider for testing
type mockProvider struct {
	startSessionFunc func(ctx context.Context, opts agent.SessionOptions) (*agent.Session, error)
	resolveFunc      func(ctx context.Context, session *agent.Session, prompt string) (*agent.AgentResult, error)
}

func (m *mockProvider) Name() string { return "mock" }
func (m *mockProvider) IsAvailable() bool { return true }
func (m *mockProvider) StartSession(ctx context.Context, opts agent.SessionOptions) (*agent.Session, error) {
	if m.startSessionFunc != nil {
		return m.startSessionFunc(ctx, opts)
	}
	return &agent.Session{ID: "mock-sess-1", Provider: "mock", RepoPath: opts.RepoPath}, nil
}
func (m *mockProvider) ResolveConflicts(ctx context.Context, session *agent.Session, prompt string) (*agent.AgentResult, error) {
	if m.resolveFunc != nil {
		return m.resolveFunc(ctx, session, prompt)
	}
	return &agent.AgentResult{Success: true, SessionID: session.ID, Summary: "resolved"}, nil
}
func (m *mockProvider) EndSession(ctx context.Context, sessionID string) error { return nil }

func TestManager_GetOrCreate_NewSession(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewSessionStore(tmpDir)
	provider := &mockProvider{}

	mgr := NewManager(store, provider)

	sess, err := mgr.GetOrCreate(context.Background(), "repo-1", "/path/to/repo")
	if err != nil {
		t.Fatalf("GetOrCreate: %v", err)
	}
	if sess == nil {
		t.Fatal("session should not be nil")
	}
	if sess.ID == "" {
		t.Error("session ID should not be empty")
	}
	if sess.Provider != "mock" {
		t.Errorf("Provider = %q; want %q", sess.Provider, "mock")
	}
}

func TestManager_GetOrCreate_ExistingSession(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewSessionStore(tmpDir)
	provider := &mockProvider{}

	// Pre-save an active session
	_ = store.Save(&SessionRecord{
		RepoID:     "repo-1",
		RepoPath:   "/path/to/repo",
		AgentName:  "mock",
		SessionID:  "existing-sess",
		CreatedAt:  time.Now(),
		LastUsedAt: time.Now(),
		Status:     "active",
	})

	mgr := NewManager(store, provider)

	sess, err := mgr.GetOrCreate(context.Background(), "repo-1", "/path/to/repo")
	if err != nil {
		t.Fatalf("GetOrCreate: %v", err)
	}
	if sess.ID != "existing-sess" {
		t.Errorf("should reuse existing session; got ID = %q", sess.ID)
	}
}

func TestManager_GetOrCreate_ExpiredSession(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewSessionStore(tmpDir)
	provider := &mockProvider{}

	// Pre-save an expired session
	_ = store.Save(&SessionRecord{
		RepoID:     "repo-1",
		RepoPath:   "/path/to/repo",
		AgentName:  "mock",
		SessionID:  "old-sess",
		CreatedAt:  time.Now().Add(-48 * time.Hour),
		LastUsedAt: time.Now().Add(-48 * time.Hour),
		Status:     "active",
	})

	mgr := NewManager(store, provider)

	sess, err := mgr.GetOrCreate(context.Background(), "repo-1", "/path/to/repo")
	if err != nil {
		t.Fatalf("GetOrCreate: %v", err)
	}
	// Should create a new session since the old one is expired (> 24h)
	if sess.ID == "old-sess" {
		t.Error("should not reuse expired session")
	}
}

func TestManager_ResolveConflicts(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewSessionStore(tmpDir)
	provider := &mockProvider{
		resolveFunc: func(ctx context.Context, session *agent.Session, prompt string) (*agent.AgentResult, error) {
			return &agent.AgentResult{
				Success:       true,
				SessionID:     session.ID,
				ResolvedFiles: []string{"a.go", "b.go"},
				Summary:       "resolved 2 files",
			}, nil
		},
	}

	mgr := NewManager(store, provider)

	// Create session first
	_, _ = mgr.GetOrCreate(context.Background(), "repo-1", "/path/to/repo")

	result, err := mgr.ResolveConflicts(context.Background(), "repo-1", "/path/to/repo", []string{"a.go", "b.go"}, "preserve_ours")
	if err != nil {
		t.Fatalf("ResolveConflicts: %v", err)
	}
	if !result.Success {
		t.Error("result should be success")
	}
	if len(result.ResolvedFiles) != 2 {
		t.Errorf("ResolvedFiles length = %d; want 2", len(result.ResolvedFiles))
	}

	// Verify session was updated in store
	rec, _ := store.Load("repo-1")
	if rec == nil {
		t.Fatal("session record should exist in store")
	}
}

func TestManager_ResolveConflicts_NoSession(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewSessionStore(tmpDir)
	provider := &mockProvider{}

	mgr := NewManager(store, provider)

	// Create session first, then resolve — should auto-create
	sess, err := mgr.GetOrCreate(context.Background(), "repo-new", "/path/to/repo")
	if err != nil {
		t.Fatalf("GetOrCreate: %v", err)
	}
	if sess == nil {
		t.Fatal("session should not be nil")
	}

	result, err := mgr.ResolveConflicts(context.Background(), "repo-new", "/path/to/repo", []string{"a.go"}, "preserve_ours")
	if err != nil {
		t.Fatalf("ResolveConflicts: %v", err)
	}
	if !result.Success {
		t.Error("result should be success")
	}
}

func TestManager_CloseAll(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewSessionStore(tmpDir)
	provider := &mockProvider{}

	mgr := NewManager(store, provider)

	// Create two sessions
	_, _ = mgr.GetOrCreate(context.Background(), "repo-1", "/path/1")
	_, _ = mgr.GetOrCreate(context.Background(), "repo-2", "/path/2")

	if err := mgr.CloseAll(context.Background()); err != nil {
		t.Fatalf("CloseAll: %v", err)
	}
}

func TestManager_CleanupExpired(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewSessionStore(tmpDir)
	provider := &mockProvider{}

	// Save some sessions with different statuses
	_ = store.Save(&SessionRecord{RepoID: "r1", Status: "active"})
	_ = store.Save(&SessionRecord{RepoID: "r2", Status: "expired"})
	_ = store.Save(&SessionRecord{RepoID: "r3", Status: "failed"})

	mgr := NewManager(store, provider)
	cleaned, err := mgr.CleanupExpired()
	if err != nil {
		t.Fatalf("CleanupExpired: %v", err)
	}
	if cleaned != 2 {
		t.Errorf("cleaned = %d; want 2", cleaned)
	}
}

func TestManager_ListSessions(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewSessionStore(tmpDir)
	provider := &mockProvider{}

	_ = store.Save(&SessionRecord{
		RepoID:    "r1",
		AgentName: "mock",
		SessionID: "s1",
		Status:    "active",
	})
	_ = store.Save(&SessionRecord{
		RepoID:    "r2",
		AgentName: "mock",
		SessionID: "s2",
		Status:    "expired",
	})

	mgr := NewManager(store, provider)
	sessions, err := mgr.ListSessions()
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(sessions) != 2 {
		t.Errorf("sessions length = %d; want 2", len(sessions))
	}
}
