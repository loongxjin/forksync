package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/loongxjin/forksync/engine/internal/agent"
	"github.com/loongxjin/forksync/engine/internal/agent/session"
	"github.com/loongxjin/forksync/engine/internal/config"
	"github.com/loongxjin/forksync/engine/internal/repo"
	"github.com/loongxjin/forksync/engine/pkg/types"
	"github.com/spf13/cobra"
)

var (
	resolveAgent     string // --agent <name>
	resolveNoConfirm bool   // --no-confirm
	resolveReject    bool   // --reject
	resolveDone      bool   // --done

	// signalsToWatch lists OS signals that should trigger status rollback
	// when the Go process is killed during agent conflict resolution.
	signalsToWatch = []os.Signal{os.Interrupt, syscall.SIGTERM}
)

var resolveCmd = &cobra.Command{
	Use:   "resolve <repo-name>",
	Short: "Resolve conflicts using an AI agent",
	Long: `Resolve merge conflicts in a repository using an AI coding agent.

Examples:
  forksync resolve my-repo                        # Auto-resolve with agent
  forksync resolve my-repo --agent claude         # Use specific agent
  forksync resolve my-repo --no-confirm           # Auto-commit without confirmation
  forksync resolve my-repo --reject               # Reject last resolution (rollback)
  forksync resolve my-repo --done                 # Mark conflicts as resolved`,
	Args: cobra.ExactArgs(1),
	RunE: runResolve,
}

func init() {
	resolveCmd.Flags().StringVar(&resolveAgent, "agent", "", "specify agent to use (claude, opencode, droid, codex)")
	resolveCmd.Flags().BoolVar(&resolveNoConfirm, "no-confirm", false, "auto-commit without user confirmation")
	resolveCmd.Flags().BoolVar(&resolveReject, "reject", false, "reject last resolution and rollback")
	resolveCmd.Flags().BoolVar(&resolveDone, "done", false, "mark all conflicts as resolved")
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

	// Handle --done
	if resolveDone {
		return runResolveDone(cmd, r, store)
	}

	// Handle --reject: rollback to pre-resolution state
	if resolveReject {
		return runResolveReject(cmd, r, store)
	}

	// Not in conflict state
	if r.Status != types.RepoStatusConflict && r.Status != types.RepoStatusResolved {
		if isJSON() {
			outputJSON(types.DoneData{RepoID: r.ID, AllResolved: true}, nil)
		} else {
			outputText("No conflicts to resolve for %s", r.Name)
		}
		return nil
	}

	// Detect conflict files
	conflictPaths := detectConflicts(cmd.Context(), r.Path)
	if len(conflictPaths) == 0 {
		if isJSON() {
			outputJSON(types.DoneData{RepoID: r.ID, AllResolved: true}, nil)
		} else {
			outputText("No conflict files found")
		}
		return nil
	}

	// Resolve with agent
	return resolveWithAgent(cmd, cfg, r, store, conflictPaths)
}

