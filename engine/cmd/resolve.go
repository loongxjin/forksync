package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/loongxjin/forksync/engine/internal/ai"
	"github.com/loongxjin/forksync/engine/internal/config"
	"github.com/loongxjin/forksync/engine/internal/conflict"
	"github.com/loongxjin/forksync/engine/internal/git"
	"github.com/loongxjin/forksync/engine/internal/repo"
	"github.com/loongxjin/forksync/engine/pkg/types"
	"github.com/spf13/cobra"
)

var (
	resolveAI      bool
	resolveAccept  string // filepath to accept
	resolveContent string // merged content for --accept
	resolveDone    bool   // mark conflicts as resolved
)

var resolveCmd = &cobra.Command{
	Use:   "resolve <repo-name>",
	Short: "Resolve conflicts in a repository",
	Long: `Resolve merge conflicts in a repository.

Examples:
  forksync resolve my-repo                    # Show conflicts
  forksync resolve my-repo --ai               # AI-powered resolution
  forksync resolve my-repo --accept file.txt --content "resolved content"
  forksync resolve my-repo --done             # Mark conflicts as resolved`,
	Args: cobra.ExactArgs(1),
	RunE: runResolve,
}

func init() {
	resolveCmd.Flags().BoolVar(&resolveAI, "ai", false, "use AI to resolve conflicts")
	resolveCmd.Flags().StringVar(&resolveAccept, "accept", "", "accept resolution for a specific file (requires --content)")
	resolveCmd.Flags().StringVar(&resolveContent, "content", "", "merged content to write (used with --accept)")
	resolveCmd.Flags().BoolVar(&resolveDone, "done", false, "mark all conflicts as resolved and complete merge")
	rootCmd.AddCommand(resolveCmd)
}

func runResolve(cmd *cobra.Command, args []string) error {
	cfgMgr := config.NewManager()
	cfg, _ := cfgMgr.Load()

	store := repo.NewJSONStore(cfgMgr.ConfigDir())
	if err := store.Load(); err != nil {
		return fmt.Errorf("load repo store: %w", err)
	}

	r, ok := store.GetByName(args[0])
	if !ok {
		return fmt.Errorf("repository %q not found", args[0])
	}

	// Handle --done: check remaining conflicts, complete merge
	if resolveDone {
		return runResolveDone(cmd, r, store)
	}

	// Handle --accept: write resolved content to file, git add
	if resolveAccept != "" {
		return runResolveAccept(cmd, r, store)
	}

	// Not in conflict state
	if r.Status != types.RepoStatusConflict {
		if isJSON() {
			outputJSON(types.DoneData{RepoID: r.ID, AllResolved: true}, nil)
		} else {
			outputText("No conflicts to resolve for %s", r.Name)
		}
		return nil
	}

	// Use the git module to find conflicted files
	gitOps := git.NewOperations()
	_, _ = gitOps.Status(cmd.Context(), r)

	conflictPaths := detectConflicts(cmd.Context(), r.Path)
	if len(conflictPaths) == 0 {
		if isJSON() {
			outputJSON(types.DoneData{RepoID: r.ID, AllResolved: true}, nil)
		} else {
			outputText("No conflict files found")
		}
		return nil
	}

	detector := conflict.NewDetector()
	conflictFiles, err := detector.GetConflictFiles(cmd.Context(), r.Path, conflictPaths)
	if err != nil {
		return fmt.Errorf("get conflict files: %w", err)
	}

	if resolveAI {
		return resolveWithAI(cmd, cfg, r, store, conflictFiles)
	}

	// Manual resolve: just show conflicts
	if isJSON() {
		outputJSON(types.ResolveData{RepoID: r.ID, Conflicts: conflictFiles}, nil)
	} else {
		outputText("Conflicts in %d files:", len(conflictFiles))
		for _, cf := range conflictFiles {
			outputText("  %s", cf.Path)
		}
		outputText("")
		outputText("Use --ai flag to resolve with AI, or resolve manually and run:")
		outputText("  forksync resolve %s --accept <filepath> --content <content>", r.Name)
		outputText("  forksync resolve %s --done", r.Name)
	}

	return nil
}

