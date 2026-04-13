# ForkSync

**English** | [中文](./README_zh.md)

<p align="center">
  <strong>Keep your GitHub fork repos in sync — automatically.</strong>
</p>

ForkSync monitors your forked repositories, fetches upstream changes, and resolves merge conflicts with AI agents. It runs as a **desktop app** (Electron + Go) or a **CLI** — your choice.

---

## ✨ Features

- 🔄 **Auto Sync** — Periodically fetches and merges upstream changes
- 🤖 **AI Conflict Resolution** — Integrates Claude Code, OpenCode, Droid, and Codex for automatic merge conflict resolution
- 📊 **Desktop Dashboard** — React-based UI to manage repos, view conflicts, and configure agents
- 🔔 **macOS Notifications** — Get notified on sync success, conflicts, or errors
- 🔍 **Directory Scanning** — Scan any directory to discover and batch-add fork repos
- 📝 **Sync History** — SQLite-backed history with filters and cleanup
- 🌐 **i18n Support** — Multi-language interface
- 🖥️ **IDE Integration** — Open repos in VSCode, Cursor, or Trae
- ⚙️ **Flexible Config** — YAML config with multiple conflict strategies and agent preferences

---

## 📦 Installation

### Download

Download the latest release for your platform:

| Platform | Format |
|----------|--------|
| macOS | `.dmg` |
| Linux | `.AppImage` |
| Windows | `.exe` (NSIS installer) |

### Build from Source

**Prerequisites:**

- Go 1.22+
- Node.js 18+
- npm 9+
- Git

```bash
# Clone the repo
git clone https://github.com/loongxjin/forksync.git
cd forksync

# Build everything (Go engine + Electron app)
./build/build.sh
```

The packaged app will be in `app/dist/`.

### CLI Only (No Desktop App)

```bash
cd engine
go build -o forksync .
./forksync --help
```

---

## 🚀 Quick Start

### 1. Set up GitHub Token (Optional)

ForkSync uses the GitHub API to detect fork parents. Set your token in the config:

```bash
mkdir -p ~/.forksync
```

Edit `~/.forksync/config.yaml`:

```yaml
github:
  token: "ghp_your_token_here"
```

> **Note:** Token is optional but recommended. Without it, upstream detection relies on git remotes.

### 2. Add Repositories

```bash
# Add a single repo
forksync add ~/projects/my-fork

# Scan a directory for fork repos
forksync scan ~/projects
```

### 3. Sync

```bash
# Sync all repos
forksync sync --all

# Sync a specific repo
forksync sync my-fork

# Start background sync service (every 30 minutes)
forksync serve
```

### 4. Desktop App

```bash
cd app
npm install
npm run dev
```

The Electron app starts with a dashboard showing all managed repos, sync status, and conflict alerts.

---

## 🖥️ Desktop App

Built with **Electron** + **React** + **TypeScript** + **Tailwind CSS**.

### Pages

| Page | Route | Description |
|------|-------|-------------|
| **Dashboard** | `/` | Overview: synced/conflict/syncing counts, recent activity, agent status |
| **Repos** | `/repos` | Manage repos: add, scan directory, sync, remove |
| **Conflicts** | `/conflicts` | List repos with conflicts, resolve via agents |
| **Conflict Detail** | `/conflicts/:repoId` | Diff viewer, agent summary, accept/reject resolution |
| **History** | `/history` | Sync history timeline with filters and cleanup |
| **Settings** | `/settings` | General settings, agent configuration, IDE preferences |

### Architecture

```
┌───────────────────────────────────┐
│       Electron UI (React)          │
│  Dashboard · Repos · Conflicts     │
│  History · Settings · ConflictDetail │
└───────────────┬───────────────────┘
                │ IPC (contextBridge)
┌───────────────▼───────────────────┐
│     EngineClient (TypeScript)      │
│  Spawns Go binary, parses JSON     │
└───────────────┬───────────────────┘
                │ CLI (--json flag)
┌───────────────▼───────────────────┐
│        Go CLI Engine (Cobra)       │
│  add · remove · scan · sync        │
│  status · resolve · serve          │
│  agent · config · history          │
└───────────────────────────────────┘
```

---

## ⌨️ CLI Reference

All commands support `--json` for structured output (used by the desktop app).

