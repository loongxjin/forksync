package agent

import (
	"context"
	"fmt"
	"os/exec"
	"time"
)

// OpenCodeAdapter implements AgentProvider for OpenCode CLI.
//
// CLI flags:
//   - --prompt <text>: non-interactive prompt
//   - --continue: resume existing session
type OpenCodeAdapter struct {
	binary string
}

func NewOpenCodeAdapter() *OpenCodeAdapter {
	return &OpenCodeAdapter{binary: "opencode"}
}

func (a *OpenCodeAdapter) Name() string { return "opencode" }

func (a *OpenCodeAdapter) IsAvailable() bool {
	_, err := exec.LookPath(a.binary)
	return err == nil
}

func (a *OpenCodeAdapter) StartSession(ctx context.Context, opts SessionOptions) (*Session, error) {
	contextPrompt := buildContextInjectionPrompt(opts)
	output, err := a.runCommand(ctx, "", opts.RepoPath, contextPrompt)
	if err != nil {
		return nil, fmt.Errorf("opencode start session: %w", err)
	}

	return &Session{
		ID:        extractSessionID(output),
		Provider:  "opencode",
		RepoPath:  opts.RepoPath,
		StartedAt: time.Now(),
	}, nil
}

func (a *OpenCodeAdapter) ResolveConflicts(ctx context.Context, session *Session, prompt string) (*AgentResult, error) {
	output, err := a.runCommand(ctx, session.ID, session.RepoPath, prompt)
	if err != nil {
		return &AgentResult{
			Success:   false,
			SessionID: session.ID,
			Summary:   fmt.Sprintf("opencode error: %v", err),
		}, fmt.Errorf("opencode resolve: %w", err)
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

func (a *OpenCodeAdapter) EndSession(ctx context.Context, sessionID string) error {
	return nil
}

func (a *OpenCodeAdapter) buildArgs(sessionID, prompt string) []string {
	args := []string{}
	if sessionID != "" {
		args = append(args, "--continue")
	}
	args = append(args, "--prompt", prompt)
	return args
}

func (a *OpenCodeAdapter) runCommand(ctx context.Context, sessionID, repoPath, prompt string) (string, error) {
	args := a.buildArgs(sessionID, prompt)
	cmd := exec.CommandContext(ctx, a.binary, args...)
	cmd.Dir = repoPath

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("opencode CLI: %s: %w", string(output), err)
	}
	return string(output), nil
}
