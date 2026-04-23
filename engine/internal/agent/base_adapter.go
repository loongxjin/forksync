package agent

import (
	"context"
	"fmt"
	"os/exec"
	"time"
)

// baseAdapter provides shared logic for simple CLI-based agent adapters
// (Droid, OpenCode, Codex) that follow the same pattern:
//   - build CLI args with optional session ID
//   - exec the binary and capture combined output
//   - extract session ID from text output
//
// ClaudeAdapter is NOT based on this because it uses JSON structured output.
//
// Each adapter embeds baseAdapter and implements buildArgs to provide
// the agent-specific CLI arguments. The sessionID parameter is used
// to decide whether to include session resumption flags.
type baseAdapter struct {
	binary string
	name   string
}

func (a *baseAdapter) Name() string { return a.name }

func (a *baseAdapter) IsAvailable() bool {
	_, err := exec.LookPath(a.binary)
	return err == nil
}

func (a *baseAdapter) StartSession(ctx context.Context, opts SessionOptions, buildArgs func(sessionID, prompt string) []string) (*Session, error) {
	args := buildArgs("", "ok")
	output, err := a.execCLI(ctx, opts.RepoPath, args)
	if err != nil {
		return nil, fmt.Errorf("%s start session: %w", a.name, err)
	}

	return &Session{
		ID:        extractSessionID(output),
		Provider:  a.name,
		RepoPath:  opts.RepoPath,
		StartedAt: time.Now(),
		IsNew:     true,
	}, nil
}

func (a *baseAdapter) ResolveConflicts(ctx context.Context, session *Session, prompt string, buildArgs func(sessionID, prompt string) []string) (*AgentResult, error) {
	args := buildArgs(session.ID, prompt)
	output, err := a.execCLI(ctx, session.RepoPath, args)
	if err != nil {
		return &AgentResult{
			Success:   false,
			SessionID: session.ID,
			Summary:   fmt.Sprintf("%s error: %v", a.name, err),
		}, fmt.Errorf("%s resolve: %w", a.name, err)
	}

	sessionID := extractSessionID(output)
	if sessionID == "" {
		sessionID = session.ID
	}

	return &AgentResult{
		Success:   true,
		SessionID: sessionID,
		Summary:   truncateOutput(output, maxSummaryLength),
	}, nil
}

func (a *baseAdapter) EndSession(_ context.Context, _ string) error {
	return nil
}

// execCLI runs the agent binary with the given args in the specified directory.
func (a *baseAdapter) execCLI(ctx context.Context, repoPath string, args []string) (string, error) {
	cmd := exec.CommandContext(ctx, a.binary, args...)
	cmd.Dir = repoPath

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s CLI: %s: %w", a.name, string(output), err)
	}
	return string(output), nil
}