### `forksync add <path>`

Add a repository to management.

```bash
forksync add ~/projects/my-fork
forksync add ~/projects/my-fork --upstream https://github.com/upstream/repo.git
```

| Flag | Description |
|------|-------------|
| `--upstream <url>` | Upstream URL (auto-detected via GitHub API if omitted) |

### `forksync remove <name>`

Remove a repo from tracking (does **not** delete the local repo).

```bash
forksync remove my-fork
```

### `forksync scan <directory>`

Recursively scan a directory for git fork repos.

```bash
forksync scan ~/projects
```

### `forksync sync [repo-name]`

Sync fork repos with their upstream.

```bash
forksync sync --all          # Sync all managed repos
forksync sync my-fork        # Sync a specific repo
```

| Flag | Description |
|------|-------------|
| `--all` | Sync all managed repositories |

### `forksync status`

Show status of all managed repos.

```bash
forksync status
```

Status icons: 🟢 synced · 🟡 syncing · 🔴 conflict · 🟠 resolving · ✅ resolved · ❌ error · ⚪ unconfigured

### `forksync resolve <repo-name>`

Resolve merge conflicts with AI agents.

```bash
forksync resolve my-fork                     # Interactive resolve
forksync resolve my-fork --agent claude      # Use specific agent
forksync resolve my-fork --no-confirm        # Auto-commit without confirmation
forksync resolve my-fork --accept            # Mark as resolved (accept)
forksync resolve my-fork --reject            # Reject and rollback
```

| Flag | Description |
|------|-------------|
| `--agent <name>` | Use specific agent: `claude`, `opencode`, `droid`, `codex` |
| `--no-confirm` | Auto-commit resolution without user confirmation |
| `--accept` | Accept all conflicts as resolved |
| `--reject` | Reject last resolution (rollback via `git checkout`) |

### `forksync serve`

Start background sync service.

```bash
forksync serve                # Default: every 30 minutes
forksync serve --interval 15m # Custom interval
```

| Flag | Description |
|------|-------------|
| `--interval <duration>` | Sync interval (e.g., `15m`, `1h`, `2h`) |

### `forksync agent`

Manage AI agent integrations.

```bash
forksync agent list       # Detect installed agents
forksync agent sessions   # List active agent sessions
forksync agent cleanup    # Remove expired/failed sessions
```

### `forksync config`

Manage ForkSync configuration.

```bash
forksync config get                          # Show all config values
forksync config set agent.preferred claude   # Set a config value (dot-notation)
forksync config set sync.default_interval 1h # Set sync interval
forksync config keys                        # List all available keys
```

| Subcommand | Description |
|------------|-------------|
| `get` | Display all configuration values |
| `set <key> <value>` | Set a configuration value (dot-notation keys) |
| `keys` | List all supported configuration keys and types |

### `forksync history [repo-name]`

Show and manage sync history.

```bash
forksync history                # Show recent history for all repos
forksync history my-fork        # Show history for a specific repo
forksync history --cleanup      # Clear all history
forksync history --cleanup --keep-days 30  # Keep last 30 days
```

| Flag | Description |
|------|-------------|
| `--limit <n>` | Number of records to show (default: 20) |
| `--cleanup` | Clean up sync history |
| `--keep-days <n>` | Keep records from last N days when cleaning up |

---

## 🤖 AI Agent Support

ForkSync integrates with four AI coding agents for automatic merge conflict resolution:

| Agent | Binary | Key Flags |
|-------|--------|-----------|
| **Claude Code** | `claude` | `--print --dangerously-skip-permissions` |
| **OpenCode** | `opencode` | `run --session` |
| **Droid** | `droid` | `exec --auto high` |
| **Codex** | `codex` | `--dangerously-bypass-approvals-and-sandbox` |

Agents are auto-discovered via `PATH`. Set a preferred agent in config or let ForkSync pick the first available one.

### Conflict Resolution Strategies

| Strategy | Description |
|----------|-------------|
| `preserve_ours` | Keep local changes, accept non-conflicting upstream changes |
| `accept_theirs` | Prefer upstream changes, keep local only where necessary |
| `balanced` | Smart merge, try to preserve both sides' changes |
| `agent_resolve` | Delegate to AI agent for automatic resolution |

