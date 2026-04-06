package cmd

import (
	"context"
	"fmt"
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

var resolveAI bool

var resolveCmd = &cobra.Command{
	Use:   "resolve <repo-name>",
	Short: "Resolve conflicts in a repository",
	Args:  cobra.ExactArgs(1),
	RunE:  runResolve,
}

func init() {
	resolveCmd.Flags().BoolVar(&resolveAI, "ai", false, "use AI to resolve conflicts")
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
		outputText("Use --ai flag to resolve with AI, or resolve manually and run 'forksync sync %s'", r.Name)
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
	for _, cf := range conflictFiles {
		req := ai.ConflictRequest{
			FilePath:        cf.Path,
			ConflictContent: cf.OursContent,
			Language:        detectLanguage(cf.Path),
		}

		resolution, err := provider.ResolveConflicts(cmd.Context(), req)
		if err != nil {
			cf.AIExplanation = fmt.Sprintf("AI resolution failed: %v", err)
		} else {
			cf.MergedContent = resolution.MergedContent
			cf.AIExplanation = resolution.Explanation
		}
		resolvedConflicts = append(resolvedConflicts, cf)
	}

	if isJSON() {
		outputJSON(types.ResolveData{RepoID: r.ID, Conflicts: resolvedConflicts}, nil)
	} else {
		outputText("AI resolved %d conflicts:", len(resolvedConflicts))
		for _, cf := range resolvedConflicts {
			status := "✅ resolved"
			if cf.MergedContent == "" {
				status = "❌ failed"
			}
			outputText("  %s %s", status, cf.Path)
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
