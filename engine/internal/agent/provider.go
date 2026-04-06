package agent

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// AgentProvider defines the interface for interacting with an AI coding agent CLI.
// Each supported agent (Claude, OpenCode, Droid, Codex) implements this interface.
type AgentProvider interface {
	// Name returns the agent's identifier (e.g., "claude", "opencode").
	Name() string

	// IsAvailable checks whether the agent CLI binary is installed and accessible.
	IsAvailable() bool

	// StartSession starts a new agent session in the given repository.
	// The agent receives project context via the SessionOptions.
	StartSession(ctx context.Context, opts SessionOptions) (*Session, error)

	// ResolveConflicts sends a conflict resolution task to the agent.
	// The agent works directly in the repo directory, reading and editing files.
	ResolveConflicts(ctx context.Context, session *Session, prompt string) (*AgentResult, error)

	// EndSession terminates an active agent session and cleans up resources.
	EndSession(ctx context.Context, sessionID string) error
}

// SessionOptions contains parameters for creating a new agent session.
type SessionOptions struct {
	// RepoPath is the absolute path to the git repository.
	RepoPath string

	// RepoName is the display name of the repository.
	RepoName string

	// ContextFiles is a list of file paths (relative to RepoPath) to inject
	// as project context when starting a new session.
	// Examples: "README.md", "go.mod", "AGENTS.md"
	ContextFiles []string
}

// Session represents an active agent session for a specific repository.
type Session struct {
	// ID is the session identifier returned by the agent CLI.
	// Used to resume the session in subsequent calls.
	ID string

	// Provider is the name of the agent providing this session.
	Provider string

	// RepoPath is the absolute path to the repository this session is for.
	RepoPath string

	// StartedAt is the time the session was created.
	StartedAt time.Time
}

// AgentResult contains the output of an agent conflict resolution attempt.
type AgentResult struct {
	// Success indicates whether the agent successfully resolved all conflicts.
	Success bool

	// Diff contains the git diff output showing the agent's changes.
	Diff string

	// Summary is a human-readable description of what the agent did.
	Summary string

	// SessionID is the session identifier, potentially updated after resolution.
	SessionID string

	// ResolvedFiles lists the file paths that the agent modified.
	ResolvedFiles []string
}

// BuildConflictPrompt constructs the prompt sent to the agent for conflict resolution.
// Only file paths are provided — the agent reads files directly from disk.
func BuildConflictPrompt(files []string, strategy string) string {
	var sb strings.Builder

	sb.WriteString("以下文件存在合并冲突，请解决它们：\n")
	for _, f := range files {
		sb.WriteString(fmt.Sprintf("- %s\n", f))
	}

	sb.WriteString("\n策略：")
	switch strategy {
	case "preserve_ours":
		sb.WriteString("保留我们的自定义修改，接受上游的非冲突变更。")
	case "accept_theirs":
		sb.WriteString("优先接受上游的变更，仅在必要处保留本地修改。")
	case "balanced":
		sb.WriteString("智能合并，尽量同时保留双方的修改。")
	default:
		sb.WriteString("保留我们的自定义修改，接受上游的非冲突变更。")
	}

	sb.WriteString("\n\n解决后请确保：")
	sb.WriteString("\n1. 没有残留的冲突标记（<<<<<<<, =======, >>>>>>>）")
	sb.WriteString("\n2. 代码语法正确")
	sb.WriteString("\n3. 项目风格一致")

	return sb.String()
}

// buildContextInjectionPrompt creates the initial prompt for a new session.
// It injects project context files and a system-level instruction.
func buildContextInjectionPrompt(opts SessionOptions) string {
	var sb strings.Builder

	sb.WriteString("你是一个代码合并助手。你正在处理一个 fork 仓库的合并冲突。\n\n")
	sb.WriteString(fmt.Sprintf("项目名称：%s\n", opts.RepoName))

	if len(opts.ContextFiles) > 0 {
		sb.WriteString("\n项目上下文文件：\n")
		for _, f := range opts.ContextFiles {
			sb.WriteString(fmt.Sprintf("- %s\n", f))
		}
	}

	sb.WriteString("\n接下来我会让你解决合并冲突。请保持项目风格一致。")
	return sb.String()
}

// extractSessionID attempts to find a session ID from agent CLI output.
// Returns empty string if no session ID can be found.
func extractSessionID(output string) string {
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Session:") || strings.HasPrefix(line, "session_id:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1])
			}
		}
	}
	return ""
}

// truncateOutput limits output to maxLen characters for summaries.
func truncateOutput(output string, maxLen int) string {
	if len(output) <= maxLen {
		return output
	}
	return output[:maxLen] + "..."
}
