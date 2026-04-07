package agent

import (
	"context"
	"fmt"
	"os/exec"
	"time"
)

// ClaudeAdapter implements AgentProvider for Claude Code CLI.
//
// Invocation: claude --print --dangerously-skip-permissions [--resume <id>] --append-system-prompt <text> <prompt>
//
// Claude Code CLI flags:
//   - --print: non-interactive output mode
//   - --dangerously-skip-permissions: autonomous mode (no approval prompts)
//   - --resume <session-id>: resume existing session
//   - --append-system-prompt <text>: inject system prompt
type ClaudeAdapter struct {
	binary string
}

func NewClaudeAdapter() *ClaudeAdapter {
	return &ClaudeAdapter{binary: "claude"}
}

func (a *ClaudeAdapter) Name() string { return "claude" }

func (a *ClaudeAdapter) IsAvailable() bool {
	_, err := exec.LookPath(a.binary)
	return err == nil
}

func (a *ClaudeAdapter) StartSession(ctx context.Context, opts SessionOptions) (*Session, error) {
	// Claude Code creates sessions implicitly on first interaction.
	contextPrompt := buildContextInjectionPrompt(opts)
	result, err := a.runCommand(ctx, "", opts.RepoPath, contextPrompt)
	if err != nil {
		return nil, fmt.Errorf("claude start session: %w", err)
	}

	sessionID := extractSessionID(result)
	return &Session{
		ID:        sessionID,
		Provider:  "claude",
		RepoPath:  opts.RepoPath,
		StartedAt: time.Now(),
	}, nil
}

func (a *ClaudeAdapter) ResolveConflicts(ctx context.Context, session *Session, prompt string) (*AgentResult, error) {
	output, err := a.runCommand(ctx, session.ID, session.RepoPath, prompt)
	if err != nil {
		return &AgentResult{
			Success:   false,
			SessionID: session.ID,
			Summary:   fmt.Sprintf("claude error: %v", err),
		}, fmt.Errorf("claude resolve: %w", err)
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

func (a *ClaudeAdapter) EndSession(ctx context.Context, sessionID string) error {
	// Claude Code sessions don't need explicit cleanup
	return nil
}

// buildArgs constructs the CLI arguments for a Claude Code invocation.
func (a *ClaudeAdapter) buildArgs(sessionID, prompt string) []string {
	args := []string{"--print", "--dangerously-skip-permissions"}
	if sessionID != "" {
		args = append(args, "--resume", sessionID)
	}
	args = append(args, prompt)
	return args
}

func (a *ClaudeAdapter) runCommand(ctx context.Context, sessionID, repoPath, prompt string) (string, error) {
	args := a.buildArgs(sessionID, prompt)
	cmd := exec.CommandContext(ctx, a.binary, args...)
	cmd.Dir = repoPath

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("claude CLI: %s: %w", string(output), err)
	}
	return string(output), nil
}
