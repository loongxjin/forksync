package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/loongxjin/forksync/engine/internal/agent"
	"github.com/loongxjin/forksync/engine/internal/conflict"
	"github.com/loongxjin/forksync/engine/internal/git"
	"github.com/loongxjin/forksync/engine/internal/agent/session"
	"github.com/loongxjin/forksync/engine/internal/config"
	"github.com/loongxjin/forksync/engine/internal/history"
	"github.com/loongxjin/forksync/engine/internal/logger"
	"github.com/loongxjin/forksync/engine/internal/repo"
	"github.com/loongxjin/forksync/engine/pkg/types"
	"github.com/spf13/cobra"
)

var (
	resolveAgent     string // --agent <name>
	resolveNoConfirm bool   // --no-confirm
	resolveReject    bool   // --reject
	resolveAccept    bool   // --accept

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
  forksync resolve my-repo --accept               # Accept conflicts as resolved`,
	Args: cobra.ExactArgs(1),
	RunE: runResolve,
}

func init() {
	resolveCmd.Flags().StringVar(&resolveAgent, "agent", "", "specify agent to use (claude, opencode, droid, codex)")
	resolveCmd.Flags().BoolVar(&resolveNoConfirm, "no-confirm", false, "auto-commit without user confirmation")
	resolveCmd.Flags().BoolVar(&resolveReject, "reject", false, "reject last resolution and rollback")
	resolveCmd.Flags().BoolVar(&resolveAccept, "accept", false, "accept all conflicts as resolved")
	rootCmd.AddCommand(resolveCmd)
}

