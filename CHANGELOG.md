# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- **Engine crash supervision**: the embedded Go engine is respawned with exponential backoff (500ms→5s, up to 5 attempts) if it crashes after a healthy start. Lifecycle status (starting/ready/reconnecting/down) is broadcast to the renderer via the `engine:status` IPC channel for a reconnect banner.
- **Local auth token**: the engine generates a random 32-byte (256-bit) bearer token at startup and announces it via `FORKSYNC_TOKEN=` on stdout. The Electron parent injects `Authorization: Bearer <token>` on every request and `?token=` on the WebSocket routes. `/healthz` and `/version` stay unauthenticated for the startup readiness probe.
- **Push-based state updates**: a new in-process event bus + `/stream/events` WebSocket replaces the renderer's fixed-interval polling. The engine publishes `repos_changed` on any repo mutation and `history_changed` on summary/cleanup; the renderer opens one socket on launch and refreshes on push instead of polling `/status` (3s) and history (5s). Idle traffic drops to near zero.

### Changed
- **CSP hardened**: production `connect-src` restricted to `'self'` (the renderer never talks to the engine directly — all traffic is via main IPC), closing a localhost exfiltration channel. `default-src` drops `'unsafe-inline'`. Dev keeps localhost wildcards for Vite HMR.
- **HomePage decomposed**: the 700-line god-component is now ~415 lines. Seven concerns extracted into dedicated custom hooks (`useStartupSync`, `useHistorySync`, `useAgentLogAutoload`, `useSummaryPolling`, `useResolveActions`, `useDragDropAdd`, `useAutoExpandWorkflow`), each matching the existing named-object-return + useLogger + ref-held-timer pattern.
- **Polling removed**: the 3s status poll and 5s conflict poll are gone, superseded by `/stream/events`. The 5s summary-generation poll is retained as a guard.

### Fixed
- **List re-render storm**: `RepoRow` and `HistoryRow` are now `React.memo`-wrapped (HistoryRow with a custom comparator), so unchanged rows no longer re-render on every 3s/5s status poll. `RepoRow.onToggle` now takes `repoId` so the stable `toggleExpand` is passed instead of a per-render inline arrow.
- **Hardcoded English**: six resolve-action error messages moved to i18next keys (toast namespace) with interpolation.

### Accessibility
- **Modal**: now has `role="dialog"`, `aria-modal`, Escape-to-close, body scroll lock, and a focus trap that cycles Tab inside the modal and returns focus to the trigger on close.
- **Sheet/Drawer**: added `role="dialog"`, `aria-modal`, and focus trap (mirrors Modal; Escape and scroll lock already existed).
- **Toast**: added `role="alert" aria-live="assertive"` so screen readers announce errors immediately.
- **RepoRow**: expansion click-target changed from `<div>` to `<button>` with `aria-expanded` and `focus-visible` ring, making the accordion keyboard-operable.
- **ErrorBanner**: shared component (`role="alert"`, optional retry/dismiss) replaces hand-rolled inline error divs.
- **ConfirmDialog**: styled confirmation dialog replacing native `confirm()`/`alert()` — removes unstyled, non-localizable, main-thread-blocking browser dialogs.

## [v0.4.0]

### Added
- Multi-language support for agent conflict resolution prompts
- Conflict resolved notification with i18n
- Vitest test infrastructure for frontend unit testing
- Unit tests for Context reducers, hooks (useLogger, useDebouncedConfig, useAutoSummarize), EngineClient, and IPC input validation

### Changed
- **Frontend architecture**: extracted StepContent, HistoryRow, BranchMappingInput as reusable components; extracted useResolveStream, useAutoSummarize, useLogger hooks; extracted ToastContext from RepoContext
- **Shared types**: moved to `app/src/shared/types` for proper cross-process imports
- **Engine architecture**: extracted workflow state machine into `internal/workflow` package; replaced Set* methods with functional Option pattern; extracted `OperationsProvider` interface for testability
- **IPC refactor**: split `ipc.ts` into focused modules; unified EngineClient into single `execCommand` method; added input validation to IDE IPC handlers
- **Resolve refactor**: replaced `workflowContinue` with `resolvePrepare`; injected session.Manager into Resolver; sink resolve/status orchestration into internal packages
- **Sync cleanup**: merged workflow helpers into syncer, deleted `workflow.go`; eliminated `conflict` package and deduplicated workflow helpers; removed unused Machine abstraction
- **Agent**: replaced `truncateOutput` with `extractSummary`
- **Types cleanup**: removed unused EngineChannel, WorkflowStep, duplicate PostSyncCommand, deprecated wrappers and dead code

### Fixed
- Active streams not cleaned up on app quit
- Merge conflict detection robustness
- `useDebouncedConfigMap` useRef initialization and missing dependency array
- Extra closing braces in ScanDialog and HomePage
- Test assertion logic in TestMerge_NoUpstream

## [v0.3.0]

### Added
- Real-time streaming terminal UI for agent conflict resolution
- Diff viewer for repository changes
- Sync workflow tracking and management with step details
- Auto-summary feature when streaming completes
- Auto-confirm summarization trigger
- Retry resolve option and PID-based lock file detection
- Debug logging for repo, sync, and git operations
- Polling and cleanup for agent events/logs
- Default IDE selection when opening repo
- Label component and Toggle component in settings
- `exclude` option to engine status command

### Changed
- Config directory moved from XDG_CONFIG_HOME to `~/.forksync`
- Centralized git operations with proxy support
- Consolidated workflow step functions and status updates
- Simplified post-sync command error handling
- Extracted shared Modal component, useAnimatedMount hook, useDebouncedConfig hook
- Replaced manual workflow iteration with AdvanceStep/MarkWorkflowDone
- Removed droid agent support
- Collapsed completeAgentResolve and runResolveAccept into finalizeCommitWithWorkflow

### Fixed
- Workflow status not updating when agent finishes
- Resolved files not populated from conflict paths
- Stream cleanup on reject and workflow actions
- Sync poll race condition with reference counting
- Interference with active sync operations
- Windows shell injection vulnerability (cmd /c start)
- Content-Security-Policy headers and renderer sandbox
- Input validation for all IPC handlers
- Theme FOUC with synchronous script in index.html
- Various memory leaks (debounce timers, setTimeout cleanup, effect duplicates)
- i18n fallback locale alignment and duplicate key resolution
- Stale repo error messages and stream events not cleared
- BehindBy not reset when repo is up-to-date

## [v0.2.0]

### Added
- AI conflict resolution with confirmation flow, session manager, and history with AI summaries
- Timeout handling for git operations and commands
- Loading states and concurrent operation prevention
- Linux platform support with Flatpak packaging

### Changed
- UI redesigned as single-page layout with Lucide icons and collapsible panels
- Improved conflict resolution flow, git operations, and error handling
- Replaced string literals with enum types and centralized constants

### Fixed
- Merge conflict detection from external git operations
- Merge rollback using `git merge --abort`
- Agent crash recovery and duplicate sync prevention

## [v0.1.0]

### Added
- First release of ForkSync
- Go CLI engine for Git fork synchronization
- Electron desktop app with React frontend
- Auto-sync GitHub fork repositories with upstream
- YAML configuration support
- macOS notifications
- Directory scanning for fork repos
