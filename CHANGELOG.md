# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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
