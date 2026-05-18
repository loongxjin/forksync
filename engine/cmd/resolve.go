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
	"github.com/loongxjin/forksync/engine/internal/config"
	"github.com/loongxjin/forksync/engine/internal/logger"
	"github.com/loongxjin/forksync/engine/internal/repo"
	respkg "github.com/loongxjin/forksync/engine/internal/resolve"
	"github.com/loongxjin/forksync/engine/internal/workflow"
	"github.com/loongxjin/forksync/engine/pkg/types"
	"github.com/spf13/cobra"
)

var (
	resolveAgent     string // --agent <name>
	resolveNoConfirm bool   // --no-confirm
	resolveReject    bool   // --reject
	resolveAccept    bool   // --accept
	resolveStream    bool   // --stream
	resolvePrepare   bool   // --prepare (mark workflow ready for agent)
	resolveRetry     bool   // --retry (used with --accept to retry commit)
	resolveManual    bool   // --manual (used with --accept after manual resolution)

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
	resolveCmd.Flags().StringVar(&resolveAgent, "agent", "", "specify agent to use (claude, opencode, codex)")
	resolveCmd.Flags().BoolVar(&resolveNoConfirm, "no-confirm", false, "auto-commit without user confirmation")
	resolveCmd.Flags().BoolVar(&resolveReject, "reject", false, "reject last resolution and rollback")
	resolveCmd.Flags().BoolVar(&resolveAccept, "accept", false, "accept all conflicts as resolved")
	resolveCmd.Flags().BoolVar(&resolveStream, "stream", false, "stream agent output as NDJSON")
	resolveCmd.Flags().BoolVar(&resolvePrepare, "prepare", false, "mark workflow as ready for agent (no agent run)")
	resolveCmd.Flags().BoolVar(&resolveRetry, "retry", false, "retry commit (use with --accept)")
	resolveCmd.Flags().BoolVar(&resolveManual, "manual", false, "manual resolution (use with --accept)")
	rootCmd.AddCommand(resolveCmd)
}

