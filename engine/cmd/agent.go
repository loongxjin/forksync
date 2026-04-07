package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/loongxjin/forksync/engine/internal/agent"
	"github.com/loongxjin/forksync/engine/internal/agent/session"
	"github.com/loongxjin/forksync/engine/pkg/types"
	"github.com/spf13/cobra"
)

var agentCmd = &cobra.Command{
	Use:   "agent",
	Short: "Manage AI agent integrations",
	Long:  "Detect, list, and manage AI agent CLI integrations (Claude Code, OpenCode, Droid, Codex).",
}

var agentListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available AI agents",
	Long:  "Detect and list all supported AI agent CLIs installed on this system.",
	RunE:  runAgentList,
}

var agentSessionsCmd = &cobra.Command{
	Use:   "sessions",
	Short: "List active agent sessions",
	Long:  "Show all agent sessions currently tracked by ForkSync.",
	RunE:  runAgentSessions,
}

var agentCleanupCmd = &cobra.Command{
	Use:   "cleanup",
	Short: "Remove expired agent sessions",
	Long:  "Clean up expired and failed agent session records.",
	RunE:  runAgentCleanup,
}

func init() {
	rootCmd.AddCommand(agentCmd)
	agentCmd.AddCommand(agentListCmd)
	agentCmd.AddCommand(agentSessionsCmd)
	agentCmd.AddCommand(agentCleanupCmd)
}

func runAgentList(cmd *cobra.Command, args []string) error {
	registry := agent.NewRegistry("") // empty preferred → auto-discover
	agents := registry.Discover()

	// For the list, show ALL agents (installed + not) for discovery purposes
	allAgents := registry.ListAll()

	// Find preferred agent
	preferred := ""
	for _, a := range agents {
		if a.Installed {
			preferred = a.Name
			break
		}
	}

	if isJSON() {
		outputJSON(types.AgentListData{
			Agents:    allAgents,
			Preferred: preferred,
		}, nil)
	} else {
		if len(allAgents) == 0 {
			outputText("No agent CLIs detected.")
			outputText("")
			outputText("Install one of the following to enable auto-conflict resolution:")
			outputText("  • Claude Code: https://docs.anthropic.com/en/docs/claude-code")
			outputText("  • OpenCode:    https://github.com/opencode-ai/opencode")
			outputText("  • Droid:       https://github.com/nicepkg/droid")
			outputText("  • Codex:       https://github.com/openai/codex")
			return nil
		}

		outputText("AI Agent CLIs:")
		outputText("")
		for _, a := range allAgents {
			icon := "❌"
			status := "not installed"
			if a.Installed {
				icon = "✅"
				status = "installed"
				if a.Version != "" {
					status = fmt.Sprintf("installed (%s)", a.Version)
				}
			}
			outputText("  %s %-12s %s", icon, a.Name, status)
			if a.Path != "" {
				outputText("     Path: %s", a.Path)
			}
		}

		if preferred != "" {
			outputText("")
			outputText("Preferred agent: %s", preferred)
		}
	}

	return nil
}

func runAgentSessions(cmd *cobra.Command, args []string) error {
	home, _ := os.UserHomeDir()
	sessionDir := filepath.Join(home, ".forksync", "sessions")

	store := session.NewSessionStore(sessionDir)
	mgr := session.NewManager(store, nil) // provider not needed for listing
	infos, err := mgr.ListSessionsAsInfo()
	if err != nil {
		return fmt.Errorf("list sessions: %w", err)
	}

	if isJSON() {
		outputJSON(types.AgentSessionsData{
			Sessions: infos,
		}, nil)
	} else {
		if len(infos) == 0 {
			outputText("No active agent sessions.")
			return nil
		}

		outputText("Agent Sessions (%d):", len(infos))
		outputText("")
		for _, s := range infos {
			statusIcon := "🟢"
			switch s.Status {
			case "expired":
				statusIcon = "⏰"
			case "failed":
				statusIcon = "❌"
			}
			sid := s.ID
			if len(sid) > 16 {
				sid = sid[:16]
			}
			outputText("  %s %s (%s)", statusIcon, sid, s.AgentName)
			outputText("     Repo: %s", s.RepoID)
			outputText("     Status: %s | Last used: %s", s.Status, s.LastUsedAt.Format("2006-01-02 15:04"))
		}
	}

	return nil
}

func runAgentCleanup(cmd *cobra.Command, args []string) error {
	home, _ := os.UserHomeDir()
	sessionDir := filepath.Join(home, ".forksync", "sessions")

	store := session.NewSessionStore(sessionDir)
	mgr := session.NewManager(store, nil)
	count, err := mgr.CleanupExpired()
	if err != nil {
		return fmt.Errorf("cleanup sessions: %w", err)
	}

	if isJSON() {
		outputJSON(map[string]interface{}{
			"removed": count,
		}, nil)
	} else {
		outputText("Cleaned up %d expired/failed session(s).", count)
	}

	return nil
}
