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
//   - --output-format stream-json: real-time NDJSON streaming output
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

// ResolveConflictsWithStream runs Claude with --output-format stream-json for
// real-time NDJSON streaming output. Each line on stdout is a JSON event with a
// "type" field (init, assistant, tool_use, tool_result, result, etc.).
//
// We parse these events and forward meaningful content (text, tool calls) as
// StreamEvents to the StreamWriter so the frontend can display live progress.
func (a *ClaudeAdapter) ResolveConflictsWithStream(ctx context.Context, session *Session, prompt string, sw *StreamWriter) (*AgentResult, error) {
	args := []string{
		"--print",
		"--dangerously-skip-permissions",
		"--output-format", "stream-json",
		"--verbose",
		"--resume", session.ID,
		prompt,
	}
	logger.Info("[TRACE] claude: ResolveConflictsWithStream START (stream-json mode)", "repo", session.RepoPath, "session", session.ID)

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
		logger.Error("[TRACE] claude: cmd.Start FAILED", "error", err)
		_ = sw.WriteEvent(StreamEvent{
			Type:      StreamEventError,
			Data:      fmt.Sprintf("failed to start claude: %v", err),
			Timestamp: time.Now().UTC(),
			Success:   false,
		})
		return nil, fmt.Errorf("claude start: %w", err)
	}
	logger.Info("[TRACE] claude: process started OK (stream-json)", "pid", cmd.Process.Pid)

	var (
		resultTextBuilder strings.Builder
		sessionID         string
		stdoutLineCount   int
		stderrLineCount   int
		wg                sync.WaitGroup
	)

	// Scan stdout — each line is a JSON event from stream-json output
	wg.Add(1)
	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stdoutPipe)
		// Increase buffer size for long lines
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.TrimSpace(line) == "" {
				continue
			}
			stdoutLineCount++
			logger.Info("[TRACE] claude: stream-json line", "lineNum", stdoutLineCount, "len", len(line), "preview", truncateForLog(line, 200))

			// Parse the stream-json event
			var ev map[string]any
			if jsonErr := json.Unmarshal([]byte(line), &ev); jsonErr != nil {
				logger.Warn("[TRACE] claude: failed to parse stream-json line", "lineNum", stdoutLineCount, "error", jsonErr)
				// Treat as raw stdout
				_ = sw.WriteEvent(StreamEvent{
					Type:      StreamEventStdout,
					Data:      line,
					Timestamp: time.Now().UTC(),
				})
				continue
			}

			eventType, _ := ev["type"].(string)

			switch eventType {
			case "init":
				// Session init — extract session_id
				if sid, ok := ev["session_id"].(string); ok && sid != "" {
					sessionID = sid
					logger.Info("[TRACE] claude: stream-json init", "sessionID", sid)
				}

			case "assistant":
				// Full assistant message — extract text content
				a.processAssistantEvent(sw, ev, &resultTextBuilder)

			case "tool_use":
				// Tool call — emit as tool event
				a.processToolUseEvent(sw, ev)

			case "tool_result":
				// Tool execution result — emit as stdout
				a.processToolResultEvent(sw, ev)

			case "stream_event":
				// Partial token streaming (if --include-partial-messages was used)
				// We emit these as stdout for real-time display
				a.processStreamEvent(sw, ev, &resultTextBuilder)

			case "result":
				// Final result — extract session_id and result text
				if sid, ok := ev["session_id"].(string); ok && sid != "" {
					sessionID = sid
				}
				if isErr, _ := ev["is_error"].(bool); isErr {
					errMsg := "unknown error"
					if r, ok := ev["result"].(string); ok {
						errMsg = r
					}
					_ = sw.WriteEvent(StreamEvent{
						Type:      StreamEventError,
						Data:      fmt.Sprintf("claude returned error: %s", truncateForLog(errMsg, 200)),
						Timestamp: time.Now().UTC(),
						Success:   false,
					})
				}
				logger.Info("[TRACE] claude: stream-json result event", "sessionID", sessionID)

			default:
				// Unknown event type — log and skip
				logger.Debug("[TRACE] claude: unhandled stream-json event type", "type", eventType)
			}
		}
		if err := scanner.Err(); err != nil {
			logger.Warn("[TRACE] claude: stdout scanner error", "error", err)
		}
		logger.Info("[TRACE] claude: stdout scanner DONE", "totalLines", stdoutLineCount)
	}()

	// Scan stderr
	wg.Add(1)
	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stderrPipe)
		for scanner.Scan() {
			line := scanner.Text()
			stderrLineCount++
			logger.Info("[TRACE] claude: stderr line", "lineNum", stderrLineCount, "preview", truncateForLog(line, 120))
			_ = sw.WriteEvent(StreamEvent{
				Type:      StreamEventStderr,
				Data:      line,
				Timestamp: time.Now().UTC(),
			})
		}
		if err := scanner.Err(); err != nil {
			logger.Warn("[TRACE] claude: stderr scanner error", "error", err)
		}
		logger.Info("[TRACE] claude: stderr scanner DONE", "totalLines", stderrLineCount)
	}()

	// Wait for process and scanners
	waitErr := cmd.Wait()
	wg.Wait()

	logger.Info("[TRACE] claude: process finished", "waitErr", waitErr, "stdoutLines", stdoutLineCount, "stderrLines", stderrLineCount, "sessionID", sessionID)

	if waitErr != nil {
		logger.Warn("[TRACE] claude: process exited with error", "error", waitErr)
		_ = sw.WriteEvent(StreamEvent{
			Type:      StreamEventError,
			Data:      fmt.Sprintf("claude CLI: %v", waitErr),
			Timestamp: time.Now().UTC(),
			Success:   false,
		})
		return &AgentResult{
			Success:   false,
			SessionID: sessionID,
			Summary:   fmt.Sprintf("claude error: %v", waitErr),
		}, fmt.Errorf("claude resolve: %w", waitErr)
	}

	if sessionID == "" {
		sessionID = session.ID
	}

	resultText := resultTextBuilder.String()
	summary := truncateOutput(resultText, maxSummaryLength)

	_ = sw.WriteEvent(StreamEvent{
		Type:      StreamEventDone,
		Timestamp: time.Now().UTC(),
		Success:   true,
		Summary:   summary,
		SessionID: sessionID,
	})

	logger.Info("[TRACE] claude: streamed resolve completed", "sessionID", sessionID, "resultLen", len(resultText))

	return &AgentResult{
		Success:   true,
		SessionID: sessionID,
		Summary:   summary,
	}, nil
}

