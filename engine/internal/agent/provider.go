package agent

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/loongxjin/forksync/engine/pkg/types"
)

// AgentProvider defines the interface for interacting with an AI coding agent CLI.
// Each supported agent (Claude, OpenCode, Codex) implements this interface.
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
// language is "zh" or "en", defaults to Chinese if unrecognized.
func BuildConflictPrompt(files []string, strategy string, language string) string {
	var sb strings.Builder

	sb.WriteString(conflictFileList(files, language))
	sb.WriteString(mergeStrategyText(strategy, language))
	sb.WriteString(resolutionMethod(language))
	sb.WriteString(requirements(language))

	return sb.String()
}

func conflictFileList(files []string, language string) string {
	var sb strings.Builder
	if language == "en" {
		sb.WriteString("The following files have merge conflicts. Please resolve them:\n\n")
	} else {
		sb.WriteString("以下文件存在合并冲突，请解决它们：\n\n")
	}
	for _, f := range files {
		sb.WriteString(fmt.Sprintf("  %s\n", f))
	}
	return sb.String()
}

func  mergeStrategyText(strategy string, language string) string {
	var sb strings.Builder
	en := language == "en"

	if en {
		sb.WriteString("\n## Merge Strategy\n\n")
	} else {
		sb.WriteString("\n## 合并策略\n\n")
	}

	switch strategy {
	case types.ResolveStrategyPreserveOurs:
		if en {
			sb.WriteString("Preserve our custom modifications and accept upstream non-conflicting changes.\n")
			sb.WriteString("When both sides conflict irreconcilably, prefer the local (ours) version.\n")
		} else {
			sb.WriteString("保留我们的自定义修改，接受上游的非冲突变更。\n")
			sb.WriteString("当双方的修改矛盾不可调和，优先保留本地（ours）的版本。\n")
		}
	case types.ResolveStrategyPreserveTheirs:
		if en {
			sb.WriteString("Prefer upstream changes and keep local modifications only where necessary.\n")
			sb.WriteString("When both sides conflict irreconcilably, prefer the upstream (theirs) version.\n")
		} else {
			sb.WriteString("优先接受上游的变更，仅在必要处保留本地修改。\n")
			sb.WriteString("当双方的修改矛盾不可调和，优先采用上游（theirs）的版本。\n")
		}
	case types.ResolveStrategyBalanced:
		if en {
			sb.WriteString("Merge intelligently, preserving changes from both sides where possible.\n")
			sb.WriteString("Only choose between them when both modified the exact same lines.\n")
		} else {
			sb.WriteString("智能合并，尽量同时保留双方的修改。\n")
			sb.WriteString("只有当双方修改了完全相同的行且无法自动整合时才需要取舍。\n")
		}
	default:
		if en {
			sb.WriteString("Preserve our custom modifications and accept upstream non-conflicting changes.\n")
		} else {
			sb.WriteString("保留我们的自定义修改，接受上游的非冲突变更。\n")
		}
	}
	return sb.String()
}

func resolutionMethod(language string) string {
	en := language == "en"
	var sb strings.Builder

	if en {
		sb.WriteString("\n## How to Resolve\n\n")
		sb.WriteString("Do NOT rush to edit files immediately. Follow this process:\n\n")
		sb.WriteString("1. For each conflicting file, first read the file to understand the conflict blocks.\n")
		sb.WriteString("2. Use `git log` on the conflicting file to find the commits that introduced each side:\n")
		sb.WriteString("   - For the local side: `git log HEAD...MERGE_HEAD -- <file>`\n")
		sb.WriteString("   - For the upstream side: `git log MERGE_HEAD...HEAD -- <file>`\n")
		sb.WriteString("3. Use `git show <commit>` or `git log -p` to read the full diff and commit message.\n")
		sb.WriteString("4. Understand WHY each change was made — what problem it solved, what feature it added.\n")
		sb.WriteString("5. After understanding both sides, resolve the conflict:\n")
		sb.WriteString("   - If preserving local: keep the local logic intact, but also adopt any upstream improvements that do not conflict\n")
		sb.WriteString("   - If accepting upstream: adopt the upstream logic, but keep any local additions whose purpose is still valid\n")
		sb.WriteString("   - If balanced: merge both sets of logic gracefully, preserving each side's intent\n")
		sb.WriteString("6. After resolving, verify the file compiles / passes syntax checks if possible.\n")
		sb.WriteString("7. Finally, briefly report what you did and why.\n")
	} else {
		sb.WriteString("\n## 解决方法\n\n")
		sb.WriteString("不要急于直接编辑文件。请按以下步骤进行：\n\n")
		sb.WriteString("1. 先阅读每个冲突文件，理解冲突块的内容。\n")
		sb.WriteString("2. 使用 `git log` 查找冲突双方各自的提交记录：\n")
		sb.WriteString("   - 查看本地修改的提交：`git log HEAD...MERGE_HEAD -- <文件>`\n")
		sb.WriteString("   - 查看上游修改的提交：`git log MERGE_HEAD...HEAD -- <文件>`\n")
		sb.WriteString("3. 使用 `git show <提交>` 或 `git log -p` 查看完整的改动和提交信息。\n")
		sb.WriteString("4. 理解每次修改的意图——它解决了什么问题、实现了什么功能、为什么要这样改。\n")
		sb.WriteString("5. 在充分理解双方意图之后，再进行冲突解决：\n")
		sb.WriteString("   - 保留本地时：保持本地逻辑完整，同时采纳上游不冲突的改进\n")
		sb.WriteString("   - 接受上游时：采用上游逻辑，但保留仍有效的本地补充功能\n")
		sb.WriteString("   - 智能合并时：优雅整合双方逻辑，不丢失任何一方的有效意图\n")
		sb.WriteString("6. 解决完成后，如果可能，检查文件能否通过编译或语法检查。\n")
		sb.WriteString("7. 最后，简要报告你做了什么、为什么这样解决。\n")
	}
	return sb.String()
}

