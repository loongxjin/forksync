package agent

import "context"

// DroidAdapter implements AgentProvider for Droid CLI.
type DroidAdapter struct {
	binary string
}

func NewDroidAdapter() *DroidAdapter {
	return &DroidAdapter{binary: "droid"}
}

func (a *DroidAdapter) Name() string { return "droid" }
func (a *DroidAdapter) IsAvailable() bool { return false }
func (a *DroidAdapter) StartSession(ctx context.Context, opts SessionOptions) (*Session, error) {
	return nil, nil
}
func (a *DroidAdapter) ResolveConflicts(ctx context.Context, session *Session, prompt string) (*AgentResult, error) {
	return nil, nil
}
func (a *DroidAdapter) EndSession(ctx context.Context, sessionID string) error {
	return nil
}