func runResolve(cmd *cobra.Command, args []string) error {
	cfg, cfgMgr := getSharedConfig()

	store, err := loadRepoStore()
	if err != nil {
		return err
	}

	r, ok := store.GetByName(args[0])
	if !ok {
		return fmt.Errorf("repository %q not found", args[0])
	}

	// Handle --prepare: mark workflow ready for agent (lightweight, no agent run)
	if resolvePrepare {
		return runResolvePrepare(cmd, r, store)
	}

	// Handle --accept
	if resolveAccept {
		return runResolveAccept(cmd, r, store, cfg, cfgMgr)
	}

	// Handle --reject: rollback to pre-resolution state
	if resolveReject {
		return runResolveReject(cmd, r, store, cfg, cfgMgr)
	}

	// Not in a conflict-related state
	if r.Status != types.RepoStatusConflict && r.Status != types.RepoStatusResolved &&
		r.Status != types.RepoStatusResolving && r.Status != types.RepoStatusWaiting {
		if isJSON() {
			outputJSON(types.AcceptData{RepoID: r.ID, Resolved: true}, nil)
		} else {
			outputText("No conflicts to resolve for %s", r.Name)
		}
		return nil
	}

	// Detect conflict files
	gitOps := newGitOps(cfg)
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
func resolveWithAgent(cmd *cobra.Command, cfg *config.Config, r types.Repo, store repo.Store, _ []string) error {
	// Determine which agent to use
	provider, err := resolveAgentProvider(cfg)
	if err != nil {
		return err
	}

	cfgMgr := config.NewManager()
	resolver := respkg.NewResolver(newGitOps(cfg), store, cfg, cfgMgr)

	// Parse timeout
	timeout := resolveTimeout(cfg)

	// Determine resolve sub-strategy for the agent prompt
	resolveStrategy := resolveStrategyOrDefault(cfg)

	// resolved tracks whether the agent finished successfully.
	var resolved atomic.Bool

	// Install guards BEFORE setting status to resolving. Otherwise a kill
	// between the status write and guard registration leaves a permanent
	// "resolving" deadlock on disk.
	defer func() {
		if !resolved.Load() {
			_, _ = resolver.Reject(cmd.Context(), r)
		}
	}()

	stopSignalGuard := installSignalGuard(&r, store, &resolved)
	defer stopSignalGuard()

	// Update repo status to resolving
	r.Status = types.RepoStatusResolving
	updateRepoWithLog(r, store, "resolving")

	// Set timeout context
	ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
	defer cancel()

	streamWriter, closeLogWriter := setupResolveStreamWriter(cfgMgr.ConfigDir(), r.Name)
	defer closeLogWriter()

	logger.Debug("[TRACE] resolve: calling resolver.ResolveWithAgent", "repo", r.Name, "hasStreamWriter", streamWriter != nil, "isJSON", isJSON())
	res, err := resolver.ResolveWithAgent(ctx, r, provider, resolveStrategy, streamWriter)
	logger.Debug("[TRACE] resolve: resolver.ResolveWithAgent returned", "repo", r.Name, "err", err, "resultNil", res == nil)

	if err != nil {
		logger.Error("resolve: agent resolve failed", "repo", r.Name, "error", err)
		if streamWriter != nil {
			_ = streamWriter.WriteEvent(agent.StreamEvent{
				Type:      agent.StreamEventError,
				Data:      fmt.Sprintf("agent resolve failed: %v", err),
				Timestamp: time.Now().UTC(),
				Success:   false,
			})
		}
		resolved.Store(true)
		return fmt.Errorf("agent resolve: %w", err)
	}

	if len(res.Unresolved) > 0 {
		resolved.Store(true)
		return handleUnresolvedConflicts(conflictResolution{repo: res.Repo, store: store, agentResult: res.AgentResult, provider: provider, resolvedFlag: &resolved}, res.Unresolved)
	}

	r = res.Repo
	resolved.Store(true)

	// Auto-confirm or wait for user
	confirmBeforeCommit := true
	if cfg != nil {
		confirmBeforeCommit = cfg.Agent.ConfirmBeforeCommit
	}

	logger.Debug("resolve: auto-confirm check",
		"repo", r.Name,
		"resolveNoConfirm", resolveNoConfirm,
		"confirmBeforeCommit", confirmBeforeCommit,
		"cfg_nil", cfg == nil,
	)

	if resolveNoConfirm || !confirmBeforeCommit {
		return finalizeCommitWithWorkflow(ctx, r, store, newGitOps(cfg), commitWorkflowParams{
			commitMsg:     types.CommitMsgAgentResolved,
			recordHistory: true,
			silentOutput:  resolveStream,
		})
	}

	// Show diff and wait for confirmation
	if !resolveStream {
		logger.Debug("[TRACE] resolve: calling showResolutionDiff (non-stream)", "repo", r.Name)
		showResolutionDiff(r, res.Diff, res.AgentResult, provider)
	} else {
		logger.Debug("[TRACE] resolve: skipping showResolutionDiff (stream mode)", "repo", r.Name)
	}
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

// installSignalGuard listens for OS signals (SIGTERM/SIGINT) and rolls back
// the repo status when the process is killed during agent resolution.
// Returns a stop function that must be deferred by the caller.
func installSignalGuard(r *types.Repo, store repo.Store, resolved *atomic.Bool) func() {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, signalsToWatch...)
	go func() {
		if _, ok := <-sigCh; ok && !resolved.Load() {
			r.Status = types.RepoStatusConflict
			r.ErrorMessage = "agent process was terminated, conflict resolution incomplete"
			if r.Workflow != nil {
				workflow.AdvanceStep(r.Workflow, types.StepAgentResolve, types.StepStatusFailed, "agent process was terminated")
				r.Workflow.Status = types.WorkflowFailed
			}
			updateRepoWithLog(*r, store, "signal-rollback")
		}
	}()
	return func() { signal.Stop(sigCh) }
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

// handleUnresolvedConflicts updates repo status and outputs the result when
// the agent could not resolve all conflicts.
func handleUnresolvedConflicts(cr conflictResolution, trulyUnresolved []string) error {
	cr.resolvedFlag.Store(true)
	cr.repo.Status = types.RepoStatusConflict
	cr.repo.ErrorMessage = fmt.Sprintf("agent left %d unresolved conflicts: %s", len(trulyUnresolved), strings.Join(trulyUnresolved, ", "))
	updateRepoWithLog(cr.repo, cr.store, "unresolved-conflicts")

	logger.Error("resolve: agent left unresolved conflicts",
		"repo", cr.repo.Name,
		"remaining", trulyUnresolved,
		"agent", cr.provider.Name(),
		"summary", cr.agentResult.Summary,
		"resolved_files", cr.agentResult.ResolvedFiles,
	)

	if isJSON() && !resolveStream {
		outputJSON(types.ResolveData{
			RepoID:      cr.repo.ID,
			Conflicts:   toConflictFiles(trulyUnresolved),
			AgentResult: agentResultToTypes(cr.agentResult),
		}, fmt.Errorf("agent did not resolve all conflicts"))
	} else if !isJSON() {
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

// runResolveReject rolls back the merge using git merge --abort,
// restoring the repository to its pre-merge state.
func runResolveReject(cmd *cobra.Command, r types.Repo, store repo.Store, cfg *config.Config, cfgMgr *config.Manager) error {
	resolver := respkg.NewResolver(newGitOps(cfg), store, cfg, cfgMgr)
	repo, err := resolver.Reject(cmd.Context(), r)
	if err != nil {
		return err
	}
	outputResult(types.RejectData{RepoID: repo.ID, RolledBack: true}, "🔄 Rolled back merge for %s", repo.Name)
	return nil
}

// runResolveAccept checks for remaining conflicts and completes the merge.
func runResolveAccept(cmd *cobra.Command, r types.Repo, store repo.Store, cfg *config.Config, cfgMgr *config.Manager) error {
	resolver := respkg.NewResolver(newGitOps(cfg), store, cfg, cfgMgr)
	repo, result, err := resolver.Accept(cmd.Context(), r, resolveManual, resolveRetry)

	if err != nil && !result.Success {
		// Conflicts still unresolved
		if isJSON() {
			outputJSON(types.AcceptData{
				RepoID:   repo.ID,
				Resolved: false,
			}, nil)
		} else {
			outputText("⚠️  %s", err.Error())
		}
		return nil
	}

	if result.Success && err == nil {
		// Check if this was accept-no-merge (no MERGE_HEAD)
		mergeHead := filepath.Join(repo.Path, ".git", "MERGE_HEAD")
		if _, serr := os.Stat(mergeHead); serr != nil && repo.Status == types.RepoStatusUpToDate {
			outputResult(types.AcceptData{RepoID: repo.ID, Resolved: true}, "✅ No merge in progress. Status updated.")
			return nil
		}
	}

	outputResult(types.AcceptData{RepoID: repo.ID, Resolved: true}, "✅ Accepted changes for %s", repo.Name)
	return nil
}

// runResolvePrepare marks the workflow as ready for agent resolution without actually
// running the agent. Used by the Electron frontend as a lightweight pre-step before
// spawning resolve --stream.
func runResolvePrepare(cmd *cobra.Command, r types.Repo, store repo.Store) error {
	cfg, cfgMgr := getSharedConfig()
	resolver := respkg.NewResolver(newGitOps(cfg), store, cfg, cfgMgr)
	repo, err := resolver.Prepare(r)
	if err != nil {
		return err
	}
	outputWorkflowResult(repo)
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

// setupResolveStreamWriter creates a stream writer for agent resolution output.
// In stream mode, writes to both stdout (real-time Electron consumption) and disk
// (later replay). Returns nil and a no-op cleanup when streaming is disabled.
func setupResolveStreamWriter(configDir, repoName string) (*agent.StreamWriter, func()) {
	if !resolveStream {
		return nil, func() {}
	}
	logger.Debug("[TRACE] resolve: streaming mode enabled", "repo", repoName)
	stdoutSW := agent.NewStreamWriter(os.Stdout)
	lw, lwErr := agent.NewLogWriter(configDir, repoName)
	if lwErr != nil {
		logger.Warn("resolve: failed to create log writer, using stdout only", "repo", repoName, "error", lwErr)
		return stdoutSW, func() {}
	}
	msw := agent.NewMultiStreamWriter(stdoutSW, lw.StreamWriter())
	logger.Debug("resolve: streaming to stdout + disk log", "repo", repoName)
	return msw.StreamWriter(), func() { lw.Close() }
}

// resolveStrategyOrDefault returns the resolve strategy from config, or the default.
func resolveStrategyOrDefault(cfg *config.Config) string {
	return config.ResolveStrategyOrDefault(cfg)
}

// outputResult outputs data either as JSON or text depending on the output mode.
func outputResult(data any, textFormat string, textArgs ...any) {
	if isJSON() {
		outputJSON(data, nil)
	} else {
		outputText(textFormat, textArgs...)
	}
}