---

## ⚙️ Configuration

**Location:** `~/.forksync/config.yaml`

```yaml
sync:
  default_interval: "30m"        # Sync interval
  sync_on_startup: true          # Run sync immediately on serve start
  auto_launch: false             # Auto-launch on login

agent:
  preferred: ""                  # Preferred agent (e.g., "claude")
  priority:                      # Agent priority order
    - claude
    - opencode
    - droid
    - codex
  timeout: "10m"                 # Agent resolve timeout
  conflict_strategy: "preserve_ours"  # preserve_ours | accept_theirs | balanced | agent_resolve
  confirm_before_commit: true    # Show diff before committing agent changes
  session_ttl: "24h"             # Agent session expiration

github:
  token: ""                      # GitHub personal access token

notification:
  enabled: true

proxy:
  enabled: false
  url: ""
```

**Data files:**

| File | Purpose |
|------|---------|
| `~/.forksync/config.yaml` | User configuration |
| `~/.forksync/repos.json` | Managed repository list |
| `~/.forksync/sessions/<id>.json` | Agent session records |
| `~/.forksync/db/forksync.db` | SQLite sync history database |
| `~/.forksync/logs/sync-*.log` | Daily-rotated log files |

---

## 🏗️ Project Structure

```
forksync/
├── engine/                      # Go CLI engine
│   ├── cmd/                     # Cobra commands (add, sync, resolve, config, history, etc.)
│   ├── internal/
│   │   ├── agent/               # AI agent adapters + registry
│   │   │   ├── session/         # Session manager + persistent store
│   │   │   ├── claude.go        # Claude Code adapter
│   │   │   ├── opencode.go      # OpenCode adapter
│   │   │   ├── droid.go         # Droid adapter
│   │   │   └── codex.go         # Codex adapter
│   │   ├── config/              # Viper-based config management
│   │   ├── conflict/            # Conflict detection
│   │   ├── git/                 # Git operations (go-git + CLI fallback)
│   │   ├── github/              # GitHub API client
│   │   ├── history/             # SQLite-backed sync history store
│   │   ├── logger/              # File-based logging with daily rotation
│   │   ├── notify/              # macOS notifications
│   │   ├── repo/                # Repository JSON store
│   │   ├── scheduler/           # Background sync scheduler
│   │   └── sync/                # Sync pipeline
│   └── pkg/types/               # Shared types (Repo, SyncResult, etc.)
│
├── app/                         # Electron desktop app
│   ├── src/
│   │   ├── main/                # Electron main process
│   │   │   ├── index.ts         # Window creation
│   │   │   ├── engine.ts        # EngineClient (spawns Go binary)
│   │   │   ├── ipc.ts           # IPC handler registration
│   │   │   ├── i18n.ts          # Internationalization helper
│   │   │   ├── ide.ts           # IDE detection & management (VSCode, Cursor, Trae)
│   │   │   └── notify.ts        # System notifications with click-through
│   │   ├── preload/             # Context bridge (window.api)
│   │   └── renderer/            # React UI
│   │       ├── src/
│   │       │   ├── pages/       # Dashboard, Repos, Conflicts, History, Settings
│   │       │   ├── components/  # UI components (dialogs, badges, etc.)
│   │       │   ├── contexts/    # React Context state management
│   │       │   ├── hooks/       # Custom hooks (useTheme)
│   │       │   └── lib/         # API wrapper + utilities
│   │       └── App.tsx          # Root component with router
│   └── electron-builder.yml     # Packaging config
│
├── build/
│   └── build.sh                 # Unified build script
│
└── docs/                        # Documentation
```

---

## 🧪 Testing

The Go engine has comprehensive test coverage:

```bash
cd engine
go test ./... -v
```

**146 tests** across 15 test files covering:
- Repository store CRUD
- Sync pipeline
- Agent adapters & provider interface
- Session manager & store
- Agent registry & discovery
- Config loading & saving
- GitHub API client
- Type serialization
- Git operations
- Conflict detection
- Sync history store & cleanup
- Logger & log rotation

---

## 🛠️ Development

Want to contribute? Check out the [Development Guide](./docs/DEVELOPMENT.md) for setup instructions, architecture overview, and development workflows.

---

## 📝 License

This project is licensed under the MIT License.
