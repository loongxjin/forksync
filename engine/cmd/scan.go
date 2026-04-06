package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/loongxjin/forksync/engine/internal/config"
	"github.com/loongxjin/forksync/engine/internal/git"
	"github.com/loongxjin/forksync/engine/internal/github"
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
	cfgMgr := config.NewManager()
	cfg, _ := cfgMgr.Load()
	ghToken := ""
	if cfg != nil {
		ghToken = cfg.GitHub.Token
	}

	gitOps := git.NewOperations()
	ghClient := github.NewClient(ghToken)

	var scannedRepos []types.ScannedRepo

	err = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip errors
		}

		// Skip hidden directories and common non-project dirs
		if d.IsDir() && (d.Name()[0] == '.' || d.Name() == "node_modules" || d.Name() == "vendor") {
			return filepath.SkipDir
		}

		// Check if this is a git repo
		if !d.IsDir() {
			return nil
		}

		if !gitOps.IsGitRepo(cmd.Context(), path) {
			return nil
		}

		// Get remotes to find origin URL
		remotes, _ := gitOps.GetRemotes(cmd.Context(), path)
		originURL := ""
		for _, r := range remotes {
			if r.Name == "origin" {
				originURL = r.URL
				break
			}
		}

		repoName := filepath.Base(path)
		scanned := types.ScannedRepo{
			Path:   path,
			Name:   repoName,
			Origin: originURL,
		}

		// Try to detect if it's a fork via GitHub API
		if originURL != "" && github.IsGitHubURL(originURL) {
			owner, repo, parseErr := github.ParseRepoURL(originURL)
			if parseErr == nil {
				result, detectErr := ghClient.DetectFork(cmd.Context(), owner, repo)
				if detectErr == nil {
					scanned.IsFork = result.IsFork
					if result.UpstreamURL != "" {
						scanned.SuggestedUpstream = result.UpstreamURL
					}
				}
			}
		}

		scannedRepos = append(scannedRepos, scanned)

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