// runResolveAccept writes resolved content to a file and stages it with git add.
func runResolveAccept(cmd *cobra.Command, r types.Repo, store repo.Store) error {
	if resolveContent == "" {
		// Try reading from stdin if --content is not provided
		stat, _ := os.Stdin.Stat()
		if (stat.Mode() & os.ModeCharDevice) != 0 {
			return fmt.Errorf("--accept requires --content or piped stdin content")
		}
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return fmt.Errorf("read stdin: %w", err)
		}
		resolveContent = string(data)
	}

	// Write the resolved content to the file
	fullPath := filepath.Join(r.Path, resolveAccept)
	if err := os.WriteFile(fullPath, []byte(resolveContent), 0644); err != nil {
		return fmt.Errorf("write resolved file: %w", err)
	}

	// Stage the file with git add
	gitAddCmd := exec.CommandContext(cmd.Context(), "git", "add", resolveAccept)
	gitAddCmd.Dir = r.Path
	if output, err := gitAddCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git add %s: %s: %w", resolveAccept, string(output), err)
	}

	if isJSON() {
		outputJSON(types.AcceptData{
			RepoID:   r.ID,
			File:     resolveAccept,
			Resolved: true,
		}, nil)
	} else {
		outputText("✅ Accepted resolution for %s", resolveAccept)
	}

	// Check if there are remaining conflicts
	remaining := detectConflicts(cmd.Context(), r.Path)
	if len(remaining) == 0 {
		outputText("All conflicts resolved. Run 'forksync resolve %s --done' to complete.", r.Name)
	}

	_ = store
	return nil
}

// runResolveDone checks for remaining conflicts and completes the merge.
func runResolveDone(cmd *cobra.Command, r types.Repo, store repo.Store) error {
	remaining := detectConflicts(cmd.Context(), r.Path)

	if len(remaining) > 0 {
		if isJSON() {
			outputJSON(types.DoneData{
				RepoID:             r.ID,
				AllResolved:        false,
				RemainingConflicts: remaining,
			}, nil)
		} else {
			outputText("⚠️  %d conflicts still unresolved:", len(remaining))
			for _, f := range remaining {
				outputText("  - %s", f)
			}
		}
		return nil
	}

	// No remaining conflicts — complete the merge
	// Check if we're in a merge state
	mergeHead := filepath.Join(r.Path, ".git", "MERGE_HEAD")
	if _, err := os.Stat(mergeHead); err != nil {
		// Not in a merge state, just update status
		r.Status = types.RepoStatusSynced
		r.ErrorMessage = ""
		_ = store.Update(r)

		if isJSON() {
			outputJSON(types.DoneData{RepoID: r.ID, AllResolved: true}, nil)
		} else {
			outputText("✅ No merge in progress. Status updated.")
		}
		return nil
	}

	// Complete the merge with git commit
	commitCmd := exec.CommandContext(cmd.Context(), "git", "commit", "--no-edit")
	commitCmd.Dir = r.Path
	output, err := commitCmd.CombinedOutput()
	if err != nil {
		// If --no-edit fails (e.g., no editor configured), try with a message
		commitCmd = exec.CommandContext(cmd.Context(), "git", "commit", "-m", "Merge upstream changes (conflicts resolved)")
		commitCmd.Dir = r.Path
		output, err = commitCmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("git commit: %s: %w", string(output), err)
		}
	}

	// Update repo status
	r.Status = types.RepoStatusSynced
	r.ErrorMessage = ""
	_ = store.Update(r)

	if isJSON() {
		outputJSON(types.DoneData{RepoID: r.ID, AllResolved: true}, nil)
	} else {
		outputText("✅ Merge completed for %s", r.Name)
	}

	return nil
}

