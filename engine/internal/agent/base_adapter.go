package agent

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/loongxjin/forksync/engine/internal/logger"
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

// ResolveConflictsWithStream runs the agent with real-time streaming output.
// This is NOT part of the AgentProvider interface; it is called by session.Manager
// when a StreamWriter is provided.
func (a *baseAdapter) ResolveConflictsWithStream(ctx context.Context, session *Session, prompt string, buildArgs func(sessionID, prompt string) []string, sw *StreamWriter) (*AgentResult, error) {
	args := buildArgs(session.ID, prompt)
	logger.Info("baseAdapter: starting streamed resolve", "agent", a.name, "repo", session.RepoPath, "session", session.ID)

	cmd := exec.CommandContext(ctx, a.binary, args...)
	cmd.Dir = session.RepoPath

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("%s stdout pipe: %w", a.name, err)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("%s stderr pipe: %w", a.name, err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("%s start: %w", a.name, err)
	}
	logger.Debug("baseAdapter: agent process started", "agent", a.name, "pid", cmd.Process.Pid)

	var outputBuilder strings.Builder
	var wg sync.WaitGroup

	// Emit start event
	_ = sw.WriteEvent(StreamEvent{
		Type:      StreamEventStart,
		Agent:     a.name,
		Timestamp: time.Now().UTC(),
	})

	// Scan stdout
	wg.Add(1)
	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stdoutPipe)
		for scanner.Scan() {
			line := scanner.Text()
			outputBuilder.WriteString(line)
			outputBuilder.WriteByte('\n')
			_ = sw.WriteEvent(StreamEvent{
				Type:      StreamEventStdout,
				Data:      line,
				Timestamp: time.Now().UTC(),
			})
		}
		if err := scanner.Err(); err != nil {
			logger.Warn("baseAdapter: stdout scanner error", "agent", a.name, "error", err)
		}
		logger.Debug("baseAdapter: stdout scanner done", "agent", a.name)
	}()

	// Scan stderr
	wg.Add(1)
	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stderrPipe)
		for scanner.Scan() {
			line := scanner.Text()
			outputBuilder.WriteString(line)
			outputBuilder.WriteByte('\n')
			_ = sw.WriteEvent(StreamEvent{
				Type:      StreamEventStderr,
				Data:      line,
				Timestamp: time.Now().UTC(),
			})
		}
		if err := scanner.Err(); err != nil {
			logger.Warn("baseAdapter: stderr scanner error", "agent", a.name, "error", err)
		}
		logger.Debug("baseAdapter: stderr scanner done", "agent", a.name)
	}()

	// Wait for process and scanners
	waitErr := cmd.Wait()
	wg.Wait()

	output := outputBuilder.String()

	if waitErr != nil {
		logger.Warn("baseAdapter: agent process exited with error", "agent", a.name, "error", waitErr)
		_ = sw.WriteEvent(StreamEvent{
			Type:      StreamEventError,
			Data:      fmt.Sprintf("%s CLI: %s: %v", a.name, output, waitErr),
			Timestamp: time.Now().UTC(),
			Success:   false,
		})
		return &AgentResult{
			Success:   false,
			SessionID: session.ID,
			Summary:   fmt.Sprintf("%s error: %v", a.name, waitErr),
		}, fmt.Errorf("%s resolve: %w", a.name, waitErr)
	}

	sessionID := extractSessionID(output)
	if sessionID == "" {
		sessionID = session.ID
	}

	summary := truncateOutput(output, maxSummaryLength)
	logger.Info("baseAdapter: agent process completed", "agent", a.name, "sessionID", sessionID)
	_ = sw.WriteEvent(StreamEvent{
		Type:      StreamEventDone,
		Timestamp: time.Now().UTC(),
		Success:   true,
		Summary:   summary,
		SessionID: sessionID,
	})

	return &AgentResult{
		Success:   true,
		SessionID: sessionID,
		Summary:   summary,
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
