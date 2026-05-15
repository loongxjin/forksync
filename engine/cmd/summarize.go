package cmd

import (
	"context"
	"fmt"

	"github.com/loongxjin/forksync/engine/internal/agent"
	"github.com/loongxjin/forksync/engine/internal/config"
	"github.com/loongxjin/forksync/engine/internal/history"
	"github.com/loongxjin/forksync/engine/internal/logger"
	"github.com/loongxjin/forksync/engine/internal/repo"
	"github.com/loongxjin/forksync/engine/internal/summarizer"
	"github.com/loongxjin/forksync/engine/pkg/types"
	"github.com/spf13/cobra"
)

var summarizeRetry bool

var summarizeCmd = &cobra.Command{
	Use:   "summarize <repo-name>",
	Short: "Generate AI summary for the latest sync of a repo",
	Long: `Generate an AI-generated summary for the most recent sync of a repository.
Uses the configured agent CLI to summarize the pulled commits.`,
	Args: cobra.ExactArgs(1),
	RunE: runSummarize,
}

func init() {
	summarizeCmd.Flags().BoolVar(&summarizeRetry, "retry", false, "retry a failed summary generation")
	rootCmd.AddCommand(summarizeCmd)
}

// SummarizeData is the response for the summarize command.
type SummarizeData struct {
	HistoryID     int64  `json:"historyId"`
	RepoName      string `json:"repoName"`
	Summary       string `json:"summary"`
	SummaryStatus string `json:"summaryStatus"`
}

func runSummarize(cmd *cobra.Command, args []string) error {
	cfg, cfgMgr := getSharedConfig()

	repoStore := repo.NewJSONStore(cfgMgr.ConfigDir())
	if err := repoStore.Load(); err != nil {
		return fmt.Errorf("load repo store: %w", err)
	}

	r, ok := repoStore.GetByName(args[0])
	if !ok {
		return fmt.Errorf("repository %q not found", args[0])
	}

	histStore, err := history.NewStore(cfgMgr.ConfigDir())
	if err != nil {
		return fmt.Errorf("open history store: %w", err)
	}
	defer histStore.Close()

	defer logger.Close()

	record, err := histStore.LatestByRepo(r.ID)
	if err != nil {
		return fmt.Errorf("no sync history found for %q", args[0])
	}

	if summarizeRetry && record.SummaryStatus != string(types.SummaryStatusFailed) {
		return fmt.Errorf("latest sync for %q is not in failed state (current: %s)", args[0], record.SummaryStatus)
	}

	if !summarizeRetry && record.SummaryStatus == string(types.SummaryStatusDone) {
		if isJSON() {
			outputJSON(SummarizeData{
				HistoryID:     record.ID,
				RepoName:      record.RepoName,
				Summary:       record.Summary,
				SummaryStatus: record.SummaryStatus,
			}, nil)
		} else {
			outputText("📝 %s — already summarized", record.RepoName)
			outputText("   %s", record.Summary)
		}
		return nil
	}

	summary, err := generateSummary(cmd.Context(), cfg, histStore, record, r)
	if err != nil {
		return err
	}

	if isJSON() {
		outputJSON(SummarizeData{
			HistoryID:     record.ID,
			RepoName:      record.RepoName,
			Summary:       summary,
			SummaryStatus: string(types.SummaryStatusDone),
		}, nil)
	} else {
		outputText("📝 %s — summarized", record.RepoName)
		outputText("   %s", summary)
	}

	return nil
}

// generateSummary performs synchronous summarization for the given history record.
// It is shared between the summarize command and resolve --accept flow.
func generateSummary(
	ctx context.Context,
	cfg *config.Config,
	histStore *history.Store,
	record *history.Record,
	r types.Repo,
) (string, error) {
	// Determine agent
	agentName := ""
	if cfg != nil {
		agentName = cfg.Sync.SummaryAgent
	}
	if agentName == "" {
		registry := agent.NewRegistry("")
		if prov, err := registry.GetPreferred(); err == nil {
			agentName = prov.Name()
		}
	}
	if agentName == "" {
		return "", fmt.Errorf("no agent available. Install Claude Code, OpenCode, or Codex, or configure sync.summary_agent")
	}

	if !summarizer.IsAgentAvailable(agentName) {
		return "", fmt.Errorf("agent %q is not installed", agentName)
	}

	// Records without old HEAD cannot be summarized — mark as failed so the
	// frontend stops polling and the user can retry manually if needed.
	if record.OldHEAD == "" {
		if updateErr := histStore.UpdateSummary(record.ID, "", string(types.SummaryStatusFailed)); updateErr != nil {
			logger.Error("summarize: failed to set failed status (no old HEAD)", "error", updateErr)
		}
		return "", fmt.Errorf("no old HEAD recorded for %q, cannot determine pulled commits", r.Name)
	}

	// Update status to generating
	if updateErr := histStore.UpdateSummary(record.ID, "", string(types.SummaryStatusGenerating)); updateErr != nil {
		logger.Error("summarize: failed to set generating status", "error", updateErr)
	}

	// Get commits (oldHEAD..upstreamRef)
	gitOps := newGitOps(cfg)
	upstreamRef := gitOps.ResolveUpstreamRef(ctx, r)
	gitCommits, err := gitOps.GetCommitLog(ctx, r.Path, record.OldHEAD, upstreamRef)
	if err != nil || len(gitCommits) == 0 {
		if updateErr := histStore.UpdateSummary(record.ID, "", string(types.SummaryStatusFailed)); updateErr != nil {
			logger.Error("summarize: failed to set failed status (no commits)", "error", updateErr)
		}
		return "", fmt.Errorf("no commits found for summarization")
	}

	var commits []summarizer.CommitInfo
	for _, c := range gitCommits {
		commits = append(commits, summarizer.CommitInfo{
			Hash:    c.Hash,
			Message: c.Message,
		})
	}

	// Determine language from config (default zh)
	lang := types.DefaultSummaryLanguage
	if cfg != nil && cfg.Sync.SummaryLanguage != "" {
		lang = cfg.Sync.SummaryLanguage
	}

	// Execute summarization
	executor := summarizer.NewExecutor()
	summary, err := executor.Summarize(ctx, commits, lang, agentName)
	if err != nil {
		if updateErr := histStore.UpdateSummary(record.ID, "", string(types.SummaryStatusFailed)); updateErr != nil {
			logger.Error("summarize: failed to set failed status after error", "error", updateErr)
		}
		return "", fmt.Errorf("summarization failed: %w", err)
	}

	// Save result
	if updateErr := histStore.UpdateSummary(record.ID, summary, string(types.SummaryStatusDone)); updateErr != nil {
		logger.Error("summarize: failed to save summary result", "error", updateErr)
	}

	return summary, nil
}
