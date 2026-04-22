<div align="center">

# 🔀 ForkSync

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

- ❌ **Forget to sync** — your fork falls behind, missing bug fixes and features
- ❌ **Resolve conflicts manually** — reading `<<<<<<<` markers for hours
- ❌ **Give up and re-fork** — losing your local modifications

**ForkSync solves this.** It automatically syncs your forks and uses AI coding agents (Claude Code, OpenCode, Droid, Codex) to resolve merge conflicts — so you never have to touch conflict markers again.

## ✨ Key Features

| Feature | Description |
|---------|-------------|
| 🔄 **Auto Sync** | Periodically fetches and merges upstream changes (configurable interval) |
| 🤖 **AI Conflict Resolution** | Delegates merge conflicts to AI agents (Claude Code, OpenCode, Droid, Codex) |
| 🖥️ **Desktop App** | Polished Electron GUI — dashboard, conflict viewer, settings |
| ⌨️ **CLI** | Full-featured command-line tool for terminal workflows |
| 🔍 **Directory Scanner** | Recursively scans any directory to discover and batch-add fork repos |
| 📝 **Sync History** | SQLite-backed history with filters, AI-generated summaries, and cleanup |
| 🔔 **System Notifications** | macOS native alerts on sync success, conflicts, or errors |
| 🖥️ **IDE Integration** | Open repos directly in VSCode, Cursor, or Trae |
| 🌐 **i18n** | Multi-language interface |
| ⚙️ **Flexible Strategies** | `preserve_ours` / `preserve_theirs` / `balanced` / `agent_resolve` |

---

## 📦 Install

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

## 🚀 Quick Start

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
forksync resolve my-fork --agent claude --no-confirm
```

### 5. Launch Desktop App

```bash
cd app && npm install && npm run dev
```

---

## 🤖 AI Conflict Resolution

This is the core feature that sets ForkSync apart. When a sync produces merge conflicts, ForkSync can automatically delegate resolution to an AI coding agent:

```
┌─────────────┐    conflict     ┌───────────────┐    resolve    ┌────────────────┐
│   Upstream   │ ──────────────▶ │  ForkSync     │ ────────────▶│  AI Agent      │
│   Change     │                 │  detects      │              │  (Claude/etc.) │
└─────────────┘                 │  conflict     │              └───────┬────────┘
                                └───────────────┘                      │
                                                                       │ resolved
                                                                       ▼
                                ┌───────────────┐              ┌────────────────┐
                                │  ForkSync     │ ◀───────────│  Verify &      │
                                │  commits      │   commit    │  Commit        │
                                └───────────────┘              └────────────────┘
```

**Supported Agents:**

| Agent | Binary | Auto-detected |
|-------|--------|:------------:|
| Claude Code | `claude` | ✅ |
| OpenCode | `opencode` | ✅ |
| Droid | `droid` | ✅ |
| Codex | `codex` | ✅ |

Agents are auto-discovered via `PATH`. Set a preferred agent in config:

```yaml
agent:
  preferred: "claude"
  conflict_strategy: "agent_resolve"
```

**Resolution strategies:**

| Strategy | Behavior |
|----------|----------|
| `preserve_ours` | Keep local changes, accept non-conflicting upstream |
| `preserve_theirs` | Prefer upstream changes |
| `balanced` | Smart merge preserving both sides |
| `agent_resolve` | Delegate to AI agent |

---

## 🖥️ Desktop App

Built with **Electron** + **React** + **TypeScript** + **Tailwind CSS** + **shadcn/ui**.

| Page | Description |
|------|-------------|
| **Dashboard** | Overview: synced/conflict counts, recent activity, agent status |
| **Repos** | Add, scan, sync, remove repositories |
| **Conflicts** | List repos with conflicts, resolve via agents |
| **Conflict Detail** | Diff viewer, agent summary, accept/reject resolution |
| **History** | Sync timeline with filters and cleanup |
| **Settings** | General settings, agent config, IDE preferences, theme |

**Architecture:**

```
┌───────────────────────────────────┐
│       Electron UI (React)          │
│  Dashboard · Repos · Conflicts     │
│  History · Settings · Detail       │
└───────────────┬───────────────────┘
                │ IPC (contextBridge)
┌───────────────▼───────────────────┐
│     EngineClient (TypeScript)      │
│  Spawns Go binary, parses JSON     │
└───────────────┬───────────────────┘
                │ --json flag
┌───────────────▼───────────────────┐
│        Go CLI Engine (Cobra)       │
│  add · sync · resolve · serve      │
│  agent · config · history          │
└───────────────────────────────────┘
```

---

## ⌨️ CLI Reference

All commands support `--json` for structured output.

```bash
# Repository management
forksync add <path> [--upstream <url>]       # Add repo
forksync remove <name>                       # Remove from tracking
forksync scan <directory>                    # Batch-discover fork repos

# Sync
forksync sync [--all | <name>]              # Sync repos
forksync serve [--interval 15m]             # Background sync service
forksync status                             # Show all repo statuses

# AI conflict resolution
forksync resolve <name> [--agent claude] [--no-confirm] [--accept] [--reject]

# Agent management
forksync agent list                         # Detect installed agents
forksync agent sessions                     # List active sessions
forksync agent cleanup                      # Remove expired sessions

# Configuration
forksync config get                         # Show all config
forksync config set <key> <value>           # Set config value
forksync config keys                        # List available keys

# History
forksync history [--limit 20] [--cleanup [--keep-days 30]]
```

---

## ⚙️ Configuration

**Location:** `~/.forksync/config.yaml`

```yaml
sync:
  default_interval: "30m"
  sync_on_startup: true

agent:
  preferred: "claude"
  priority: [claude, opencode, droid, codex]
  timeout: "10m"
  conflict_strategy: "agent_resolve"
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
| `~/.forksync/db/forksync.db` | SQLite sync history |
| `~/.forksync/logs/sync-*.log` | Daily-rotated log files |

---

## 🏗️ Project Structure

```
forksync/
├── engine/                      # Go CLI engine
│   ├── cmd/                     # Cobra commands
│   ├── internal/
│   │   ├── agent/               # AI agent adapters (Claude, OpenCode, Droid, Codex)
│   │   │   └── session/         # Session lifecycle management
│   │   ├── config/              # Viper-based YAML config
│   │   ├── conflict/            # Merge conflict detection
│   │   ├── git/                 # Git operations (go-git + CLI fallback)
│   │   ├── github/              # GitHub REST API client
│   │   ├── history/             # SQLite sync history store
│   │   ├── logger/              # File logger with daily rotation
│   │   ├── notify/              # macOS system notifications
│   │   ├── repo/                # Repository JSON store (thread-safe)
│   │   ├── scheduler/           # Background sync scheduler
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

## 🧪 Testing

```bash
cd engine && go test ./... -v
```

**146 tests** across 15 test files — covering sync pipeline, agent adapters, session management, git operations, conflict detection, config, history, and more.

---

## 🛠️ Development

See [Development Guide](./docs/DEVELOPMENT.md) for setup instructions and architecture details.

## 📝 License

[MIT](./LICENSE)