func resolveWithAI(cmd *cobra.Command, cfg *config.Config, r types.Repo, store repo.Store, conflictFiles []types.ConflictFile) error {
	// Get AI provider config
	providerName := "openai"
	if cfg != nil && cfg.AI.DefaultProvider != "" {
		providerName = cfg.AI.DefaultProvider
	}

	var apiKey, model, baseURL string
	if cfg != nil {
		if p, ok := cfg.AI.Providers[providerName]; ok {
			apiKey = p.APIKey
			model = p.Model
			baseURL = p.BaseURL
		}
	}

	if apiKey == "" {
		return fmt.Errorf("AI provider %s not configured (missing API key)", providerName)
	}

	provider := ai.NewOpenAIAdapter(apiKey, model, baseURL)

	var resolvedConflicts []types.ConflictFile
	allSucceeded := true

	for _, cf := range conflictFiles {
		req := ai.ConflictRequest{
			FilePath:        cf.Path,
			ConflictContent: cf.OursContent,
			Language:        detectLanguage(cf.Path),
		}

		resolution, err := provider.ResolveConflicts(cmd.Context(), req)
		if err != nil {
			cf.AIExplanation = fmt.Sprintf("AI resolution failed: %v", err)
			allSucceeded = false
			resolvedConflicts = append(resolvedConflicts, cf)
			continue
		}

		cf.MergedContent = resolution.MergedContent
		cf.AIExplanation = resolution.Explanation

		// Validate: reject if conflict markers remain
		if conflict.HasConflictMarkers(resolution.MergedContent) {
			cf.AIExplanation = "AI output still contains conflict markers, manual review required"
			cf.MergedContent = "" // clear so it's not written
			allSucceeded = false
			resolvedConflicts = append(resolvedConflicts, cf)
			continue
		}

		// Write resolved content to file
		fullPath := filepath.Join(r.Path, cf.Path)
		if writeErr := os.WriteFile(fullPath, []byte(resolution.MergedContent), 0644); writeErr != nil {
			cf.AIExplanation = fmt.Sprintf("failed to write file: %v", writeErr)
			allSucceeded = false
			resolvedConflicts = append(resolvedConflicts, cf)
			continue
		}

		// Stage the file with git add
		gitAddCmd := exec.CommandContext(cmd.Context(), "git", "add", cf.Path)
		gitAddCmd.Dir = r.Path
		if addOutput, addErr := gitAddCmd.CombinedOutput(); addErr != nil {
			cf.AIExplanation = fmt.Sprintf("git add failed: %s: %v", string(addOutput), addErr)
			allSucceeded = false
			resolvedConflicts = append(resolvedConflicts, cf)
			continue
		}

		resolvedConflicts = append(resolvedConflicts, cf)
	}

	// If all conflicts resolved, complete the merge
	if allSucceeded && len(resolvedConflicts) > 0 {
		commitCmd := exec.CommandContext(cmd.Context(), "git", "commit", "-m", "Merge upstream (AI-resolved conflicts)")
		commitCmd.Dir = r.Path
		if commitOutput, commitErr := commitCmd.CombinedOutput(); commitErr != nil {
			outputText("⚠️  Files staged but commit failed: %s", string(commitOutput))
			outputText("Run 'forksync resolve %s --done' to complete manually.", r.Name)
		} else {
			// Update repo status
			r.Status = types.RepoStatusSynced
			r.ErrorMessage = ""
			_ = store.Update(r)
		}
	}

	if isJSON() {
		outputJSON(types.ResolveData{RepoID: r.ID, Conflicts: resolvedConflicts}, nil)
	} else {
		outputText("AI resolved %d conflicts:", len(resolvedConflicts))
		for _, cf := range resolvedConflicts {
			status := "✅ resolved & staged"
			if cf.MergedContent == "" {
				status = "❌ failed"
			}
			outputText("  %s %s", status, cf.Path)
			if cf.AIExplanation != "" {
				outputText("     %s", cf.AIExplanation)
			}
		}
		if allSucceeded {
			outputText("")
			outputText("✅ Merge completed for %s", r.Name)
		} else {
			outputText("")
			outputText("⚠️  Some conflicts could not be auto-resolved.")
			outputText("Resolve manually and run:")
			outputText("  forksync resolve %s --accept <filepath> --content <content>", r.Name)
			outputText("  forksync resolve %s --done", r.Name)
		}
	}

	return nil
}

func detectConflicts(ctx context.Context, repoPath string) []string {
	cmd := exec.CommandContext(ctx, "git", "diff", "--name-only", "--diff-filter=U")
	cmd.Dir = repoPath
	output, err := cmd.Output()
	if err != nil {
		return nil
	}
	var files []string
	for _, f := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if f != "" {
			files = append(files, f)
		}
	}
	return files
}

func detectLanguage(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".go":
		return "go"
	case ".ts", ".tsx":
		return "typescript"
	case ".js", ".jsx":
		return "javascript"
	case ".py":
		return "python"
	case ".rs":
		return "rust"
	case ".java":
		return "java"
	case ".rb":
		return "ruby"
	case ".c", ".h":
		return "c"
	case ".cpp", ".cc", ".cxx":
		return "cpp"
	default:
		return ""
	}
}
