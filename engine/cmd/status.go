package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/loongxjin/forksync/engine/internal/agent"
	syncpkg "github.com/loongxjin/forksync/engine/internal/sync"
	"github.com/loongxjin/forksync/engine/pkg/types"
	"github.com/spf13/cobra"
)

var statusExclude []string

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show status of all managed repositories",
	RunE:  runStatus,
}

func init() {
	rootCmd.AddCommand(statusCmd)
	statusCmd.Flags().StringSliceVar(&statusExclude, "exclude", nil, "comma-separated repo names to skip (e.g. --exclude repo1,repo2)")
}

// statusTimeout is the per-repo timeout for status operations (fetch + rev-list).
// 30 seconds is generous for a single repo; the overall command may take longer
// when there are many repos, but each individual operation is bounded.
const statusTimeout = 30 * time.Second

// staleWorkflowThreshold is the age after which an active workflow is considered stale.
const staleWorkflowThreshold = 30 * time.Minute

// resolvingStaleThreshold is the age after which a "resolving" status is considered
// stale and safe to recover. Must be longer than the max agent resolve timeout.
const resolvingStaleThreshold = 10 * time.Minute

// actionStatusUpdate is the log action label for repo status refresh updates.
const actionStatusUpdate = "status-update"

func runStatus(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithTimeout(cmd.Context(), statusTimeout)
	defer cancel()

	cfg, _ := getSharedConfig()

	store, err := loadRepoStore()
	if err != nil {
		return err
	}

	repos, err := store.List()
	if err != nil {
		return fmt.Errorf("list repos: %w", err)
	}

	gitOps := newGitOps(cfg)
	refresher := syncpkg.NewStatusRefresher(gitOps, store, cfg)

	repos, err = refresher.RefreshAll(ctx, repos, statusExclude)
	if err != nil {
		return fmt.Errorf("refresh status: %w", err)
	}

	// Detect installed agents
	registry := agent.NewRegistry("")
	agents := registry.Discover()

	// Determine preferred agent
	preferredAgent := ""
	if len(agents) > 0 {
		preferredAgent = agents[0].Name
	}

	if isJSON() {
		outputJSON(types.StatusData{
			Repos:          repos,
			Agents:         agents,
			PreferredAgent: preferredAgent,
		}, nil)
	} else {
		printStatusText(repos, agents, preferredAgent)
	}

	return nil
}


func printStatusText(repos []types.Repo, agents []types.AgentInfo, preferredAgent string) {
	if len(repos) == 0 {
		outputText("No repositories managed. Use 'forksync add <path>' to add one.")
	} else {
		outputText("Managed Repositories (%d):", len(repos))
		outputText("")
		for _, r := range repos {
			statusIcon := "⚪"
			switch r.Status {
			case types.RepoStatusUpToDate:
				statusIcon = "🟢"
			case types.RepoStatusSyncing:
				statusIcon = "🟡"
			case types.RepoStatusConflict:
				statusIcon = "🔴"
			case types.RepoStatusError:
				statusIcon = "❌"
			}

			outputText("  %s %s", statusIcon, r.Name)
			if r.Upstream != "" {
				outputText("     Upstream: %s", r.Upstream)
			}
			if r.BehindBy > 0 {
				outputText("     Behind by %d commits", r.BehindBy)
			}
			if r.AheadBy > 0 {
				outputText("     Ahead by %d commits", r.AheadBy)
			}
			if r.ErrorMessage != "" {
				outputText("     Error: %s", r.ErrorMessage)
			}
		}
	}

	// Show agent detection
	if len(agents) > 0 {
		outputText("")
		outputText("Agents detected: %s", preferredAgent)
	} else {
		outputText("")
		outputText("No AI agents detected. Install Claude Code, OpenCode, or Codex for auto-conflict resolution.")
	}
}
