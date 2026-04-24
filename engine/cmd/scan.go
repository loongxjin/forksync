package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/loongxjin/forksync/engine/internal/git"
	"github.com/loongxjin/forksync/engine/internal/github"
	"github.com/loongxjin/forksync/engine/internal/logger"
	"github.com/loongxjin/forksync/engine/pkg/types"
	"github.com/spf13/cobra"
)

var scanCmd = &cobra.Command{
	Use:   "scan <directory>",
	Short: "Scan a directory for git repositories and detect forks",
	Args:  cobra.ExactArgs(1),
	RunE:  runScan,
}

func init() {
	rootCmd.AddCommand(scanCmd)
}

func runScan(cmd *cobra.Command, args []string) error {
	dir := args[0]

	// Verify directory exists
	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("directory not found: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", dir)
	}

	// Load config for GitHub token
	cfg, _ := getSharedConfig()
	ghToken := ""
	if cfg != nil {
		ghToken = cfg.GitHub.Token
	}

	gitOps := git.NewOperations()
	ghClient := github.NewClient(ghToken)

	scannedRepos := make([]types.ScannedRepo, 0)

	err = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			logger.Warn("scan: skip path", "path", path, "error", err)
			return nil // skip but log
		}

		// Skip hidden directories and common non-project dirs
		name := d.Name()
		if d.IsDir() && (len(name) == 0 || name[0] == '.' || name == "node_modules" || name == "vendor") {
			return filepath.SkipDir
		}
		// Check if this is a git repo
		if !d.IsDir() {
			return nil
		}
		if !gitOps.IsGitRepo(cmd.Context(), path) {
			return nil
		}

		scanned := processScannedRepo(cmd.Context(), path, gitOps, ghClient)
		if scanned != nil {
			scannedRepos = append(scannedRepos, *scanned)
		}

		// Don't recurse into .git subdirectories
		return filepath.SkipDir
	})

	if err != nil {
		return fmt.Errorf("scan error: %w", err)
	}

	if isJSON() {
		outputJSON(types.ScanData{Repos: scannedRepos}, nil)
	} else {
		if len(scannedRepos) == 0 {
			outputText("No git repositories found in %s", dir)
		} else {
			outputText("Found %d git repositories:", len(scannedRepos))
			for _, r := range scannedRepos {
				forkLabel := ""
				if r.IsFork {
					forkLabel = " (fork)"
					if r.SuggestedUpstream != "" {
						forkLabel = fmt.Sprintf(" (fork → %s)", r.SuggestedUpstream)
					}
				}
				outputText("  %s%s", r.Name, forkLabel)
			}
		}
	}

	return nil
}

// processScannedRepo handles the per-directory logic for scanning a git repo.
// It returns nil if the directory should be skipped.
func processScannedRepo(ctx context.Context, dir string, gitOps *git.Operations, ghClient *github.Client) *types.ScannedRepo {
	// Get remotes to find origin URL
	remotes, _ := gitOps.GetRemotes(ctx, dir)
	originURL := ""
	for _, r := range remotes {
		if r.Name == "origin" {
			originURL = r.URL
			break
		}
	}

	repoName := filepath.Base(dir)
	scanned := types.ScannedRepo{
		Path:   dir,
		Name:   repoName,
		Origin: originURL,
	}

	// Try to detect if it's a fork via GitHub API
	if originURL != "" && github.IsGitHubURL(originURL) {
		owner, repo, parseErr := github.ParseRepoURL(originURL)
		if parseErr == nil {
			result, detectErr := ghClient.DetectFork(ctx, owner, repo)
			if detectErr == nil {
				scanned.IsFork = result.IsFork
				if result.UpstreamURL != "" {
					scanned.SuggestedUpstream = result.UpstreamURL
				}
			}
		}
	}

	localBranches, _ := gitOps.GetLocalBranches(ctx, dir)
	scanned.LocalBranches = localBranches

	originBranches, _ := gitOps.GetRemoteBranches(ctx, dir, "origin")
	scanned.RemoteBranches = originBranches

	for _, r := range remotes {
		if r.Name == "upstream" {
			upstreamBranches, _ := gitOps.GetRemoteBranches(ctx, dir, "upstream")
			branchMap := make(map[string]struct{})
			for _, b := range scanned.RemoteBranches {
				branchMap[b] = struct{}{}
			}
			for _, b := range upstreamBranches {
				if _, ok := branchMap[b]; !ok {
					scanned.RemoteBranches = append(scanned.RemoteBranches, b)
					branchMap[b] = struct{}{}
				}
			}
			break
		}
	}

	return &scanned
}
