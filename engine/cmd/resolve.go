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
	"github.com/loongxjin/forksync/engine/internal/logger"
	"github.com/loongxjin/forksync/engine/internal/repo"
	syncpkg "github.com/loongxjin/forksync/engine/internal/sync"
	"github.com/loongxjin/forksync/engine/pkg/types"
	"github.com/spf13/cobra"
)

var (
	resolveAgent     string // --agent <name>
	resolveNoConfirm bool   // --no-confirm
	resolveReject    bool   // --reject
	resolveAccept    bool   // --accept
	resolveStream    bool   // --stream

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
	resolveCmd.Flags().BoolVar(&resolveStream, "stream", false, "stream agent output as NDJSON")
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

	// Handle --accept
	if resolveAccept {
		return runResolveAccept(cmd, r, store, cfg, cfgMgr)
	}

	// Handle --reject: rollback to pre-resolution state
	if resolveReject {
		return runResolveReject(cmd, r, store, cfg)
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
func resolveWithAgent(cmd *cobra.Command, cfg *config.Config, r types.Repo, store repo.Store, conflictPaths []string) error {
	// Determine which agent to use
	provider, err := resolveAgentProvider(cfg)
	if err != nil {
		return err
	}

	// Create session manager
	cfgMgr := config.NewManager()
	sessionsDir := sessionsDir(cfgMgr)
	sessionStore := session.NewSessionStore(sessionsDir)
	sessionMgr := session.NewManager(sessionStore, provider)

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
			r.Status = types.RepoStatusConflict
			r.ErrorMessage = "agent process exited unexpectedly, conflict resolution incomplete"
			if r.Workflow != nil {
				syncpkg.AdvanceStep(r.Workflow, types.StepAgentResolve, types.StepStatusFailed, "agent process exited unexpectedly")
				r.Workflow.Status = types.WorkflowFailed
			}
			updateRepoWithLog(r, store, "defer-rollback")
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

	logger.Info("[TRACE] resolve: calling sessionMgr.ResolveConflicts", "repo", r.Name, "hasStreamWriter", streamWriter != nil, "isJSON", isJSON())
	result, err := sessionMgr.ResolveConflicts(ctx, r.ID, r.Path, conflictPaths, resolveStrategy, streamWriter)
	logger.Info("[TRACE] resolve: sessionMgr.ResolveConflicts returned", "repo", r.Name, "err", err, "resultNil", result == nil)
	if err != nil {
		logger.Warn("resolve: agent resolve failed", "repo", r.Name, "error", err)
		if streamWriter != nil {
			_ = streamWriter.WriteEvent(agent.StreamEvent{
				Type:      agent.StreamEventError,
				Data:      fmt.Sprintf("agent resolve failed: %v", err),
				Timestamp: time.Now().UTC(),
				Success:   false,
			})
		}
		resolved.Store(true) // agent finished (with error) — we handle the status
		r.Status = types.RepoStatusConflict
		r.ErrorMessage = fmt.Sprintf("agent resolve failed: %v", err)
		if r.Workflow != nil {
			syncpkg.AdvanceStep(r.Workflow, types.StepAgentResolve, types.StepStatusFailed, err.Error())
			r.Workflow.Status = types.WorkflowFailed
		}
		updateRepoWithLog(r, store, "agent-error")
		return fmt.Errorf("agent resolve: %w", err)
	}
	logger.Info("resolve: agent resolve completed", "repo", r.Name, "success", result != nil && result.Success)

	// Verify: check for remaining conflict markers
	gitOps := newGitOps(cfg)
	trulyUnresolved := verifyAgentResolution(ctx, r, gitOps.DetectConflicts(ctx, r.Path), cfg)
	if len(trulyUnresolved) > 0 {
		return handleUnresolvedConflicts(conflictResolution{repo: r, store: store, agentResult: result, provider: provider, resolvedFlag: &resolved}, trulyUnresolved)
	}

	// Get diff for user confirmation
	diffBytes, _ := newGitOps(cfg).Diff(ctx, r.Path)
	diff := string(diffBytes)

	result.Diff = diff
	result.ResolvedFiles = conflictPaths
	result.AgentName = provider.Name()

	// Update status — agent resolved successfully
	resolved.Store(true)
	r.Status = types.RepoStatusResolved
	r.ErrorMessage = ""
	updateWorkflowAgentResolve(&r, provider.Name(), "")
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
	// In --stream mode, skip outputJSON() since the NDJSON stream on stdout is
	// being consumed by the Electron process. The final result is already
	// delivered via the done stream event.
	if !resolveStream {
		logger.Info("[TRACE] resolve: calling showResolutionDiff (non-stream)", "repo", r.Name)
		showResolutionDiff(r, diff, result, provider)
	} else {
		logger.Info("[TRACE] resolve: skipping showResolutionDiff (stream mode — result already sent via done event)", "repo", r.Name)
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
				syncpkg.AdvanceStep(r.Workflow, types.StepAgentResolve, types.StepStatusFailed, "agent process was terminated")
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

// verifyAgentResolution checks remaining conflict files and auto-stages those
// that have been resolved (no conflict markers). Returns the list of truly unresolved files.
func verifyAgentResolution(ctx context.Context, r types.Repo, remaining []string, cfg *config.Config) []string {
	if len(remaining) == 0 {
		return nil
	}

	gitOps := newGitOps(cfg)
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

// completeAgentResolve stages files and completes the merge.
func completeAgentResolve(ctx context.Context, cmd *cobra.Command, rc resolveContext, result *agent.AgentResult) error {
	// Agent logs are meaningless once the workflow is complete — clean them up.
	agent.DeleteAllLogs(rc.cfgMgr.ConfigDir(), rc.repo.Name)

	// Execute post-sync commands now that the merge is committed.
	results := syncpkg.RunPostSyncCommands(ctx, rc.repo)
	if err := syncpkg.PostSyncError(results); err != "" {
		logger.Error("resolve: post-sync command failed", "repo", rc.repo.Name, "error", err)
	}

	// Stage all resolved files
	gitOps := newGitOps(rc.cfg)
	for _, f := range result.ResolvedFiles {
		if err := gitOps.StageFile(ctx, rc.repo.Path, f); err != nil {
			return fmt.Errorf("git add %s: %w", f, err)
		}
	}

	// Commit — skip pre-commit hooks since this is an automated merge commit
	commitMsg := types.CommitMsgAgentResolved
	if err := gitOps.Commit(ctx, rc.repo.Path, commitMsg); err != nil {
		logger.Warn("resolve: commit failed after agent resolution, keeping resolved state for manual confirmation",
			"repo", rc.repo.Name, "error", err)
		if isJSON() && !resolveStream {
			outputJSON(types.ResolveData{
				RepoID:      rc.repo.ID,
				Conflicts:   []types.ConflictFile{},
				AgentResult: agentResultToTypes(result),
				CommitError: fmt.Sprintf("auto-commit failed: %v", err),
			}, nil)
		} else if !isJSON() {
			outputText("Agent resolved conflicts but commit failed: %v", err)
			outputText("Please fix the issue and run 'forksync resolve %s --accept' to complete the merge.", rc.repo.Name)
		}
		return nil
	}

	// Update status
	rc.repo.Status = types.RepoStatusUpToDate
	rc.repo.ErrorMessage = ""
	updateWorkflowCommit(&rc.repo)
	updateRepoWithLog(rc.repo, rc.store, "complete")

	info := workflowCompletionInfo(rc.repo.Workflow)
	recordWorkflowComplete(rc.repo, 0, info)

	if !resolveStream {
		outputResult(types.AcceptData{RepoID: rc.repo.ID, Resolved: true}, "✅ Merge completed for %s (agent-resolved)", rc.repo.Name)
	} else {
		logger.Info("[TRACE] resolve: skipping outputResult in stream mode", "repo", rc.repo.Name)
	}
	return nil
}

// runResolveReject rolls back the merge using git merge --abort,
// restoring the repository to its pre-merge state.
func runResolveReject(cmd *cobra.Command, r types.Repo, store repo.Store, cfg *config.Config) error {
	_, cfgMgr := getSharedConfig()
	agent.DeleteAllLogs(cfgMgr.ConfigDir(), r.Name)
	ctx := cmd.Context()
	gitOps := newGitOps(cfg)

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
	// User explicitly rejected — clear the workflow entirely rather than leaving
	// a failed record behind. The git state has already been rolled back.
	r.Workflow = nil
	updateRepoWithLog(r, store, "reject")

	outputResult(types.RejectData{RepoID: r.ID, RolledBack: true}, "🔄 Rolled back merge for %s", r.Name)
	return nil
}

// runResolveAccept checks for remaining conflicts and completes the merge.
func runResolveAccept(cmd *cobra.Command, r types.Repo, store repo.Store, cfg *config.Config, cfgMgr *config.Manager) error {
	agent.DeleteAllLogs(cfgMgr.ConfigDir(), r.Name)

	remaining := newGitOps(cfg).DetectConflicts(cmd.Context(), r.Path)

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

	gitOps := newGitOps(cfg)
	// Stage all resolved files before committing.
	if err := gitOps.StageAll(cmd.Context(), r.Path); err != nil {
		return fmt.Errorf("git add: %w", err)
	}

	// Complete the merge.
	if err := gitOps.CommitNoEdit(cmd.Context(), r.Path); err != nil {
		if err := gitOps.Commit(cmd.Context(), r.Path, types.CommitMsgAgentResolved); err != nil {
			return fmt.Errorf("git commit: %w", err)
		}
	}

	r.Status = types.RepoStatusUpToDate
	r.ErrorMessage = ""
	updateWorkflowCommit(&r)
	updateRepoWithLog(r, store, "accept")

	// Execute post-sync commands now that the merge is committed.
	results := syncpkg.RunPostSyncCommands(cmd.Context(), r)
	if err := syncpkg.PostSyncError(results); err != "" {
		logger.Error("resolve: post-sync command failed", "repo", r.Name, "error", err)
	}

	info := workflowCompletionInfo(r.Workflow)
	recordWorkflowComplete(r, 0, info)

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

// setupResolveStreamWriter creates a stream writer for agent resolution output.
// In stream mode, writes to both stdout (real-time Electron consumption) and disk
// (later replay). Returns nil and a no-op cleanup when streaming is disabled.
func setupResolveStreamWriter(configDir, repoName string) (*agent.StreamWriter, func()) {
	if !resolveStream {
		return nil, func() {}
	}
	logger.Info("[TRACE] resolve: streaming mode enabled", "repo", repoName)
	stdoutSW := agent.NewStreamWriter(os.Stdout)
	lw, lwErr := agent.NewLogWriter(configDir, repoName)
	if lwErr != nil {
		logger.Warn("resolve: failed to create log writer, using stdout only", "repo", repoName, "error", lwErr)
		return stdoutSW, func() {}
	}
	msw := agent.NewMultiStreamWriter(stdoutSW, lw.StreamWriter())
	logger.Info("resolve: streaming to stdout + disk log", "repo", repoName)
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

// updateWorkflowAgentResolve updates the workflow when agent resolve succeeds.
func updateWorkflowAgentResolve(r *types.Repo, agentName string, commitErr string) {
	if r.Workflow == nil {
		return
	}
	now := types.Time{Time: time.Now()}
	for i := range r.Workflow.Steps {
		if r.Workflow.Steps[i].Step == types.StepAgentResolve {
			r.Workflow.Steps[i].Status = types.StepStatusSuccess
			r.Workflow.Steps[i].Message = fmt.Sprintf("resolved by %s", agentName)
			r.Workflow.Steps[i].EndedAt = &now
		}
		if r.Workflow.Steps[i].Step == types.StepAcceptChanges {
			if commitErr != "" {
				r.Workflow.Steps[i].Status = types.StepStatusWaiting
				r.Workflow.Steps[i].Message = commitErr
			} else {
				r.Workflow.Steps[i].Status = types.StepStatusWaiting
			}
		}
	}
	r.Workflow.Status = types.WorkflowWaiting
}

// updateWorkflowCommit updates the workflow when commit succeeds.
func updateWorkflowCommit(r *types.Repo) {
	if r.Workflow == nil {
		return
	}
	now := types.Time{Time: time.Now()}
	for i := range r.Workflow.Steps {
		if r.Workflow.Steps[i].Step == types.StepCommit {
			r.Workflow.Steps[i].Status = types.StepStatusSuccess
			r.Workflow.Steps[i].EndedAt = &now
		}
		if r.Workflow.Steps[i].Step == types.StepAcceptChanges && r.Workflow.Steps[i].Status == types.StepStatusWaiting {
			r.Workflow.Steps[i].Status = types.StepStatusSuccess
			r.Workflow.Steps[i].EndedAt = &now
		}
	}
	r.Workflow.Status = types.WorkflowSuccess
	r.Workflow.FinishedAt = &now
}

// updateWorkflowAbort updates the workflow when the merge is aborted.
func updateWorkflowAbort(r *types.Repo) {
	if r.Workflow == nil {
		return
	}
	now := types.Time{Time: time.Now()}
	for i := range r.Workflow.Steps {
		if r.Workflow.Steps[i].Status == types.StepStatusPending {
			r.Workflow.Steps[i].Status = types.StepStatusSkipped
			r.Workflow.Steps[i].EndedAt = &now
		}
	}
	r.Workflow.Status = types.WorkflowFailed
	r.Workflow.FinishedAt = &now
}
