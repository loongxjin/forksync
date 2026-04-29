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
	"github.com/loongxjin/forksync/engine/internal/agent/session"
	"github.com/loongxjin/forksync/engine/internal/config"
	"github.com/loongxjin/forksync/engine/internal/conflict"
	"github.com/loongxjin/forksync/engine/internal/git"
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

const (
	// defaultResolveTimeout is the fallback agent resolution timeout.
	defaultResolveTimeout = 10 * time.Minute

	// defaultDiffPreviewMaxLines is the maximum number of diff lines shown to the user.
	defaultDiffPreviewMaxLines = 100
)

// resolveContext groups parameters that always appear together during agent resolution.
type resolveContext struct {
	repo   types.Repo
	store  repo.Store
	cfg    *config.Config
	cfgMgr *config.Manager
}

// conflictResolution groups parameters for conflict resolution handlers.
type conflictResolution struct {
	repo         types.Repo
	store        repo.Store
	agentResult  *agent.AgentResult
	provider     agent.AgentProvider
	resolvedFlag *atomic.Bool
}

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
	gitOps := git.NewOperations()
	conflictPaths := gitOps.DetectConflicts(cmd.Context(), r.Path)
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
	provider, err := resolveAgentProvider(cfg)
	if err != nil {
		return err
	}

	// Create session manager
	cfgMgr := config.NewManager()
	sessionsDir := filepath.Join(cfgMgr.ConfigDir(), "sessions")
	sessionStore := session.NewSessionStore(sessionsDir)
	sessionMgr := session.NewManager(sessionStore, provider)

	// Parse timeout
	timeout := resolveTimeout(cfg)

	// Determine resolve sub-strategy for the agent prompt
	resolveStrategy := resolveStrategyOrDefault(cfg)

	// Update repo status to resolving
	r.Status = types.RepoStatusResolving
	updateRepoWithLog(r, store, "resolving")

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
			updateRepoWithLog(r, store, "defer-rollback")
		}
	}()

	// Signal guard: listen for SIGTERM / SIGINT (e.g. Electron timeout
	// killing the Go process). When received, roll back status before
	// the process exits.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, signalsToWatch...)
	defer signal.Stop(sigCh)
	go func(repo types.Repo) {
		if _, ok := <-sigCh; ok && !resolved.Load() {
			repo.Status = types.RepoStatusConflict
			repo.ErrorMessage = "agent process was terminated, conflict resolution incomplete"
			updateRepoWithLog(repo, store, "signal-rollback")
		}
	}(r)

	// Set timeout context
	ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
	defer cancel()

	// Resolve conflicts
	result, err := sessionMgr.ResolveConflicts(ctx, r.ID, r.Path, conflictPaths, resolveStrategy)
	if err != nil {
		resolved.Store(true) // agent finished (with error) — we handle the status
		r.Status = types.RepoStatusConflict
		r.ErrorMessage = fmt.Sprintf("agent resolve failed: %v", err)
		updateRepoWithLog(r, store, "agent-error")
		return fmt.Errorf("agent resolve: %w", err)
	}

	// Verify: check for remaining conflict markers
	gitOps := git.NewOperations()
	trulyUnresolved := verifyAgentResolution(ctx, r, gitOps.DetectConflicts(ctx, r.Path))
	if len(trulyUnresolved) > 0 {
		return handleUnresolvedConflicts(conflictResolution{repo: r, store: store, agentResult: result, provider: provider, resolvedFlag: &resolved}, trulyUnresolved)
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
	updateRepoWithLog(r, store, "resolved")

	// Auto-confirm or wait for user
	confirmBeforeCommit := true
	if cfg != nil {
		confirmBeforeCommit = cfg.Agent.ConfirmBeforeCommit
	}

	logger.Info("resolve: auto-confirm check",
		"repo", r.Name,
		"resolveNoConfirm", resolveNoConfirm,
		"confirmBeforeCommit", confirmBeforeCommit,
		"cfg_nil", cfg == nil,
	)

	if resolveNoConfirm || !confirmBeforeCommit {
		return completeAgentResolve(ctx, cmd, resolveContext{repo: r, store: store, cfg: cfg, cfgMgr: cfgMgr}, result)
	}

	// Show diff and wait for confirmation
	showResolutionDiff(r, diff, result, provider)
	return nil
}