func runResolve(cmd *cobra.Command, args []string) error {
	cfg, cfgMgr := getSharedConfig()

	store := repo.NewJSONStore(cfgMgr.ConfigDir())
	if err := store.Load(); err != nil {
		return fmt.Errorf("load repo store: %w", err)
	}

	r, ok := store.GetByName(args[0])
	if !ok {
		return fmt.Errorf("repository %q not found", args[0])
	}

	// Handle --accept
	if resolveAccept {
		return runResolveAccept(cmd, r, store, cfg, cfgMgr)
	}

	// Handle --reject: rollback to pre-resolution state
	if resolveReject {
		return runResolveReject(cmd, r, store)
	}

	// Not in conflict state
	if r.Status != types.RepoStatusConflict && r.Status != types.RepoStatusResolved {
		if isJSON() {
			outputJSON(types.AcceptData{RepoID: r.ID, Resolved: true}, nil)
		} else {
			outputText("No conflicts to resolve for %s", r.Name)
		}
		return nil
	}

	// Detect conflict files
	conflictPaths := conflict.DetectConflicts(cmd.Context(), r.Path)
	if len(conflictPaths) == 0 {
		if isJSON() {
			outputJSON(types.AcceptData{RepoID: r.ID, Resolved: true}, nil)
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

	// Determine resolve sub-strategy for the agent prompt
	resolveStrategy := "preserve_ours"
	if cfg != nil && cfg.Agent.ResolveStrategy != "" {
		resolveStrategy = cfg.Agent.ResolveStrategy
	}

	// Update repo status to resolving
	r.Status = types.RepoStatusResolving
	if updateErr := store.Update(r); updateErr != nil {
		logger.Error("resolve: failed to update repo to resolving", "repo", r.Name, "error", updateErr)
	}

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
			if updateErr := store.Update(r); updateErr != nil {
				logger.Error("resolve: failed to roll back repo", "repo", r.Name, "error", updateErr)
			}
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
			if updateErr := store.Update(r); updateErr != nil {
				logger.Error("resolve: failed to roll back repo on signal", "repo", r.Name, "error", updateErr)
			}
		}
	}()

	// Set timeout context
	ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
	defer cancel()

	// Resolve conflicts
	result, err := sessionMgr.ResolveConflicts(ctx, r.ID, r.Path, conflictPaths, resolveStrategy)
	if err != nil {
		resolved.Store(true) // agent finished (with error) — we handle the status
		r.Status = types.RepoStatusConflict
		r.ErrorMessage = fmt.Sprintf("agent resolve failed: %v", err)
		if updateErr := store.Update(r); updateErr != nil {
			logger.Error("resolve: failed to update repo after agent error", "repo", r.Name, "error", updateErr)
		}
		return fmt.Errorf("agent resolve: %w", err)
	}

		// Verify: check for remaining conflict markers
		remaining := conflict.DetectConflicts(ctx, r.Path)
		if len(remaining) > 0 {
			// Agent may have removed conflict markers but not staged the files,
			// leaving them in unmerged state. Check and auto-stage those files.
			var trulyUnresolved []string
			for _, f := range remaining {
				content, err := git.NewOperations().GetConflictedContent(ctx, r.Path, f)
				if err != nil {
					trulyUnresolved = append(trulyUnresolved, f)
					continue
				}
				if conflict.HasConflictMarkers(content) {
					// Still has markers — agent didn't resolve this one
					trulyUnresolved = append(trulyUnresolved, f)
				} else {
					// Markers removed but not staged — auto-stage to mark as resolved
					if stageErr := git.NewOperations().StageFile(ctx, r.Path, f); stageErr != nil {
						logger.Warn("resolve: auto-stage resolved file failed",
							"repo", r.Name, "file", f, "error", stageErr)
						trulyUnresolved = append(trulyUnresolved, f)
					}
				}
			}

			if len(trulyUnresolved) > 0 {
				resolved.Store(true) // agent finished but left conflicts — we handle the status
				r.Status = types.RepoStatusConflict
				r.ErrorMessage = fmt.Sprintf("agent left %d unresolved conflicts: %s", len(trulyUnresolved), strings.Join(trulyUnresolved, ", "))
				if updateErr := store.Update(r); updateErr != nil {
					logger.Error("resolve: failed to update repo after unresolved conflicts", "repo", r.Name, "error", updateErr)
				}

				logger.Warn("resolve: agent left unresolved conflicts",
					"repo", r.Name,
					"remaining", trulyUnresolved,
					"agent", provider.Name(),
					"summary", result.Summary,
					"resolved_files", result.ResolvedFiles,
				)

				if isJSON() {
					outputJSON(types.ResolveData{
						RepoID:      r.ID,
						Conflicts:   toConflictFiles(trulyUnresolved),
						AgentResult: agentResultToTypes(result),
					}, fmt.Errorf("agent did not resolve all conflicts"))
				} else {
					outputText("⚠️  Agent could not resolve all conflicts (%d remaining)", len(trulyUnresolved))
					outputText("   Unresolved: %s", strings.Join(trulyUnresolved, ", "))
					if len(result.ResolvedFiles) > 0 {
						outputText("   Resolved: %s", strings.Join(result.ResolvedFiles, ", "))
					}
					if result.Summary != "" {
						outputText("   Agent summary: %s", result.Summary)
					}
				}
				return nil
			}
		}

	// Get diff for user confirmation
	diffBytes, _ := git.NewOperations().Diff(ctx, r.Path)
	diff := string(diffBytes)

	result.Diff = diff
	result.ResolvedFiles = conflictPaths
	result.AgentName = provider.Name()

	// Update status — agent resolved successfully
	resolved.Store(true)
	r.Status = types.RepoStatusResolved
	r.ErrorMessage = ""
	if updateErr := store.Update(r); updateErr != nil {
		logger.Error("resolve: failed to update repo to resolved", "repo", r.Name, "error", updateErr)
	}

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
		outputText("Run 'forksync resolve %s --accept' to accept, or '--reject' to rollback.", r.Name)
	}

	return nil
}

// completeAgentResolve stages files and completes the merge.
func completeAgentResolve(ctx context.Context, r types.Repo, store repo.Store, result *agent.AgentResult) error {
	// Stage all resolved files
	gitOps := git.NewOperations()
	for _, f := range result.ResolvedFiles {
		if err := gitOps.StageFile(ctx, r.Path, f); err != nil {
			return fmt.Errorf("git add %s: %w", f, err)
		}
	}

	// Commit — skip pre-commit hooks since this is an automated merge commit
	commitMsg := "Merge upstream (agent-resolved conflicts)"
	if err := gitOps.Commit(ctx, r.Path, commitMsg, true); err != nil {
		return fmt.Errorf("git commit: %w", err)
	}

	// Update status
	r.Status = types.RepoStatusUpToDate
	r.ErrorMessage = ""
	if updateErr := store.Update(r); updateErr != nil {
		logger.Error("resolve: failed to update repo after complete", "repo", r.Name, "error", updateErr)
	}

	if isJSON() {
		outputJSON(types.AcceptData{RepoID: r.ID, Resolved: true}, nil)
	} else {
		outputText("✅ Merge completed for %s (agent-resolved)", r.Name)
	}
	return nil
}

