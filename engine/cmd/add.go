package cmd

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/loongxjin/forksync/engine/internal/config"
	"github.com/loongxjin/forksync/engine/internal/git"
	"github.com/loongxjin/forksync/engine/internal/github"
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
	cfgMgr := config.NewManager()
	cfg, _ := cfgMgr.Load()

	gitOps := git.NewOperations()

	// Verify it's a git repo
	if !gitOps.IsGitRepo(cmd.Context(), repoPath) {
		return fmt.Errorf("%s is not a git repository", repoPath)
	}

	// Get origin URL
	remotes, _ := gitOps.GetRemotes(cmd.Context(), repoPath)
	originURL := ""
	for _, r := range remotes {
		if r.Name == "origin" {
			originURL = r.URL
			break
		}
	}

	// Auto-detect upstream if not provided
	resolvedUpstream := upstreamURL
	if resolvedUpstream == "" && originURL != "" && github.IsGitHubURL(originURL) {
		ghToken := ""
		if cfg != nil {
			ghToken = cfg.GitHub.Token
		}
		ghClient := github.NewClient(ghToken)
		owner, repoName, parseErr := github.ParseRepoURL(originURL)
		if parseErr == nil {
			result, detectErr := ghClient.DetectFork(cmd.Context(), owner, repoName)
			if detectErr == nil && result.IsFork && result.UpstreamURL != "" {
				resolvedUpstream = result.UpstreamURL
			}
		}
	}

	// Get current branch
	statusResult, _ := gitOps.Status(cmd.Context(), types.Repo{Path: repoPath, Upstream: resolvedUpstream})
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
	saved, _ := store.GetByName(repoName)

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
