package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/loongxjin/forksync/engine/internal/logger"
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

// ResolveConflictsWithStream runs Claude with real-time streaming output.
// Claude emits JSON at the end; we stream the content of the "result" field
// as stdout lines while the process runs.
func (a *ClaudeAdapter) ResolveConflictsWithStream(ctx context.Context, session *Session, prompt string, sw *StreamWriter) (*AgentResult, error) {
	args := []string{
		"--print",
		"--dangerously-skip-permissions",
		"--output-format", "json",
		"--resume", session.ID,
		prompt,
	}
	logger.Info("claude: starting streamed resolve", "repo", session.RepoPath, "session", session.ID)

	cmd := exec.CommandContext(ctx, a.binary, args...)
	cmd.Dir = session.RepoPath

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("claude stdout pipe: %w", err)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("claude stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("claude start: %w", err)
	}
	logger.Debug("claude: process started", "pid", cmd.Process.Pid)

	var stdoutBuilder strings.Builder
	var wg sync.WaitGroup

	// Emit start event
	_ = sw.WriteEvent(StreamEvent{
		Type:      StreamEventStart,
		Agent:     a.Name(),
		Timestamp: time.Now().UTC(),
	})

	// Scan stdout — accumulate raw output for JSON parsing at the end
	wg.Add(1)
	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stdoutPipe)
		for scanner.Scan() {
			line := scanner.Text()
			stdoutBuilder.WriteString(line)
			stdoutBuilder.WriteByte('\n')
			// We don't know yet if this line is part of the final JSON blob.
			// Emit it as stdout so the user sees progress (Claude sometimes
			// prints intermediate text before the JSON).
			_ = sw.WriteEvent(StreamEvent{
				Type:      StreamEventStdout,
				Data:      line,
				Timestamp: time.Now().UTC(),
			})
		}
		if err := scanner.Err(); err != nil {
			logger.Warn("claude: stdout scanner error", "error", err)
		}
		logger.Debug("claude: stdout scanner done")
	}()

	// Scan stderr
	wg.Add(1)
	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stderrPipe)
		for scanner.Scan() {
			line := scanner.Text()
			_ = sw.WriteEvent(StreamEvent{
				Type:      StreamEventStderr,
				Data:      line,
				Timestamp: time.Now().UTC(),
			})
		}
		if err := scanner.Err(); err != nil {
			logger.Warn("claude: stderr scanner error", "error", err)
		}
		logger.Debug("claude: stderr scanner done")
	}()

	// Wait for process and scanners
	waitErr := cmd.Wait()
	wg.Wait()

	// Parse the final JSON output from accumulated stdout
	stdoutRaw := stdoutBuilder.String()
	var parsed claudeJSONResult
	jsonErr := json.Unmarshal([]byte(stdoutRaw), &parsed)

	if waitErr != nil {
		logger.Warn("claude: process exited with error", "error", waitErr)
		_ = sw.WriteEvent(StreamEvent{
			Type:      StreamEventError,
			Data:      fmt.Sprintf("claude CLI: %v", waitErr),
			Timestamp: time.Now().UTC(),
			Success:   false,
		})
		return &AgentResult{
			Success:   false,
			SessionID: session.ID,
			Summary:   fmt.Sprintf("claude error: %v", waitErr),
		}, fmt.Errorf("claude resolve: %w", waitErr)
	}

	if jsonErr != nil {
		logger.Error("claude: JSON parse error", "error", jsonErr, "rawLength", len(stdoutRaw))
		_ = sw.WriteEvent(StreamEvent{
			Type:      StreamEventError,
			Data:      fmt.Sprintf("claude JSON parse error: %v", jsonErr),
			Timestamp: time.Now().UTC(),
			Success:   false,
		})
		return &AgentResult{
			Success:   false,
			SessionID: session.ID,
			Summary:   fmt.Sprintf("claude JSON parse error: %v", jsonErr),
		}, fmt.Errorf("claude resolve: failed to parse JSON: %w", jsonErr)
	}

	if parsed.IsError {
		logger.Error("claude: agent returned error", "result", parsed.Result)
		_ = sw.WriteEvent(StreamEvent{
			Type:      StreamEventError,
			Data:      fmt.Sprintf("claude returned error: %s", parsed.Result),
			Timestamp: time.Now().UTC(),
			Success:   false,
		})
		return &AgentResult{
			Success:   false,
			SessionID: session.ID,
			Summary:   fmt.Sprintf("claude error: %s", parsed.Result),
		}, fmt.Errorf("claude resolve: %s", parsed.Result)
	}

	// Split the result text by line and emit as stdout events
	// (overwriting the raw JSON lines we emitted earlier, so the user sees
	// clean agent output rather than raw JSON).
	resultLines := strings.Split(parsed.Result, "\n")
	for _, line := range resultLines {
		if line == "" {
			continue
		}
		_ = sw.WriteEvent(StreamEvent{
			Type:      StreamEventStdout,
			Data:      line,
			Timestamp: time.Now().UTC(),
		})
	}

	summary := truncateOutput(parsed.Result, maxSummaryLength)
	logger.Info("claude: streamed resolve completed", "sessionID", parsed.SessionID)
	_ = sw.WriteEvent(StreamEvent{
		Type:      StreamEventDone,
		Timestamp: time.Now().UTC(),
		Success:   true,
		Summary:   summary,
		SessionID: parsed.SessionID,
	})

	return &AgentResult{
		Success:   true,
		SessionID: parsed.SessionID,
		Summary:   summary,
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
