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

// agentArgs maps each supported summary agent to its base CLI arguments (before the prompt).
var summaryAgentArgs = map[string][]string{
	"claude":   {"--print", "--dangerously-skip-permissions"},
	"opencode": {"run"},
	"droid":    {"exec", "--auto", "high"},
	"codex":    {"exec", "--dangerously-bypass-approvals-and-sandbox"},
}

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
// agentName is the binary name (e.g. "claude", "opencode", "droid", "codex"). If empty, returns an error.
func (e *Executor) Summarize(ctx context.Context, commits []CommitInfo, lang string, agentName string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()

	baseArgs, ok := summaryAgentArgs[agentName]
	if !ok {
		return "", fmt.Errorf("unsupported summary agent: %s", agentName)
	}

	prompt := BuildPrompt(commits, lang)
	args := append(append([]string{}, baseArgs...), prompt)
	return e.runAgentCLI(ctx, agentName, args)
}

// runAgentCLI runs the agent binary with the given args and returns cleaned output.
func (e *Executor) runAgentCLI(ctx context.Context, binary string, args []string) (string, error) {
	cmd := exec.CommandContext(ctx, binary, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s CLI: %s: %w", binary, strings.TrimSpace(string(output)), err)
	}

	result := strings.TrimSpace(string(output))
	return StripMarkdownBlocks(result), nil
}

// IsAgentAvailable checks if the given agent binary is available on PATH.
func IsAgentAvailable(agentName string) bool {
	_, err := exec.LookPath(agentName)
	return err == nil
}
