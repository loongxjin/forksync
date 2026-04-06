package agent

import "context"

// ClaudeAdapter implements AgentProvider for Claude Code CLI.
type ClaudeAdapter struct {
	binary string
}

// NewClaudeAdapter creates a new Claude Code adapter.
func NewClaudeAdapter() *ClaudeAdapter {
	return &ClaudeAdapter{binary: "claude"}
}

func (a *ClaudeAdapter) Name() string { return "claude" }
func (a *ClaudeAdapter) IsAvailable() bool { return false }
func (a *ClaudeAdapter) StartSession(ctx context.Context, opts SessionOptions) (*Session, error) {
	return nil, nil
}
func (a *ClaudeAdapter) ResolveConflicts(ctx context.Context, session *Session, prompt string) (*AgentResult, error) {
	return nil, nil
}
func (a *ClaudeAdapter) EndSession(ctx context.Context, sessionID string) error {
	return nil
}
