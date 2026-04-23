package agent

import (
	"context"
)

// OpenCodeAdapter implements AgentProvider for OpenCode CLI.
//
// Invocation: opencode run [--session <id>] <message>
//
// OpenCode CLI uses the "run" subcommand for non-interactive execution.
// There is no autonomous-mode flag — OpenCode executes directly without confirmation in run mode.
type OpenCodeAdapter struct {
	baseAdapter
}

func NewOpenCodeAdapter() *OpenCodeAdapter {
	return &OpenCodeAdapter{baseAdapter{binary: "opencode", name: "opencode"}}
}

func (a *OpenCodeAdapter) StartSession(ctx context.Context, opts SessionOptions) (*Session, error) {
	return a.baseAdapter.StartSession(ctx, opts, a.buildArgs)
}

func (a *OpenCodeAdapter) ResolveConflicts(ctx context.Context, session *Session, prompt string) (*AgentResult, error) {
	return a.baseAdapter.ResolveConflicts(ctx, session, prompt, a.buildArgs)
}

// buildArgs constructs the CLI arguments for an OpenCode invocation.
func (a *OpenCodeAdapter) buildArgs(sessionID, prompt string) []string {
	args := []string{"run"}
	if sessionID != "" {
		args = append(args, "--session", sessionID)
	}
	args = append(args, prompt)
	return args
}
