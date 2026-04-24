package cmd

import (
	"context"
	"fmt"

	"github.com/loongxjin/forksync/engine/internal/agent"
	"github.com/loongxjin/forksync/engine/internal/config"
	"github.com/loongxjin/forksync/engine/internal/git"
	"github.com/loongxjin/forksync/engine/internal/history"
	"github.com/loongxjin/forksync/engine/internal/logger"
	"github.com/loongxjin/forksync/engine/internal/summarizer"
	"github.com/loongxjin/forksync/engine/pkg/types"
)

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
		return "", fmt.Errorf("no agent available. Install Claude Code, OpenCode, Droid, or Codex, or configure sync.summary_agent")
	}

	if !summarizer.IsAgentAvailable(agentName) {
		return "", fmt.Errorf("agent %q is not installed", agentName)
	}

	// Update status to generating
	if updateErr := histStore.UpdateSummary(record.ID, "", string(types.SummaryStatusGenerating)); updateErr != nil {
		logger.Error("summarize: failed to set generating status", "error", updateErr)
	}

	// Get commits (oldHEAD..upstreamRef)
	upstreamRef := resolveUpstreamRef(ctx, r)
	if record.OldHEAD == "" {
		if updateErr := histStore.UpdateSummary(record.ID, "", string(types.SummaryStatusFailed)); updateErr != nil {
			logger.Error("summarize: failed to set failed status (no old HEAD)", "error", updateErr)
		}
		return "", fmt.Errorf("no old HEAD recorded for %q, cannot determine pulled commits", r.Name)
	}

	gitOps := git.NewOperations()
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
