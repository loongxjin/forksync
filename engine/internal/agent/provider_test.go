package agent

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

// MockProvider implements AgentProvider for testing.
type MockProvider struct {
	NameFunc          func() string
	IsAvailableFunc   func() bool
	StartSessionFunc  func(ctx context.Context, opts SessionOptions) (*Session, error)
	ResolveFunc       func(ctx context.Context, session *Session, prompt string) (*AgentResult, error)
	EndSessionFunc    func(ctx context.Context, sessionID string) error
}

func (m *MockProvider) Name() string {
	if m.NameFunc != nil {
		return m.NameFunc()
	}
	return "mock"
}

func (m *MockProvider) IsAvailable() bool {
	if m.IsAvailableFunc != nil {
		return m.IsAvailableFunc()
	}
	return true
}

func (m *MockProvider) StartSession(ctx context.Context, opts SessionOptions) (*Session, error) {
	if m.StartSessionFunc != nil {
		return m.StartSessionFunc(ctx, opts)
	}
	return &Session{ID: "mock-session", Provider: "mock", RepoPath: opts.RepoPath}, nil
}

func (m *MockProvider) ResolveConflicts(ctx context.Context, session *Session, prompt string) (*AgentResult, error) {
	if m.ResolveFunc != nil {
		return m.ResolveFunc(ctx, session, prompt)
	}
	return &AgentResult{Success: true, SessionID: session.ID, Summary: "mock resolved"}, nil
}

func (m *MockProvider) EndSession(ctx context.Context, sessionID string) error {
	if m.EndSessionFunc != nil {
		return m.EndSessionFunc(ctx, sessionID)
	}
	return nil
}

func TestAgentProviderInterface(t *testing.T) {
	// Verify MockProvider satisfies AgentProvider interface at compile time
	var _ AgentProvider = &MockProvider{}
}

func TestSessionOptionsFields(t *testing.T) {
	opts := SessionOptions{
		RepoPath:     "/path/to/repo",
		RepoName:     "my-repo",
		ContextFiles: []string{"README.md", "go.mod"},
	}

	if opts.RepoPath != "/path/to/repo" {
		t.Errorf("RepoPath = %q; want %q", opts.RepoPath, "/path/to/repo")
	}
	if len(opts.ContextFiles) != 2 {
		t.Errorf("ContextFiles length = %d; want 2", len(opts.ContextFiles))
	}
}

func TestSessionFields(t *testing.T) {
	s := &Session{
		ID:        "sess-123",
		Provider:  "claude",
		RepoPath:  "/path/to/repo",
		StartedAt: mustParseTime("2026-04-06T12:00:00Z"),
	}

	if s.ID != "sess-123" {
		t.Errorf("ID = %q; want %q", s.ID, "sess-123")
	}
	if s.Provider != "claude" {
		t.Errorf("Provider = %q; want %q", s.Provider, "claude")
	}
}

func TestAgentResultFields(t *testing.T) {
	r := &AgentResult{
		Success:       true,
		Diff:          "--- a.go\n+++ a.go",
		Summary:       "resolved 2 conflicts",
		SessionID:     "sess-123",
		ResolvedFiles: []string{"a.go", "b.go"},
	}

	if !r.Success {
		t.Error("Success should be true")
	}
	if len(r.ResolvedFiles) != 2 {
		t.Errorf("ResolvedFiles length = %d; want 2", len(r.ResolvedFiles))
	}
}

func TestMockProviderStartSession(t *testing.T) {
	mock := &MockProvider{
		StartSessionFunc: func(ctx context.Context, opts SessionOptions) (*Session, error) {
			return &Session{ID: "new-sess", Provider: "mock", RepoPath: opts.RepoPath}, nil
		},
	}

	sess, err := mock.StartSession(context.Background(), SessionOptions{
		RepoPath: "/tmp/repo",
		RepoName: "test",
	})
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}
	if sess.ID != "new-sess" {
		t.Errorf("session ID = %q; want %q", sess.ID, "new-sess")
	}
}

func TestMockProviderResolveConflicts(t *testing.T) {
	mock := &MockProvider{
		ResolveFunc: func(ctx context.Context, session *Session, prompt string) (*AgentResult, error) {
			return &AgentResult{
				Success:       true,
				SessionID:     session.ID,
				ResolvedFiles: []string{"a.go"},
				Summary:       "resolved",
			}, nil
		},
	}

	sess := &Session{ID: "sess-1", Provider: "mock", RepoPath: "/tmp/repo"}
	result, err := mock.ResolveConflicts(context.Background(), sess, "resolve these files: a.go")
	if err != nil {
		t.Fatalf("ResolveConflicts: %v", err)
	}
	if !result.Success {
		t.Error("result should be success")
	}
}

func TestBuildConflictPrompt(t *testing.T) {
	files := []string{"pkg/handler.go", "internal/service/user.go"}
	strategy := "preserve_ours"

	prompt := BuildConflictPrompt(files, strategy)

	if len(prompt) == 0 {
		t.Error("prompt should not be empty")
	}
	for _, f := range files {
		if !strings.Contains(prompt, f) {
			t.Errorf("prompt should contain %q", f)
		}
	}
	// strategy parameter is used to select the description text
	if !strings.Contains(prompt, "策略") {
		t.Error("prompt should contain strategy description")
	}
}

func TestExtractSessionID(t *testing.T) {
	tests := []struct {
		name   string
		output string
		wantID string
	}{
		{
			name:   "claude session line",
			output: "Session: sess-abc123\nResolved 2 files",
			wantID: "sess-abc123",
		},
		{
			name:   "empty output",
			output: "",
			wantID: "",
		},
		{
			name:   "no session line",
			output: "just some output",
			wantID: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractSessionID(tt.output)
			if got != tt.wantID {
				t.Errorf("extractSessionID(%q) = %q; want %q", tt.output, got, tt.wantID)
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStr(s, substr))
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func mustParseTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(fmt.Sprintf("invalid time %q: %v", s, err))
	}
	return t
}
