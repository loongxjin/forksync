package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/loongxjin/forksync/engine/internal/config"
	"github.com/loongxjin/forksync/engine/internal/git"
	"github.com/loongxjin/forksync/engine/internal/github"
	"github.com/loongxjin/forksync/engine/internal/logger"
	"github.com/loongxjin/forksync/engine/internal/repo"
	"github.com/loongxjin/forksync/engine/pkg/types"
	"github.com/spf13/cobra"
)

var upstreamURL string
var branchMappingJSON string

var addCmd = &cobra.Command{
	Use:   "add <repo-path>",
	Short: "Add a repository to ForkSync management",
	Args:  cobra.ExactArgs(1),
	RunE:  runAdd,
}

func init() {
	addCmd.Flags().StringVar(&upstreamURL, "upstream", "", "upstream repository URL")
	addCmd.Flags().StringVar(&branchMappingJSON, "branch-mapping", "", "optional branch mapping as JSON object, e.g. {\"localBranch\":\"main\",\"remoteBranch\":\"master\"}")
	rootCmd.AddCommand(addCmd)
}

func runAdd(cmd *cobra.Command, args []string) error {
	repoPath, err := filepath.Abs(args[0])
	if err != nil {
		return fmt.Errorf("invalid path: %w", err)
	}

	// Load config early for GitHub token
	cfg, cfgMgr := getSharedConfig()

	gitOps := git.NewOperations()

	// Verify it's a git repo
	if !gitOps.IsGitRepo(cmd.Context(), repoPath) {
		return fmt.Errorf("%s is not a git repository", repoPath)
	}

	// Get origin URL
	remotes, err := gitOps.GetRemotes(cmd.Context(), repoPath)
	if err != nil {
		logger.Warn("add: failed to get remotes", "repo", repoPath, "error", err)
	}
	originURL := ""
	for _, r := range remotes {
		if r.Name == "origin" {
			originURL = r.URL
			break
		}
	}

	// Auto-detect upstream if not provided
	resolvedUpstream := upstreamURL
	if resolvedUpstream == "" {
		resolvedUpstream = autoDetectUpstream(cmd.Context(), cfg, originURL)
	}

	// Get current branch
	statusResult, err := gitOps.Status(cmd.Context(), types.Repo{Path: repoPath, Upstream: resolvedUpstream})
	if err != nil {
		logger.Warn("add: failed to get status", "repo", repoPath, "error", err)
	}
	branch := "main"
	if statusResult != nil && statusResult.Branch != "" {
		branch = statusResult.Branch
	}

	var branchMapping *types.BranchMapping
	if branchMappingJSON != "" {
		var mapping types.BranchMapping
		if err := json.Unmarshal([]byte(branchMappingJSON), &mapping); err != nil {
			return fmt.Errorf("invalid branch-mapping JSON: %w", err)
		}
		if mapping.LocalBranch != "" && mapping.RemoteBranch != "" {
			branchMapping = &mapping
		}
	}

	repoName := filepath.Base(repoPath)

	newRepo := types.Repo{
		Name:          repoName,
		Path:          repoPath,
		Origin:        originURL,
		Upstream:      resolvedUpstream,
		Branch:        branch,
		BranchMapping: branchMapping,
		Status:        types.RepoStatusUnconfigured,
	}

	// Load store
	store := repo.NewJSONStore(cfgMgr.ConfigDir())
	if err := store.Load(); err != nil {
		return fmt.Errorf("load repo store: %w", err)
	}

	if err := store.Add(newRepo); err != nil {
		return fmt.Errorf("add repo: %w", err)
	}

	// Get the saved repo (with auto-generated ID)
	saved, ok := store.GetByName(repoName)
	if !ok {
		return fmt.Errorf("add: failed to retrieve saved repo %q", repoName)
	}

	if isJSON() {
		outputJSON(types.AddData{Repo: saved}, nil)
	} else {
		outputText("Added repository: %s", repoName)
		if resolvedUpstream != "" {
			outputText("  Upstream: %s", resolvedUpstream)
		} else {
			outputText("  No upstream configured (use --upstream to set)")
		}
		if branchMapping != nil {
			outputText("  Branch mapping: %s -> %s", branchMapping.LocalBranch, branchMapping.RemoteBranch)
		}
	}

	return nil
}

// autoDetectUpstream attempts to detect the upstream URL for a fork via the GitHub API.
// Returns the upstream URL if detected, or an empty string if detection fails.
func autoDetectUpstream(ctx context.Context, cfg *config.Config, originURL string) string {
	if originURL == "" || !github.IsGitHubURL(originURL) {
		return ""
	}

	ghToken := ""
	if cfg != nil {
		ghToken = cfg.GitHub.Token
	}

	ghClient := github.NewClient(ghToken)
	owner, repoName, parseErr := github.ParseRepoURL(originURL)
	if parseErr != nil {
		return ""
	}

	result, detectErr := ghClient.DetectFork(ctx, owner, repoName)
	if detectErr != nil {
		return ""
	}

	if result.IsFork && result.UpstreamURL != "" {
		return result.UpstreamURL
	}

	return ""
}
