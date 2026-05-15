<div align="center">

# ForkSync

**Auto-sync your GitHub fork repos — resolve conflicts with AI.**

[English](./README.md) · [中文](./README_zh.md)

[![Go](https://img.shields.io/badge/Go-1.22+-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![Electron](https://img.shields.io/badge/Electron-31-47848F?logo=electron&logoColor=white)](https://www.electronjs.org/)
[![React](https://img.shields.io/badge/React-18-61DAFB?logo=react&logoColor=black)](https://react.dev/)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](./LICENSE)

</div>

<p align="center">
  <img src="image/README/1776830486988.png" alt="ForkSync Desktop App" width="720">
</p>

---

## Why ForkSync?

Maintaining forked repositories is tedious. Upstream authors keep shipping changes, and every sync risks merge conflicts. You either:

- **Forget to sync** — your fork falls behind, missing bug fixes and features
- **Resolve conflicts manually** — reading `<<<<<<<` markers for hours
- **Give up and re-fork** — losing your local modifications

**ForkSync solves this.** It automatically syncs your forks and uses AI coding agents (Claude Code, OpenCode, Codex) to resolve merge conflicts — so you never have to touch conflict markers again.

## Key Features

| Feature | Description |
|---------|-------------|
| **Auto Sync** | Periodically fetches and merges upstream changes (configurable interval) |
| **AI Conflict Resolution** | Delegates merge conflicts to AI agents with git-aware prompts |
| **Workflow-guided UI** | Step-by-step workflow: fetch → merge → detect conflicts → agent resolve → review → commit |
| **Live Agent Terminal** | Real-time streaming view of agent output (stdout, tool calls, errors) during resolution |
| **Desktop App** | Polished Electron GUI — dashboard, workflow steps, settings |
| **CLI** | Full-featured command-line tool for terminal workflows |
| **Directory Scanner** | Recursively scans any directory to discover and batch-add fork repos |
| **Sync History** | SQLite-backed history with filters, AI-generated summaries, and cleanup |
| **System Notifications** | Desktop native alerts on sync success, conflicts, or errors |
| **IDE Integration** | Open repos directly in VSCode, Cursor, or Trae |
| **Post-sync Commands** | Execute custom scripts after a successful sync (e.g. `pip install`, `npm build`) |
| **i18n** | Multi-language interface (Chinese / English) |
| **Multiple Agents** | Switch between Claude Code and OpenCode freely |

---

## Install

### Download

Grab the latest release for your platform:

| Platform | Format | Link |
|----------|--------|------|
| macOS | `.dmg` | [Releases](https://github.com/loongxjin/forksync/releases) |
| Linux | `.AppImage` | [Releases](https://github.com/loongxjin/forksync/releases) |
| Windows | `.exe` (NSIS) | [Releases](https://github.com/loongxjin/forksync/releases) |

### Build from Source

```bash
git clone https://github.com/loongxjin/forksync.git
cd forksync

# Full build (Go engine + Electron app)
make build
# Output: app/dist/
```

### CLI Only

```bash
cd engine && go build -o forksync . && ./forksync --help
```

---

## Quick Start

### 1. Configure GitHub Token (Recommended)

```bash
mkdir -p ~/.forksync
```

Edit `~/.forksync/config.yaml`:

```yaml
github:
  token: "ghp_your_token_here"
```

> Token is optional but recommended — it enables automatic upstream detection via GitHub API.

### 2. Add Repos

```bash
# Add a single repo
forksync add ~/projects/my-fork

# Scan a directory to batch-discover fork repos
forksync scan ~/projects
```

### 3. Sync

```bash
# Sync all repos
forksync sync --all

# Sync a specific repo
forksync sync my-fork

# Start background sync service (every 30 min)
forksync serve
```

### 4. Resolve Conflicts with AI

```bash
# Resolve conflicts using AI (interactive)
forksync resolve my-fork

# Use a specific agent, auto-commit
forksync resolve my-fork --agent opencode --no-confirm

# Accept / reject a resolution
forksync resolve my-fork --accept
forksync resolve my-fork --reject
```

### 5. Launch Desktop App

```bash
cd app && npm install && npm run dev
```

---

## AI Conflict Resolution

This is the core feature that sets ForkSync apart. When a sync produces merge conflicts, ForkSync can automatically delegate resolution to an AI coding agent:

```
┌─────────────┐    conflict     ┌───────────────┐    resolve    ┌────────────────┐
│   Upstream   │ ──────────────▶ │  ForkSync     │ ────────────▶│  AI Agent      │
│   Change     │                 │  detects      │              │  (Claude /     │
└─────────────┘                 │  conflict     │              │   OpenCode)    │
                                └───────────────┘              └───────┬────────┘
                                                                       │ resolved
                                                                       ▼
                                ┌───────────────┐              ┌────────────────┐
                                │  ForkSync     │ ◀───────────│  Verify &      │
                                │  commits      │   commit    │  Stage         │
                                └───────────────┘              └────────────────┘
```

**Supported Agents:**

| Agent | Binary | Auto-detected |
|-------|--------|:------------:|
| Claude Code | `claude` | ✅ |
| OpenCode | `opencode` | ✅ |
| Codex | `codex` | ✅ |

Agents are auto-discovered via `PATH`. Set a preferred agent in config:

```yaml
agent:
  preferred: "claude"
```

**Conflict resolution strategies:**

| Strategy | Config key | Behavior |
|----------|-----------|----------|
| Auto-resolve with agent | `conflict_strategy: agent_resolve` | Agent resolves conflicts automatically |
| Manual resolve | `conflict_strategy: manual` | Pause at workflow — user chooses to resolve with agent or manually |
| Preserve local | `resolve_strategy: preserve_ours` | Agent told to keep local changes, accept upstream non-conflicting |
| Preserve upstream | `resolve_strategy: preserve_theirs` | Agent told to prefer upstream changes |
| Balanced | `resolve_strategy: balanced` | Agent told to smart-merge preserving both sides |

**Confirmation modes:**

| Config | Behavior |
|--------|----------|
| `confirm_before_commit: true` | After agent resolves, wait for user review and accept/reject |
| `confirm_before_commit: false` | Auto-commit immediately after agent resolves |

**Post-sync Commands:**

Run custom scripts after each successful sync:

```bash
forksync post-sync add my-repo --name "install" --cmd "pip install -e ."
forksync post-sync list my-repo
forksync post-sync remove my-repo --id <cmd-id>
```

---

## Desktop App

Built with **Electron** + **React** + **TypeScript** + **Tailwind CSS** + **shadcn/ui**.

| Section | Description |
|------|-------------|
| **Dashboard** | Overview: repo statuses, recent sync activity |
| **Repo List** | Expandable repo cards with workflow steps or detail panel |
| **Workflow Steps** | Step-by-step progress: fetch → merge → check conflicts → resolve strategy → agent resolve → accept → commit |
| **Agent Terminal** | Real-time streaming view of agent output during resolution |
| **AI Summary** | After resolution, agent's git-history-aware summary in workflow |
| **Diff Viewer** | Side-by-side diff preview when reviewing changes |
| **History** | Sync timeline with filters, AI-generated summaries, and cleanup |
| **Settings** | General, agent config, post-sync commands, IDE preferences |

**Architecture:**

```
┌───────────────────────────────────┐
│       Electron UI (React)          │
│  Dashboard · Repos · Workflow      │
│  Agent Terminal · History          │
└───────────────┬───────────────────┘
                │ IPC (contextBridge)
┌───────────────▼───────────────────┐
│     EngineClient (TypeScript)      │
│  Spawns Go binary, parses JSON     │
└───────────────┬───────────────────┘
                │ --json / --stream flag
┌───────────────▼───────────────────┐
│        Go CLI Engine (Cobra)       │
│  add · sync · resolve · serve      │
│  agent · config · history          │
│  workflow · post-sync · summarize  │
└───────────────────────────────────┘
```

---

## CLI Reference

All commands support `--json` for structured output.

```bash
# Repository management
forksync add <path> [--upstream <url>]       # Add repo
forksync remove <name>                        # Remove from tracking
forksync scan <directory>                     # Batch-discover fork repos

# Sync
forksync sync [--all | <name>]               # Sync repos
forksync serve [--interval 15m]              # Background sync service
forksync status                              # Show all repo statuses

# AI conflict resolution
forksync resolve <name> [--agent claude] [--no-confirm] [--accept] [--reject]
forksync resolve <name> --stream             # Stream agent output as NDJSON (Electron)

# Workflow management
forksync workflow continue <name> --action {accept|reject|abort|resolve_with_agent|retry_commit|continue_manual}

# Post-sync commands
forksync post-sync list <name>
forksync post-sync add <name> --name <name> --cmd <command>
forksync post-sync remove <name> --id <cmd-id>

# AI summarization
forksync summarize <name> [--retry]          # Generate AI summary for last sync

# Agent management
forksync agent list                          # Detect installed agents
forksync agent sessions                      # List active sessions
forksync agent cleanup                       # Remove expired sessions
forksync agent reset <name>                  # Remove session for specific repo

# Configuration
forksync config get                          # Show all config
forksync config set <key> <value>            # Set config value
forksync config keys                         # List available keys

# History
forksync history [--limit 20] [--cleanup [--keep-days 30]]
```

---

## Configuration

**Location:** `~/.forksync/config.yaml`

```yaml
sync:
  default_interval: "30m"
  sync_on_startup: true
  auto_launch: false
  auto_summary: true
  summary_agent: "claude"
  summary_language: "zh"               # zh / en
  summary_timeout: "3m"

agent:
  preferred: "claude"
  priority: [claude, opencode, codex]
  timeout: "10m"
  conflict_strategy: "agent_resolve"   # agent_resolve / manual
  resolve_strategy: "preserve_ours"    # preserve_ours / preserve_theirs / balanced
  confirm_before_commit: true
  session_ttl: "24h"

github:
  token: ""

notification:
  enabled: true

proxy:
  enabled: false
  url: ""
```

**Data files:**

| Path | Purpose |
|------|---------|
| `~/.forksync/config.yaml` | User configuration |
| `~/.forksync/repos.json` | Managed repository list |
| `~/.forksync/sessions/<id>.json` | Agent session records |
| `~/.forksync/agent-logs/<repo>/` | Agent stream log files (NDJSON) |
| `~/.forksync/db/sync_history.db` | SQLite sync history |
| `~/.forksync/logs/sync-*.log` | Daily-rotated log files |

---

## Project Structure

```
forksync/
├── engine/                      # Go CLI engine
│   ├── cmd/                     # Cobra commands (sync, resolve, workflow, agent, etc.)
│   ├── internal/
│   │   ├── agent/               # AI agent adapters (Claude, OpenCode, Codex)
│   │   │   └── session/         # Session lifecycle management
│   │   ├── config/              # Viper-based YAML config
│   │   ├── conflict/            # Merge conflict detection
│   │   ├── git/                 # Git operations (native + CLI)
│   │   ├── history/             # SQLite sync history store
│   │   ├── logger/              # File logger with daily rotation
│   │   ├── notify/              # Desktop system notifications
│   │   ├── repo/                # Repository JSON store (thread-safe)
│   │   ├── scheduler/           # Background sync scheduler
│   │   ├── summarizer/          # AI-powered sync commit summarizer
│   │   └── sync/                # Core sync pipeline
│   └── pkg/types/               # Shared types
│
├── app/                         # Electron desktop app
│   ├── src/main/                # Electron main process + EngineClient
│   ├── src/preload/             # Context bridge (window.api)
│   └── src/renderer/            # React UI (pages, components, contexts)
│
├── build/                       # Build scripts
└── docs/                        # Documentation
```

---

## Testing

```bash
cd engine && go test ./... -v
```

**146 tests** across 15 test files — covering sync pipeline, agent adapters, session management, git operations, conflict detection, config, history, and more.

---

## Development

See [Development Guide](./docs/DEVELOPMENT.md) for setup instructions and architecture details.

## License

[MIT](./LICENSE)
