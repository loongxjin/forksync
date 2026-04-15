package summarizer

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// DefaultTimeout for agent CLI invocations.
const DefaultTimeout = 3 * time.Minute

// Executor invokes agent CLIs to generate summaries.
type Executor struct {
	timeout time.Duration
}

// NewExecutor creates a new Executor with the default timeout.
func NewExecutor() *Executor {
	return &Executor{timeout: DefaultTimeout}
}

// NewExecutorWithTimeout creates a new Executor with a custom timeout.
func NewExecutorWithTimeout(timeout time.Duration) *Executor {
	return &Executor{timeout: timeout}
}

// Summarize calls the specified agent CLI to generate a summary of the given commits.
// agentName is the binary name (e.g. "claude", "opencode"). If empty, returns an error.
func (e *Executor) Summarize(ctx context.Context, commits []CommitInfo, lang string, agentName string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()

	prompt := BuildPrompt(commits, lang)

	switch agentName {
	case "claude":
		return e.runClaude(ctx, prompt)
	case "opencode":
		return e.runOpenCode(ctx, prompt)
	default:
		return "", fmt.Errorf("unsupported summary agent: %s", agentName)
	}
}

// runClaude invokes Claude Code CLI in --print mode.
func (e *Executor) runClaude(ctx context.Context, prompt string) (string, error) {
	args := []string{
		"--print",
		"--dangerously-skip-permissions",
		prompt,
	}

	cmd := exec.CommandContext(ctx, "claude", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("claude CLI: %s: %w", strings.TrimSpace(string(output)), err)
	}

	result := strings.TrimSpace(string(output))
	return StripMarkdownBlocks(result), nil
}

// runOpenCode invokes OpenCode CLI in non-interactive mode.
func (e *Executor) runOpenCode(ctx context.Context, prompt string) (string, error) {
	args := []string{
		"run",
		"--non-interactive",
		prompt,
	}

	cmd := exec.CommandContext(ctx, "opencode", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("opencode CLI: %s: %w", strings.TrimSpace(string(output)), err)
	}

	result := strings.TrimSpace(string(output))
	return StripMarkdownBlocks(result), nil
}

// IsAgentAvailable checks if the given agent binary is available on PATH.
func IsAgentAvailable(agentName string) bool {
	_, err := exec.LookPath(agentName)
	return err == nil
}
