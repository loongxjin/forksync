# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [v0.5.0] — 2026-06-18

### Added
- Embedded HTTP/WebSocket engine server replacing Cobra CLI (bearer-token auth, crash supervision with exponential backoff)
- Real-time push channel via `/stream/events` WebSocket + in-process event bus, replacing fixed-interval polling
- Per-resolve session IDs, agent log files named by session ID
- Standalone tabbed settings page (`/settings`) replacing settings drawer
- Per-file diff viewer with retry and empty state
- Markdown rendering for AI summaries
- ConfirmDialog and ErrorBanner shared UI components
- `state_persisted` stream event

### Changed
- Engine architecture: Cobra CLI → long-running HTTP server; Electron side switched from `spawn` to `fetch` + `ws`
- Resolve overhaul: disk log as single source of truth; Resolve → Commit contract separated; auto-resolve and manual resolve share same Resolver core
- HomePage decomposed into 7 dedicated hooks (~700→415 lines)
- RepoRow and HistoryRow memoized with `React.memo`
- Auto-summary controls moved from General to Agent settings tab
- Session expiration removed (TTL, expired status, cleanup)
- Agent session list shows repo name instead of repoId
- `engineApi` seam unifies all renderer→engine calls
- CSP tightened for production builds

### Fixed
- GET `/status` event feedback loop
- Workflow rebuild loop dropping `resolveSessionId`
- Tick storm and double `STREAM_DONE` dispatch in resolve stream
- Double done frame suppressing resolve data
- Agent event duplication from snapshot/delta overlap
- 500-character summary truncation removed
- SQLite `SQLITE_BUSY` errors (single-conn, WAL, `BEGIN IMMEDIATE`)
- Stale workflows not cleared for settled repos
- `MERGE_HEAD` fallback for merge state detection
- Silent git ref resolution errors
- Notification permission API in Electron main process
- i18n locale propagation to main process and production packaging
- History refresh after auto-summarize
- Merge rollback on auto-resolve failure
- Various app migration fixes (Node 20 `ws` library, body encoding, port retry, process tree kill on Windows)

### Accessibility
- Modal, Sheet/Drawer: `role="dialog"`, `aria-modal`, focus trap
- Toast: `role="alert" aria-live="assertive"`
- RepoRow: `<button>` with `aria-expanded`, keyboard-operable
- ErrorBanner and ConfirmDialog replace hand-rolled inline errors and native `confirm()`/`alert()`

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
