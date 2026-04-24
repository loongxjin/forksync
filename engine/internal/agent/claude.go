package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"
)

// ClaudeAdapter implements AgentProvider for Claude Code CLI.
//
// Invocation patterns:
//   - New session:  claude --print --dangerously-skip-permissions --output-format json <prompt>
//   - Resume:       claude --print --dangerously-skip-permissions --output-format json --resume <session-id> <prompt>
//
// Claude Code CLI flags:
//   - --print: non-interactive output mode
//   - --dangerously-skip-permissions: autonomous mode (no approval prompts)
//   - --output-format json: structured output containing session_id
//   - --resume <session-id>: resume an existing session
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
	// Send a minimal placeholder prompt just to obtain a session ID from
	// the Claude CLI. The real task prompt is sent later via ResolveConflicts.
	result, err := a.runCommandNew(ctx, opts.RepoPath, "ok")
	if err != nil {
		return nil, fmt.Errorf("claude start session: %w", err)
	}

	if result.SessionID == "" {
		return nil, fmt.Errorf("claude CLI did not return a session_id")
	}

	return &Session{
		ID:        result.SessionID,
		Provider:  "claude",
		RepoPath:  opts.RepoPath,
		StartedAt: time.Now(),
		IsNew:     true,
	}, nil
}

func (a *ClaudeAdapter) ResolveConflicts(ctx context.Context, session *Session, prompt string) (*AgentResult, error) {
	result, err := a.runCommandResume(ctx, session.ID, session.RepoPath, prompt)
	if err != nil {
		return &AgentResult{
			Success:   false,
			SessionID: session.ID,
			Summary:   fmt.Sprintf("claude error: %v", err),
		}, fmt.Errorf("claude resolve: %w", err)
	}

	return &AgentResult{
		Success:   true,
		SessionID: session.ID,
		Summary:   truncateOutput(result.Text, maxSummaryLength),
	}, nil
}

func (a *ClaudeAdapter) EndSession(ctx context.Context, sessionID string) error {
	// Claude Code sessions don't need explicit cleanup
	return nil
}

// claudeJSONResult represents the JSON output from Claude Code CLI with --output-format json.
type claudeJSONResult struct {
	Type      string `json:"type"`
	SessionID string `json:"session_id"`
	Result    string `json:"result"`
	IsError   bool   `json:"is_error"`
}

// claudeOutput holds the parsed result from a Claude CLI invocation.
type claudeOutput struct {
	SessionID string
	Text      string
}

// runCommandNew starts a NEW session (no --resume, no --session-id).
// Claude CLI assigns the session ID and returns it in JSON output.
func (a *ClaudeAdapter) runCommandNew(ctx context.Context, repoPath, prompt string) (*claudeOutput, error) {
	args := []string{
		"--print",
		"--dangerously-skip-permissions",
		"--output-format", "json",
		prompt,
	}
	return a.execClaude(ctx, repoPath, args)
}

// runCommandResume resumes an EXISTING session with --resume.
func (a *ClaudeAdapter) runCommandResume(ctx context.Context, sessionID, repoPath, prompt string) (*claudeOutput, error) {
	args := []string{
		"--print",
		"--dangerously-skip-permissions",
		"--output-format", "json",
		"--resume", sessionID,
		prompt,
	}
	return a.execClaude(ctx, repoPath, args)
}

func (a *ClaudeAdapter) execClaude(ctx context.Context, repoPath string, args []string) (*claudeOutput, error) {
	cmd := exec.CommandContext(ctx, a.binary, args...)
	cmd.Dir = repoPath

	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("claude CLI: %s: %w", string(output), err)
	}

	// Parse JSON output
	var result claudeJSONResult
	if jsonErr := json.Unmarshal(output, &result); jsonErr != nil {
		return nil, fmt.Errorf("claude CLI: failed to parse JSON output: %w\nraw: %s", jsonErr, string(output))
	}

	if result.IsError {
		return nil, fmt.Errorf("claude CLI returned error: %s", result.Result)
	}

	return &claudeOutput{
		SessionID: result.SessionID,
		Text:      result.Result,
	}, nil
}
