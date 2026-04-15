package cmd

import (
	"context"
	"fmt"

	"github.com/loongxjin/forksync/engine/internal/git"
	"github.com/loongxjin/forksync/engine/internal/history"
	"github.com/loongxjin/forksync/engine/internal/logger"
	"github.com/loongxjin/forksync/engine/internal/repo"
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

	if summarizeRetry && record.SummaryStatus != "failed" {
		return fmt.Errorf("latest sync for %q is not in failed state (current: %s)", args[0], record.SummaryStatus)
	}

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

	summary, err := generateSummary(cmd.Context(), cfg, histStore, record, r)
	if err != nil {
		return err
	}

	if isJSON() {
		outputJSON(SummarizeData{
			HistoryID:     record.ID,
			RepoName:      record.RepoName,
			Summary:       summary,
			SummaryStatus: "done",
		}, nil)
	} else {
		outputText("📝 %s — summarized", record.RepoName)
		outputText("   %s", summary)
	}

	return nil
}

// resolveUpstreamRef computes the upstream remote/branch ref for a repo.
func resolveUpstreamRef(ctx context.Context, r types.Repo) string {
	remoteName := r.RemoteName()
	branch := r.Branch
	if branch == "" {
		gitOps := git.NewOperations()
		if b, err := gitOps.GetCurrentBranch(ctx, r.Path); err == nil {
			branch = b
		}
	}
	if branch == "" {
		branch = "main"
	}
	return fmt.Sprintf("%s/%s", remoteName, r.GetRemoteBranchForLocal(branch))
}