// processAssistantEvent extracts text from an "assistant" stream-json event
// and emits it as stdout StreamEvents.
func (a *ClaudeAdapter) processAssistantEvent(sw *StreamWriter, ev map[string]any, builder *strings.Builder) {
	// "assistant" events have a "message" field with "content" array
	message, _ := ev["message"].(map[string]any)
	if message == nil {
		return
	}
	content, _ := message["content"].([]any)
	for _, block := range content {
		cb, ok := block.(map[string]any)
		if !ok {
			continue
		}
		blockType, _ := cb["type"].(string)
		switch blockType {
		case "text":
			if text, ok := cb["text"].(string); ok && text != "" {
				builder.WriteString(text)
				// Split multi-line text into separate events
				for _, line := range strings.Split(text, "\n") {
					_ = sw.WriteEvent(StreamEvent{
						Type:      StreamEventStdout,
						Data:      line,
						Timestamp: time.Now().UTC(),
					})
				}
			}
		case "tool_use":
			name, _ := cb["name"].(string)
			input, _ := cb["input"].(map[string]any)
			var path string
			if input != nil {
				if p, ok := input["file_path"].(string); ok {
					path = p
				} else if p, ok := input["path"].(string); ok {
					path = p
				}
			}
			_ = sw.WriteEvent(StreamEvent{
				Type:      StreamEventTool,
				Timestamp: time.Now().UTC(),
				ToolName:  name,
				ToolPath:  path,
			})
		}
	}
}

// processToolUseEvent emits a tool StreamEvent from a "tool_use" stream-json event.
func (a *ClaudeAdapter) processToolUseEvent(sw *StreamWriter, ev map[string]any) {
	name, _ := ev["name"].(string)
	input, _ := ev["input"].(map[string]any)
	var path string
	if input != nil {
		if p, ok := input["file_path"].(string); ok {
			path = p
		} else if p, ok := input["path"].(string); ok {
			path = p
		}
	}
	_ = sw.WriteEvent(StreamEvent{
		Type:      StreamEventTool,
		Timestamp: time.Now().UTC(),
		ToolName:  name,
		ToolPath:  path,
	})
}

// processToolResultEvent emits stdout from a "tool_result" stream-json event.
func (a *ClaudeAdapter) processToolResultEvent(sw *StreamWriter, ev map[string]any) {
	content, _ := ev["content"].(string)
	if content == "" {
		// content may be an array
		if contentArr, ok := ev["content"].([]any); ok {
			var parts []string
			for _, c := range contentArr {
				if cm, ok := c.(map[string]any); ok {
					if t, ok := cm["text"].(string); ok {
						parts = append(parts, t)
					}
				}
			}
			content = strings.Join(parts, "\n")
		}
	}
	if content != "" {
		for _, line := range strings.Split(content, "\n") {
			if line == "" {
				continue
			}
			_ = sw.WriteEvent(StreamEvent{
				Type:      StreamEventStdout,
				Data:      line,
				Timestamp: time.Now().UTC(),
			})
		}
	}
}

// processStreamEvent handles partial token streaming events (stream_event type).
func (a *ClaudeAdapter) processStreamEvent(sw *StreamWriter, ev map[string]any, builder *strings.Builder) {
	inner, _ := ev["event"].(map[string]any)
	if inner == nil {
		return
	}
	delta, _ := inner["delta"].(map[string]any)
	if delta == nil {
		return
	}
	deltaType, _ := delta["type"].(string)
	if deltaType == "text_delta" {
		if text, ok := delta["text"].(string); ok && text != "" {
			builder.WriteString(text)
			_ = sw.WriteEvent(StreamEvent{
				Type:      StreamEventStdout,
				Data:      text,
				Timestamp: time.Now().UTC(),
			})
		}
	}
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
