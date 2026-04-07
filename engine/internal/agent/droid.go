package agent

import (
	"context"
	"fmt"
	"os/exec"
	"time"
)

// DroidAdapter implements AgentProvider for Droid CLI (Factory).
//
// Invocation: droid exec --auto high [--session-id <id>] [--append-system-prompt <text>] <prompt>
//
// Droid CLI uses the "exec" subcommand for non-interactive execution.
// The --auto flag controls autonomy level: low, medium, high.
// For conflict resolution, "high" is needed to allow file modifications and git operations.
type DroidAdapter struct {
	binary string
}

func NewDroidAdapter() *DroidAdapter {
	return &DroidAdapter{binary: "droid"}
}

func (a *DroidAdapter) Name() string { return "droid" }

func (a *DroidAdapter) IsAvailable() bool {
	_, err := exec.LookPath(a.binary)
	return err == nil
}

func (a *DroidAdapter) StartSession(ctx context.Context, opts SessionOptions) (*Session, error) {
	contextPrompt := buildContextInjectionPrompt(opts)
	output, err := a.runCommand(ctx, "", opts.RepoPath, contextPrompt)
	if err != nil {
		return nil, fmt.Errorf("droid start session: %w", err)
	}

	return &Session{
		ID:        extractSessionID(output),
		Provider:  "droid",
		RepoPath:  opts.RepoPath,
		StartedAt: time.Now(),
	}, nil
}

func (a *DroidAdapter) ResolveConflicts(ctx context.Context, session *Session, prompt string) (*AgentResult, error) {
	output, err := a.runCommand(ctx, session.ID, session.RepoPath, prompt)
	if err != nil {
		return &AgentResult{
			Success:   false,
			SessionID: session.ID,
			Summary:   fmt.Sprintf("droid error: %v", err),
		}, fmt.Errorf("droid resolve: %w", err)
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

func (a *DroidAdapter) EndSession(ctx context.Context, sessionID string) error {
	return nil
}

// buildArgs constructs the CLI arguments for a Droid exec invocation.
func (a *DroidAdapter) buildArgs(sessionID, prompt string) []string {
	args := []string{"exec", "--auto", "high"}
	if sessionID != "" {
		args = append(args, "--session-id", sessionID)
	}
	args = append(args, prompt)
	return args
}

func (a *DroidAdapter) runCommand(ctx context.Context, sessionID, repoPath, prompt string) (string, error) {
	args := a.buildArgs(sessionID, prompt)
	cmd := exec.CommandContext(ctx, a.binary, args...)
	cmd.Dir = repoPath

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("droid CLI: %s: %w", string(output), err)
	}
	return string(output), nil
}