// resolveAgentProvider determines the agent provider to use for conflict resolution.
func resolveAgentProvider(cfg *config.Config) (agent.AgentProvider, error) {
	if resolveAgent != "" {
		registry := agent.NewRegistry("")
		provider, err := registry.GetByName(resolveAgent)
		if err != nil {
			return nil, fmt.Errorf("agent %q not found: %w", resolveAgent, err)
		}
		return provider, nil
	}

	preferred := ""
	if cfg != nil {
		preferred = cfg.Agent.Preferred
	}
	reg := agent.NewRegistry(preferred)
	provider, err := reg.GetPreferred()
	if err != nil {
		return nil, fmt.Errorf("no agent available: %w", err)
	}
	return provider, nil
}

// resolveTimeout returns the agent resolution timeout from config or the default.
func resolveTimeout(cfg *config.Config) time.Duration {
	timeout := defaultResolveTimeout
	if cfg != nil && cfg.Agent.Timeout != "" {
		if d, err := time.ParseDuration(cfg.Agent.Timeout); err == nil {
			timeout = d
		}
	}
	return timeout
}

// verifyAgentResolution checks remaining conflict files and auto-stages those
// that have been resolved (no conflict markers). Returns the list of truly unresolved files.
func verifyAgentResolution(ctx context.Context, r types.Repo, remaining []string) []string {
	if len(remaining) == 0 {
		return nil
	}

	gitOps := git.NewOperations()
	var trulyUnresolved []string
	for _, f := range remaining {
		content, err := gitOps.GetConflictedContent(ctx, r.Path, f)
		if err != nil {
			trulyUnresolved = append(trulyUnresolved, f)
			continue
		}
		if conflict.HasConflictMarkers(content) {
			trulyUnresolved = append(trulyUnresolved, f)
			continue
		}
		// Markers removed but not staged — auto-stage to mark as resolved
		if stageErr := gitOps.StageFile(ctx, r.Path, f); stageErr != nil {
			logger.Warn("resolve: auto-stage resolved file failed",
				"repo", r.Name, "file", f, "error", stageErr)
			trulyUnresolved = append(trulyUnresolved, f)
		}
	}
	return trulyUnresolved
}

// handleUnresolvedConflicts updates repo status and outputs the result when
// the agent could not resolve all conflicts.
func handleUnresolvedConflicts(cr conflictResolution, trulyUnresolved []string) error {
	cr.resolvedFlag.Store(true)
	cr.repo.Status = types.RepoStatusConflict
	cr.repo.ErrorMessage = fmt.Sprintf("agent left %d unresolved conflicts: %s", len(trulyUnresolved), strings.Join(trulyUnresolved, ", "))
	updateRepoWithLog(cr.repo, cr.store, "unresolved-conflicts")

	logger.Warn("resolve: agent left unresolved conflicts",
		"repo", cr.repo.Name,
		"remaining", trulyUnresolved,
		"agent", cr.provider.Name(),
		"summary", cr.agentResult.Summary,
		"resolved_files", cr.agentResult.ResolvedFiles,
	)

	if isJSON() {
		outputJSON(types.ResolveData{
			RepoID:      cr.repo.ID,
			Conflicts:   toConflictFiles(trulyUnresolved),
			AgentResult: agentResultToTypes(cr.agentResult),
		}, fmt.Errorf("agent did not resolve all conflicts"))
	} else {
		outputText("⚠️  Agent could not resolve all conflicts (%d remaining)", len(trulyUnresolved))
		outputText("   Unresolved: %s", strings.Join(trulyUnresolved, ", "))
		if len(cr.agentResult.ResolvedFiles) > 0 {
			outputText("   Resolved: %s", strings.Join(cr.agentResult.ResolvedFiles, ", "))
		}
		if cr.agentResult.Summary != "" {
			outputText("   Agent summary: %s", cr.agentResult.Summary)
		}
	}
	return nil
}

// showResolutionDiff displays the diff and summary for user confirmation.
func showResolutionDiff(r types.Repo, diff string, result *agent.AgentResult, provider agent.AgentProvider) {
	if isJSON() {
		outputJSON(types.ResolveData{
			RepoID:      r.ID,
			Conflicts:   toConflictFiles(nil),
			AgentResult: agentResultToTypes(result),
		}, nil)
		return
	}

	outputText("Agent: %s (session: %s)", provider.Name(), result.SessionID)
	outputText("Summary: %s", result.Summary)
	outputText("")
	if diff != "" {
		outputText("Diff:")
		lines := strings.Split(diff, "\n")
		maxLines := defaultDiffPreviewMaxLines
		if len(lines) < maxLines {
			maxLines = len(lines)
		}
		for i := 0; i < maxLines; i++ {
			outputText("  %s", lines[i])
		}
		if len(lines) > defaultDiffPreviewMaxLines {
			outputText("  ... (%d more lines)", len(lines)-defaultDiffPreviewMaxLines)
		}
	}
	outputText("")
	outputText("Run 'forksync resolve %s --accept' to accept, or '--reject' to rollback.", r.Name)
}

