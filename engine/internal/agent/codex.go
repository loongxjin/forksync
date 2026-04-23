package agent

import (
	"context"
)

// CodexAdapter implements AgentProvider for Codex CLI (OpenAI).
//
// Invocation:
//   - New session:  codex --dangerously-bypass-approvals-and-sandbox <prompt>
//   - Resume:       codex resume --last --dangerously-bypass-approvals-and-sandbox <prompt>
//
// Codex CLI uses "resume --last" as a subcommand to continue the last session.
type CodexAdapter struct {
	baseAdapter
}

func NewCodexAdapter() *CodexAdapter {
	return &CodexAdapter{baseAdapter{binary: "codex", name: "codex"}}
}

func (a *CodexAdapter) StartSession(ctx context.Context, opts SessionOptions) (*Session, error) {
	return a.baseAdapter.StartSession(ctx, opts, a.buildArgs)
}

func (a *CodexAdapter) ResolveConflicts(ctx context.Context, session *Session, prompt string) (*AgentResult, error) {
	return a.baseAdapter.ResolveConflicts(ctx, session, prompt, a.buildArgs)
}

// buildArgs constructs the CLI arguments for a Codex invocation.
// Uses "codex exec" for non-interactive execution.
// sessionID is non-empty when resuming an existing session.
func (a *CodexAdapter) buildArgs(sessionID, prompt string) []string {
	args := []string{"exec"}
	if sessionID != "" {
		args = append(args, "resume", "--last")
	}
	args = append(args, "--dangerously-bypass-approvals-and-sandbox", prompt)
	return args
}