// resolveWithAgent resolves conflicts using an agent CLI.
func resolveWithAgent(cmd *cobra.Command, cfg *config.Config, r types.Repo, store repo.Store, conflictPaths []string) error {
	// Determine which agent to use
	var provider agent.AgentProvider
	if resolveAgent != "" {
		registry := agent.NewRegistry("")
		var err error
		provider, err = registry.GetByName(resolveAgent)
		if err != nil {
			return fmt.Errorf("agent %q not found: %w", resolveAgent, err)
		}
	} else {
		// Use preferred or first available
		preferred := ""
		if cfg != nil {
			preferred = cfg.Agent.Preferred
		}
		reg := agent.NewRegistry(preferred)
		var err error
		provider, err = reg.GetPreferred()
		if err != nil {
			return fmt.Errorf("no agent available: %w", err)
		}
	}

	// Create session manager
	cfgMgr := config.NewManager()
	sessionsDir := filepath.Join(cfgMgr.ConfigDir(), "sessions")
	sessionStore := session.NewSessionStore(sessionsDir)
	sessionMgr := session.NewManager(sessionStore, provider)

	// Parse timeout
	timeout := 10 * time.Minute
	if cfg != nil && cfg.Agent.Timeout != "" {
		if d, err := time.ParseDuration(cfg.Agent.Timeout); err == nil {
			timeout = d
		}
	}

	// Determine strategy
	strategy := "preserve_ours"
	if cfg != nil && cfg.Agent.ConflictStrategy != "" {
		strategy = cfg.Agent.ConflictStrategy
	}

	// Update repo status to resolving
	r.Status = types.RepoStatusResolving
	_ = store.Update(r)

	// resolved tracks whether the agent finished successfully.
	// Used by the defer guard and signal handler to decide whether
	// to roll back the status on unexpected exit.
	var resolved atomic.Bool

	// Defer guard: if the function returns without the agent having
	// produced a final state (resolved / conflict from verify), roll
	// back to conflict so the repo doesn't get stuck in resolving.
	defer func() {
		if !resolved.Load() {
			r.Status = types.RepoStatusConflict
			r.ErrorMessage = "agent process exited unexpectedly, conflict resolution incomplete"
			_ = store.Update(r)
		}
	}()

	// Signal guard: listen for SIGTERM / SIGINT (e.g. Electron timeout
	// killing the Go process). When received, roll back status before
	// the process exits.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, signalsToWatch...)
	defer signal.Stop(sigCh)
	go func() {
		if _, ok := <-sigCh; ok && !resolved.Load() {
			r.Status = types.RepoStatusConflict
			r.ErrorMessage = "agent process was terminated, conflict resolution incomplete"
			_ = store.Update(r)
		}
	}()

	// Set timeout context
	ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
	defer cancel()

	// Resolve conflicts
	result, err := sessionMgr.ResolveConflicts(ctx, r.ID, r.Path, conflictPaths, strategy)
	if err != nil {
		resolved.Store(true) // agent finished (with error) — we handle the status
		r.Status = types.RepoStatusConflict
		r.ErrorMessage = fmt.Sprintf("agent resolve failed: %v", err)
		_ = store.Update(r)
		return fmt.Errorf("agent resolve: %w", err)
	}

	// Verify: check for remaining conflict markers
	remaining := detectConflicts(ctx, r.Path)
	if len(remaining) > 0 {
		resolved.Store(true) // agent finished but left conflicts — we handle the status
		r.Status = types.RepoStatusConflict
		r.ErrorMessage = fmt.Sprintf("agent left %d unresolved conflicts", len(remaining))
		_ = store.Update(r)

		if isJSON() {
			outputJSON(types.ResolveData{
				RepoID:    r.ID,
				Conflicts: toConflictFiles(remaining),
			}, fmt.Errorf("agent did not resolve all conflicts"))
		} else {
			outputText("⚠️  Agent could not resolve all conflicts (%d remaining)", len(remaining))
		}
		return nil
	}

	// Get diff for user confirmation
	diffCmd := exec.CommandContext(ctx, "git", "diff")
	diffCmd.Dir = r.Path
	diffBytes, _ := diffCmd.Output()
	diff := string(diffBytes)

	result.Diff = diff
	result.ResolvedFiles = conflictPaths

	// Update status — agent resolved successfully
	resolved.Store(true)
	r.Status = types.RepoStatusResolved
	r.ErrorMessage = ""
	_ = store.Update(r)

	// Auto-confirm or wait for user
	confirmBeforeCommit := true
	if cfg != nil {
		confirmBeforeCommit = cfg.Agent.ConfirmBeforeCommit
	}

	if resolveNoConfirm || !confirmBeforeCommit {
		// Auto-commit
		return completeAgentResolve(ctx, r, store, result)
	}

	// Show diff and wait for confirmation
	if isJSON() {
		outputJSON(types.ResolveData{
			RepoID:      r.ID,
			Conflicts:   toConflictFiles(conflictPaths),
			AgentResult: agentResultToTypes(result),
		}, nil)
	} else {
		outputText("Agent: %s (session: %s)", provider.Name(), result.SessionID)
		outputText("Summary: %s", result.Summary)
		outputText("")
		if diff != "" {
			outputText("Diff:")
			lines := strings.Split(diff, "\n")
			maxLines := 100
			if len(lines) < maxLines {
				maxLines = len(lines)
			}
			for i := 0; i < maxLines; i++ {
				outputText("  %s", lines[i])
			}
			if len(lines) > 100 {
				outputText("  ... (%d more lines)", len(lines)-100)
			}
		}
		outputText("")
		outputText("Run 'forksync resolve %s --done' to accept, or '--reject' to rollback.", r.Name)
	}

	return nil
}

