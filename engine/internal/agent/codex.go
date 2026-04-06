package agent

import "context"

// CodexAdapter implements AgentProvider for Codex CLI.
type CodexAdapter struct {
	binary string
}

func NewCodexAdapter() *CodexAdapter {
	return &CodexAdapter{binary: "codex"}
}

func (a *CodexAdapter) Name() string { return "codex" }
func (a *CodexAdapter) IsAvailable() bool { return false }
func (a *CodexAdapter) StartSession(ctx context.Context, opts SessionOptions) (*Session, error) {
	return nil, nil
}
func (a *CodexAdapter) ResolveConflicts(ctx context.Context, session *Session, prompt string) (*AgentResult, error) {
	return nil, nil
}
func (a *CodexAdapter) EndSession(ctx context.Context, sessionID string) error {
	return nil
}
