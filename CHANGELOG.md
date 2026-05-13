# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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