// completeAgentResolve stages files and completes the merge.
func completeAgentResolve(ctx context.Context, cmd *cobra.Command, rc resolveContext, result *agent.AgentResult) error {
	// Stage all resolved files
	gitOps := git.NewOperations()
	for _, f := range result.ResolvedFiles {
		if err := gitOps.StageFile(ctx, rc.repo.Path, f); err != nil {
			return fmt.Errorf("git add %s: %w", f, err)
		}
	}

	// Commit — skip pre-commit hooks since this is an automated merge commit
	commitMsg := "Merge upstream (agent-resolved conflicts)"
	if err := gitOps.Commit(ctx, rc.repo.Path, commitMsg); err != nil {
		return fmt.Errorf("git commit: %w", err)
	}

	// Update status
	rc.repo.Status = types.RepoStatusUpToDate
	rc.repo.ErrorMessage = ""
	updateRepoWithLog(rc.repo, rc.store, "complete")

	// Update existing conflict history record to "up_to_date"
	updateResolveHistoryStatus(rc.repo, rc.cfg, rc.cfgMgr)

	outputResult(types.AcceptData{RepoID: rc.repo.ID, Resolved: true}, "✅ Merge completed for %s (agent-resolved)", rc.repo.Name)
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
	updateRepoWithLog(r, store, "reject")

	outputResult(types.RejectData{RepoID: r.ID, RolledBack: true}, "🔄 Rolled back merge for %s", r.Name)
	return nil
}

// runResolveAccept checks for remaining conflicts and completes the merge.
func runResolveAccept(cmd *cobra.Command, r types.Repo, store repo.Store, cfg *config.Config, cfgMgr *config.Manager) error {
	remaining := git.NewOperations().DetectConflicts(cmd.Context(), r.Path)

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
		updateRepoWithLog(r, store, "accept-no-merge")

		outputResult(types.AcceptData{RepoID: r.ID, Resolved: true}, "✅ No merge in progress. Status updated.")
		return nil
	}

	gitOps := git.NewOperations()
	// Stage all resolved files before committing.
	if err := gitOps.StageAll(cmd.Context(), r.Path); err != nil {
		return fmt.Errorf("git add: %w", err)
	}

	// Complete the merge.
	if err := gitOps.CommitNoEdit(cmd.Context(), r.Path); err != nil {
		if err := gitOps.Commit(cmd.Context(), r.Path, "Merge upstream changes (agent-resolved conflicts)"); err != nil {
			return fmt.Errorf("git commit: %w", err)
		}
	}

	r.Status = types.RepoStatusUpToDate
	r.ErrorMessage = ""
	updateRepoWithLog(r, store, "accept")

	// Update existing conflict history record to "synced"
	updateResolveHistoryStatus(r, cfg, cfgMgr)

	outputResult(types.AcceptData{RepoID: r.ID, Resolved: true}, "✅ Merge completed for %s", r.Name)
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

// updateResolveHistoryStatus updates the existing conflict history record to "up_to_date".
// If auto_summary is enabled and the record has no summary status yet, it also pre-sets
// summary_status to "pending" so the frontend polling can show the generating indicator.
func updateResolveHistoryStatus(r types.Repo, cfg *config.Config, cfgMgr *config.Manager) {
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

	if updateErr := histStore.UpdateStatus(record.ID, string(types.RepoStatusUpToDate)); updateErr != nil {
		logger.Error("[resolve-accept] update history status", "error", updateErr)
	}

	if cfg != nil && cfg.Sync.AutoSummary && record.SummaryStatus == "" {
		if updateErr := histStore.UpdateSummary(record.ID, "", string(types.SummaryStatusPending)); updateErr != nil {
			logger.Error("[resolve-accept] update summary status", "error", updateErr)
		}
	}
}

// resolveStrategyOrDefault returns the resolve strategy from config, or the default.
func resolveStrategyOrDefault(cfg *config.Config) string {
	return config.ResolveStrategyOrDefault(cfg)
}

// updateRepoWithLog updates the repo in the store and logs any error.
func updateRepoWithLog(r types.Repo, store repo.Store, action string) {
	if updateErr := store.Update(r); updateErr != nil {
		logger.Error("resolve: failed to update repo", "repo", r.Name, "action", action, "error", updateErr)
	}
}

// outputResult outputs data either as JSON or text depending on the output mode.
func outputResult(data any, textFormat string, textArgs ...any) {
	if isJSON() {
		outputJSON(data, nil)
	} else {
		outputText(textFormat, textArgs...)
	}
}