func requirements(language string) string {
	en := language == "en"
	var sb strings.Builder

	if en {
		sb.WriteString("\n## Requirements\n\n")
		sb.WriteString("1. Remove all conflict markers (<<<<<<<, =======, >>>>>>>) and keep the correct code\n")
		sb.WriteString("2. Ensure the resolved code is syntactically correct and logically complete\n")
		sb.WriteString("3. Stay consistent with the existing code style of the project\n")
		sb.WriteString("4. Do not introduce unrelated changes\n")
		sb.WriteString("5. Do NOT run git add or git commit — files will be staged automatically\n")
	} else {
		sb.WriteString("\n## 要求\n\n")
		sb.WriteString("1. 移除所有冲突标记（<<<<<<<, =======, >>>>>>>）并保留正确的代码\n")
		sb.WriteString("2. 确保解决后的代码语法正确、逻辑完整\n")
		sb.WriteString("3. 保持与项目现有代码风格一致\n")
		sb.WriteString("4. 不要引入任何无关的修改\n")
		sb.WriteString("5. 不要执行 git add 或 git commit，文件会被自动暂存\n")
	}
	return sb.String()
}

// BuildInitialConflictPrompt is used for the first call on a new session.
// It combines the system-level role definition with the actual conflict task
// into a single prompt, so the agent receives the full context and task together
// and does not start working prematurely.
func BuildInitialConflictPrompt(conflictFiles []string, strategy string, language string) string {
	var sb strings.Builder

	if language == "en" {
		sb.WriteString("You are a professional Git merge conflict resolver. You are handling merge conflicts between a fork repository and its upstream.\n\n")
		sb.WriteString("## Your Role\n\n")
		sb.WriteString("Your job is not to mechanically pick one side or the other. You must understand WHY each side made its changes — by reading commit history and commit messages — and then make an informed, intelligent resolution that preserves the intent of both sides.\n\n")
	} else {
		sb.WriteString("你是一个专业的 Git 合并冲突解决助手。你正在处理一个 fork 仓库与上游仓库之间的合并冲突。\n\n")
		sb.WriteString("## 你的角色\n\n")
		sb.WriteString("你的任务不是机械地选择冲突的某一方，而是通过查阅提交历史和提交信息，理解每一处修改背后的原因和意图，然后做出有依据的、优雅的合并决策，保留双方代码的真正价值。\n\n")
	}

	sb.WriteString(BuildConflictPrompt(conflictFiles, strategy, language))

	return sb.String()
}

// StripANSI removes ANSI escape sequences (e.g. color codes) from a string.
// Some agents like OpenCode emit colored output that would render as "[0m"
// in the terminal drawer.
var ansiRegexp = regexp.MustCompile("\x1b\\[[0-9;]*m")

func StripANSI(s string) string {
	return ansiRegexp.ReplaceAllString(s, "")
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

// maxSummaryLength is the maximum character length for agent summary output.
const maxSummaryLength = 500

// truncateOutput limits output to maxLen runes for summaries.
func truncateOutput(output string, maxLen int) string {
	if utf8.RuneCountInString(output) <= maxLen {
		return output
	}
	runes := []rune(output)
	return string(runes[:maxLen]) + "..."
}

// truncateForLog truncates a string for safe use in log output.
func truncateForLog(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
