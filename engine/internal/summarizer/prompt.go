package summarizer

import (
	"fmt"
	"strings"
)

// CommitInfo represents a single git commit for summarization.
type CommitInfo struct {
	Hash    string
	Message string
}

// BuildPrompt creates a prompt for the AI agent to summarize git commits.
// lang should be "zh" or "en".
func BuildPrompt(commits []CommitInfo, lang string) string {
	var sb strings.Builder

	if lang == "zh" {
		sb.WriteString("请根据以下 git commits 列表，生成一段简洁的同步内容总结（3-5句话）。\n")
		sb.WriteString("总结要点：\n")
		sb.WriteString("- 添加了什么新功能\n")
		sb.WriteString("- 修复了什么问题\n")
		sb.WriteString("- 有哪些重要的变更\n\n")
	} else {
		sb.WriteString("Based on the following git commits, generate a concise summary of the sync changes (3-5 sentences).\n")
		sb.WriteString("Focus on:\n")
		sb.WriteString("- What new features were added\n")
		sb.WriteString("- What bugs were fixed\n")
		sb.WriteString("- Any important changes\n\n")
	}

	sb.WriteString("Commits:\n")
	for i, c := range commits {
		sb.WriteString(fmt.Sprintf("%d. [%s] %s\n", i+1, shortHash(c.Hash), c.Message))
	}

	if lang == "zh" {
		sb.WriteString("\n请直接输出总结内容，不要添加任何前缀或格式标记。")
	} else {
		sb.WriteString("\nPlease output the summary directly, without any prefixes or formatting markers.")
	}

	return sb.String()
}

// shortHash returns the first 8 characters of a git hash.
func shortHash(hash string) string {
	if len(hash) > 8 {
		return hash[:8]
	}
	return hash
}

// StripMarkdownBlocks removes leading/trailing markdown code block markers
// (```, ```text, etc.) from agent output.
func StripMarkdownBlocks(s string) string {
	s = strings.TrimSpace(s)
	// Remove leading code block markers like ``` or ```text
	for {
		trimmed := strings.TrimPrefix(s, "```")
		if trimmed == s {
			break
		}
		// Also strip the language tag on the same line
		if idx := strings.Index(trimmed, "\n"); idx >= 0 {
			s = trimmed[idx+1:]
		} else {
			s = trimmed
		}
	}
	// Remove trailing code block markers
	s = strings.TrimSuffix(s, "```")
	return strings.TrimSpace(s)
}
