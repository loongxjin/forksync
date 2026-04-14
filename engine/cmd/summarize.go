package cmd

import (
	"context"
	"fmt"

	"github.com/loongxjin/forksync/engine/internal/agent"
	"github.com/loongxjin/forksync/engine/internal/config"
	"github.com/loongxjin/forksync/engine/internal/git"
	"github.com/loongxjin/forksync/engine/internal/history"
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
	cfgMgr := config.NewManager()
	cfg, _ := cfgMgr.Load()

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

	// Get the latest history record for this repo
	record, err := histStore.LatestByRepo(r.ID)
	if err != nil {
		return fmt.Errorf("no sync history found for %q", args[0])
	}

	// If retry mode, only allow retrying failed records
	if summarizeRetry && record.SummaryStatus != "failed" {
		return fmt.Errorf("latest sync for %q is not in failed state (current: %s)", args[0], record.SummaryStatus)
	}

	// If not retry and summary already done, skip
	if !summarizeRetry && record.SummaryStatus == "done" {
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
		return fmt.Errorf("no agent available. Install Claude Code or OpenCode, or configure sync.summary_agent")
	}

	if !summarizer.IsAgentAvailable(agentName) {
		return fmt.Errorf("agent %q is not installed", agentName)
	}

	// Update status to generating
	_ = histStore.UpdateSummary(record.ID, "", "generating")

	// Get commits (oldHEAD..upstreamRef)
	upstreamRef := resolveUpstreamRef(r)
	if record.OldHEAD == "" {
		_ = histStore.UpdateSummary(record.ID, "", "failed")
		return fmt.Errorf("no old HEAD recorded for %q, cannot determine pulled commits", args[0])
	}

	gitOps := git.NewOperations()
	gitCommits, err := gitOps.GetCommitLog(cmd.Context(), r.Path, record.OldHEAD, upstreamRef)
	if err != nil || len(gitCommits) == 0 {
		_ = histStore.UpdateSummary(record.ID, "", "failed")
		return fmt.Errorf("no commits found for summarization")
	}

	var commits []summarizer.CommitInfo
	for _, c := range gitCommits {
		commits = append(commits, summarizer.CommitInfo{
			Hash:    c.Hash,
			Message: c.Message,
		})
	}

	// Determine language from config (default zh)
	lang := "zh"
	if cfg != nil && cfg.Sync.SummaryLanguage != "" {
		lang = cfg.Sync.SummaryLanguage
	}

	// Execute summarization
	executor := summarizer.NewExecutor()
	summary, err := executor.Summarize(cmd.Context(), commits, lang, agentName)
	if err != nil {
		_ = histStore.UpdateSummary(record.ID, "", "failed")
		return fmt.Errorf("summarization failed: %w", err)
	}

	// Save result
	_ = histStore.UpdateSummary(record.ID, summary, "done")

	if isJSON() {
		outputJSON(SummarizeData{
			HistoryID:     record.ID,
			RepoName:      record.RepoName,
			Summary:       summary,
			SummaryStatus: "done",
		}, nil)
	} else {
		outputText("📝 %s — summarized by %s", record.RepoName, agentName)
		outputText("   %s", summary)
	}

	return nil
}

// resolveUpstreamRef computes the upstream remote/branch ref for a repo.
func resolveUpstreamRef(r types.Repo) string {
	remoteName := "upstream"
	if r.Upstream == "" {
		remoteName = "origin"
	}
	branch := r.Branch
	if branch == "" {
		gitOps := git.NewOperations()
		if b, err := gitOps.GetCurrentBranch(context.Background(), r.Path); err == nil {
			branch = b
		}
	}
	if branch == "" {
		branch = "main"
	}
	return fmt.Sprintf("%s/%s", remoteName, r.GetRemoteBranchForLocal(branch))
}
