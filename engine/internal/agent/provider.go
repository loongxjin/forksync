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

	// IsNew indicates whether this session has not yet received a real task.
	// On the first ResolveConflicts call, the initial system prompt and the
	// conflict prompt are merged into one so the agent only starts work once.
	IsNew bool
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

	// AgentName is the name of the agent provider that produced this result.
	AgentName string
}

// BuildConflictPrompt constructs the prompt sent to the agent for conflict resolution.
// This is used when resuming an existing session that has already received
// the initial system prompt.
func BuildConflictPrompt(files []string, strategy string) string {
	var sb strings.Builder

	sb.WriteString("以下文件存在合并冲突，请解决它们：\n\n")
	for _, f := range files {
		sb.WriteString(fmt.Sprintf("  %s\n", f))
	}

	sb.WriteString("\n## 合并策略\n\n")
	switch strategy {
	case "preserve_ours":
		sb.WriteString("保留我们的自定义修改，接受上游的非冲突变更。\n")
		sb.WriteString("当双方的修改矛盾不可调和，优先保留本地（ours）的版本。\n")
	case "accept_theirs":
		sb.WriteString("优先接受上游的变更，仅在必要处保留本地修改。\n")
		sb.WriteString("当双方的修改矛盾不可调和，优先采用上游（theirs）的版本。\n")
	case "balanced":
		sb.WriteString("智能合并，尽量同时保留双方的修改。\n")
		sb.WriteString("只有当双方修改了完全相同的行且无法自动整合时才需要取舍。\n")
	default:
		sb.WriteString("保留我们的自定义修改，接受上游的非冲突变更。\n")
	}

	sb.WriteString("\n## 要求\n\n")
	sb.WriteString("1. 移除所有冲突标记（<<<<<<<, =======, >>>>>>>）并保留正确的代码\n")
	sb.WriteString("2. 确保解决后的代码语法正确、逻辑完整\n")
	sb.WriteString("3. 保持与项目现有代码风格一致\n")
	sb.WriteString("4. 不要引入任何无关的修改\n")

	return sb.String()
}

// BuildInitialConflictPrompt is used for the first call on a new session.
// It combines the system-level role definition with the actual conflict task
// into a single prompt, so the agent receives the full context and task together
// and does not start working prematurely.
func BuildInitialConflictPrompt(conflictFiles []string, strategy string) string {
	var sb strings.Builder

	sb.WriteString("你是一个专业的 Git 合并冲突解决助手。你正在处理一个 fork 仓库与上游仓库之间的合并冲突。\n\n")
	sb.WriteString("## 你的任务\n\n")
	sb.WriteString("仔细阅读每个冲突文件，理解冲突的原因和双方的意图，然后根据指定的合并策略解决所有冲突。\n\n")
	sb.WriteString("## 工作方式\n\n")
	sb.WriteString("- 直接读取并编辑文件，从磁盘上消除冲突\n")
	sb.WriteString("- 如果需要理解项目上下文，可以自行查看项目中的其他文件（如 README、配置文件等）\n")
	sb.WriteString("- 解决完所有冲突后，简要报告你做了什么\n\n")

	sb.WriteString(BuildConflictPrompt(conflictFiles, strategy))

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
