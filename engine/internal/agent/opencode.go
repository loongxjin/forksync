package agent

import "context"

// OpenCodeAdapter implements AgentProvider for OpenCode CLI.
type OpenCodeAdapter struct {
	binary string
}

func NewOpenCodeAdapter() *OpenCodeAdapter {
	return &OpenCodeAdapter{binary: "opencode"}
}

func (a *OpenCodeAdapter) Name() string { return "opencode" }
func (a *OpenCodeAdapter) IsAvailable() bool { return false }
func (a *OpenCodeAdapter) StartSession(ctx context.Context, opts SessionOptions) (*Session, error) {
	return nil, nil
}
func (a *OpenCodeAdapter) ResolveConflicts(ctx context.Context, session *Session, prompt string) (*AgentResult, error) {
	return nil, nil
}
func (a *OpenCodeAdapter) EndSession(ctx context.Context, sessionID string) error {
	return nil
}