// completeAgentResolve stages files and completes the merge.
func completeAgentResolve(ctx context.Context, r types.Repo, store repo.Store, result *agent.AgentResult) error {
	// Stage all resolved files
	for _, f := range result.ResolvedFiles {
		gitAddCmd := exec.CommandContext(ctx, "git", "add", f)
		gitAddCmd.Dir = r.Path
		if output, err := gitAddCmd.CombinedOutput(); err != nil {
			return fmt.Errorf("git add %s: %s: %w", f, string(output), err)
		}
	}

	// Commit
	commitMsg := fmt.Sprintf("Merge upstream (agent-resolved conflicts)")
	commitCmd := exec.CommandContext(ctx, "git", "commit", "-m", commitMsg)
	commitCmd.Dir = r.Path
	if output, err := commitCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git commit: %s: %w", string(output), err)
	}

	// Update status
	r.Status = types.RepoStatusSynced
	r.ErrorMessage = ""
	_ = store.Update(r)

	if isJSON() {
		outputJSON(types.AcceptData{RepoID: r.ID, Resolved: true}, nil)
	} else {
		outputText("✅ Merge completed for %s (agent-resolved)", r.Name)
	}
	return nil
}

// runResolveReject rolls back agent changes using git checkout.
func runResolveReject(cmd *cobra.Command, r types.Repo, store repo.Store) error {
	// Checkout all conflicted files to restore pre-resolution state
	conflictPaths := detectConflicts(cmd.Context(), r.Path)
	for _, f := range conflictPaths {
		checkoutCmd := exec.CommandContext(cmd.Context(), "git", "checkout", "--", f)
		checkoutCmd.Dir = r.Path
		if output, err := checkoutCmd.CombinedOutput(); err != nil {
			outputText("⚠️  checkout %s failed: %s", f, string(output))
		}
	}

	r.Status = types.RepoStatusConflict
	r.ErrorMessage = ""
	_ = store.Update(r)

	if isJSON() {
		outputJSON(types.RejectData{RepoID: r.ID, RolledBack: true}, nil)
	} else {
		outputText("🔄 Rolled back agent changes for %s", r.Name)
	}
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

	// Check if we're in a merge state
	mergeHead := filepath.Join(r.Path, ".git", "MERGE_HEAD")
	if _, err := os.Stat(mergeHead); err != nil {
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

	// Complete the merge
	commitCmd := exec.CommandContext(cmd.Context(), "git", "commit", "--no-edit")
	commitCmd.Dir = r.Path
	output, err := commitCmd.CombinedOutput()
	if err != nil {
		commitCmd = exec.CommandContext(cmd.Context(), "git", "commit", "-m", "Merge upstream changes (agent-resolved conflicts)")
		commitCmd.Dir = r.Path
		output, err = commitCmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("git commit: %s: %w", string(output), err)
		}
	}

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

// detectConflicts finds files with unresolved conflicts via git diff.
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

// toConflictFiles converts string paths to ConflictFile slices.
func toConflictFiles(paths []string) []types.ConflictFile {
	files := make([]types.ConflictFile, len(paths))
	for i, p := range paths {
		files[i] = types.ConflictFile{Path: p}
	}
	return files
}

// agentResultToTypes converts an agent.AgentResult to types.AgentResolveResult.
func agentResultToTypes(r *agent.AgentResult) *types.AgentResolveResult {
	if r == nil {
		return nil
	}
	return &types.AgentResolveResult{
		Success:       r.Success,
		ResolvedFiles: r.ResolvedFiles,
		Diff:          r.Diff,
		Summary:       r.Summary,
		SessionID:     r.SessionID,
	}
}
