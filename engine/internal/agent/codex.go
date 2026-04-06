package agent

import (
	"context"
	"fmt"
	"os/exec"
	"time"
)

// CodexAdapter implements AgentProvider for Codex CLI (OpenAI).
//
// CLI flags:
//   - --dangerously-bypass-approvals-and-sandbox: autonomous mode
//   - resume --last: resume last session
type CodexAdapter struct {
	binary string
}

func NewCodexAdapter() *CodexAdapter {
	return &CodexAdapter{binary: "codex"}
}

func (a *CodexAdapter) Name() string { return "codex" }

func (a *CodexAdapter) IsAvailable() bool {
	_, err := exec.LookPath(a.binary)
	return err == nil
}

func (a *CodexAdapter) StartSession(ctx context.Context, opts SessionOptions) (*Session, error) {
	contextPrompt := buildContextInjectionPrompt(opts)
	output, err := a.runCommand(ctx, false, opts.RepoPath, contextPrompt)
	if err != nil {
		return nil, fmt.Errorf("codex start session: %w", err)
	}

	return &Session{
		ID:        extractSessionID(output),
		Provider:  "codex",
		RepoPath:  opts.RepoPath,
		StartedAt: time.Now(),
	}, nil
}

func (a *CodexAdapter) ResolveConflicts(ctx context.Context, session *Session, prompt string) (*AgentResult, error) {
	output, err := a.runCommand(ctx, session.ID != "", session.RepoPath, prompt)
	if err != nil {
		return &AgentResult{
			Success:   false,
			SessionID: session.ID,
			Summary:   fmt.Sprintf("codex error: %v", err),
		}, fmt.Errorf("codex resolve: %w", err)
	}

	sessionID := extractSessionID(output)
	if sessionID == "" {
		sessionID = session.ID
	}

	return &AgentResult{
		Success:   true,
		SessionID: sessionID,
		Summary:   truncateOutput(output, 500),
	}, nil
}

func (a *CodexAdapter) EndSession(ctx context.Context, sessionID string) error {
	return nil
}

func (a *CodexAdapter) buildArgs(sessionID, prompt string) []string {
	args := []string{}
	if sessionID != "" {
		args = append(args, "resume", "--last")
	}
	args = append(args, "--dangerously-bypass-approvals-and-sandbox", prompt)
	return args
}

func (a *CodexAdapter) runCommand(ctx context.Context, resume bool, repoPath, prompt string) (string, error) {
	sessionID := ""
	if resume {
		sessionID = "yes" // any non-empty string triggers resume mode
	}
	args := a.buildArgs(sessionID, prompt)
	cmd := exec.CommandContext(ctx, a.binary, args...)
	cmd.Dir = repoPath

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("codex CLI: %s: %w", string(output), err)
	}
	return string(output), nil
}