// runResolveReject rolls back the merge using git merge --abort,
// restoring the repository to its pre-merge state.
func runResolveReject(cmd *cobra.Command, r types.Repo, store repo.Store) error {
	ctx := cmd.Context()
	gitOps := git.NewOperations()

	err := gitOps.AbortMerge(ctx, r.Path)
	if err != nil {
		logger.Error("resolve: merge --abort failed", "repo", r.Name, "error", err)
		r.Status = types.RepoStatusConflict
		r.ErrorMessage = fmt.Sprintf("reject failed: merge --abort error: %v", err)
		_ = store.Update(r)

		if isJSON() {
			outputJSON(types.RejectData{RepoID: r.ID, RolledBack: false}, fmt.Errorf("merge --abort failed: %w", err))
		} else {
			outputText("⚠️  Failed to rollback: %v", err)
		}
		return fmt.Errorf("merge --abort: %w", err)
	}

	r.Status = types.RepoStatusConflict
	r.ErrorMessage = ""
	if updateErr := store.Update(r); updateErr != nil {
		logger.Error("resolve: failed to update repo after reject", "repo", r.Name, "error", updateErr)
	}

	if isJSON() {
		outputJSON(types.RejectData{RepoID: r.ID, RolledBack: true}, nil)
	} else {
		outputText("🔄 Rolled back merge for %s", r.Name)
	}
	return nil
}

// runResolveAccept checks for remaining conflicts and completes the merge.
func runResolveAccept(cmd *cobra.Command, r types.Repo, store repo.Store, cfg *config.Config, cfgMgr *config.Manager) error {
	remaining := conflict.DetectConflicts(cmd.Context(), r.Path)

	if len(remaining) > 0 {
		if isJSON() {
			outputJSON(types.AcceptData{
				RepoID:   r.ID,
				Resolved: false,
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
		r.Status = types.RepoStatusUpToDate
		r.ErrorMessage = ""
		if updateErr := store.Update(r); updateErr != nil {
			logger.Error("resolve: failed to update repo after accept (no merge)", "repo", r.Name, "error", updateErr)
		}

		if isJSON() {
			outputJSON(types.AcceptData{RepoID: r.ID, Resolved: true}, nil)
		} else {
			outputText("✅ No merge in progress. Status updated.")
		}
		return nil
	}

	gitOps := git.NewOperations()
	// Stage all resolved files before committing.
	if err := gitOps.StageAll(cmd.Context(), r.Path); err != nil {
		return fmt.Errorf("git add: %w", err)
	}

	// Complete the merge.
	if err := gitOps.CommitNoEdit(cmd.Context(), r.Path, true); err != nil {
		if err := gitOps.Commit(cmd.Context(), r.Path, "Merge upstream changes (agent-resolved conflicts)", true); err != nil {
			return fmt.Errorf("git commit: %w", err)
		}
	}

	r.Status = types.RepoStatusUpToDate
	r.ErrorMessage = ""
	if updateErr := store.Update(r); updateErr != nil {
		logger.Error("resolve: failed to update repo after accept", "repo", r.Name, "error", updateErr)
	}

	// Update existing conflict history record to "synced" and trigger AI summary if enabled.
	triggerResolveSummary(cmd.Context(), r, cfg, cfgMgr)

	if isJSON() {
		outputJSON(types.AcceptData{RepoID: r.ID, Resolved: true}, nil)
	} else {
		outputText("✅ Merge completed for %s", r.Name)
	}
	return nil
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
		AgentName:     r.AgentName,
	}
}

// triggerResolveSummary updates the conflict history record to "synced" and triggers
// AI summary generation if auto_summary is enabled.
func triggerResolveSummary(ctx context.Context, r types.Repo, cfg *config.Config, cfgMgr *config.Manager) {
	histStore, err := history.NewStore(cfgMgr.ConfigDir())
	if err != nil {
		logger.Error("[resolve-accept] open history store", "error", err)
		return
	}
	defer histStore.Close()

	record, err := histStore.LatestByRepo(r.ID)
	if err != nil {
		logger.Error("[resolve-accept] find history record", "error", err)
		return
	}

	if updateErr := histStore.UpdateStatus(record.ID, "up_to_date"); updateErr != nil {
		logger.Error("[resolve-accept] update history status", "error", updateErr)
	}

	if cfg == nil || !cfg.Sync.AutoSummary {
		return
	}

	_, err = generateSummary(ctx, cfg, histStore, record, r)
	if err != nil {
		logger.Error("[resolve-accept] summary generation failed", "error", err)
	}
}
