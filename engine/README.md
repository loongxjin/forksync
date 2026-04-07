# ForkSync Engine

Go 核心引擎 for ForkSync — 自动同步 fork 仓库的 macOS 桌面应用。

Electron UI 通过 spawning 此二进制文件并解析 `--json` 输出进行通信。

## 构建

```bash
cd engine
bash build.sh
# 输出: bins/forksync (darwin/arm64)
```

## 快速开始

```bash
# 1. 扫描目录中的 git 仓库（自动检测 fork）
forksync scan ~/projects

# 2. 添加仓库到管理（自动检测 upstream）
forksync add ~/projects/my-fork

# 3. 查看状态（含 agent 检测）
forksync status

# 4. 同步
forksync sync my-fork
forksync sync --all

# 5. 检测可用的 AI agent
forksync agent list
```

## 命令参考

### `scan <directory>`
扫描目录树，检测 git 仓库并识别 GitHub fork。

```bash
forksync scan ~/projects
forksync scan ~/projects --json
```

输出 `ScanData`：每个仓库的 path、name、origin、isFork、suggestedUpstream。

### `add <repo-path>`
添加仓库到 ForkSync 管理。自动通过 GitHub API 检测 upstream。

```bash
forksync add ~/projects/my-fork
forksync add ~/projects/my-fork --upstream https://github.com/original/repo.git
```

### `status`
显示所有已管理仓库的状态、ahead/behind 计数，以及检测到的 AI agent。

```bash
forksync status
forksync status --json
```

状态图标：🟢 synced / 🟡 syncing / 🔴 conflict / 🟠 resolving / 🟣 resolved / ❌ error / ⚪ unconfigured

### `sync [repo-name]`
同步 fork 与 upstream。先 fetch，再 merge。

```bash
forksync sync my-fork          # 同步单个仓库
forksync sync --all            # 同步所有仓库
forksync sync --all --json     # JSON 输出
```

当仓库 `conflictStrategy` 设为 `"agent_resolve"` 且 `sessionMgr` 可用时，冲突会自动尝试通过 AI agent 解决。

### `resolve <repo-name>`
使用 AI agent 解决合并冲突。

```bash
# 自动解决（使用首选 agent）
forksync resolve my-repo

# 指定 agent
forksync resolve my-repo --agent claude

# 自动提交，无需确认
forksync resolve my-repo --no-confirm

# 拒绝上次解决结果（回滚）
forksync resolve my-repo --reject

# 标记冲突已全部解决（完成 merge commit）
forksync resolve my-repo --done
```

**工作流程：**
1. 检测冲突文件
2. 选择 agent（显式 `--agent` 或配置中的首选）
3. 创建/恢复 agent 会话，发送解决请求
4. 验证冲突标记已消除
5. 显示 diff，等待用户确认（除非 `--no-confirm`）
6. 确认后 `git add` + `git commit`

### `agent`
管理 AI agent 集成。支持检测 Claude Code、OpenCode、Droid、Codex。

```bash
forksync agent list       # 列出所有支持的 agent 及安装状态
forksync agent sessions   # 列出活跃的 agent 会话
forksync agent cleanup    # 清理过期和失败的会话记录
```

### `remove <repo-name>`
从 ForkSync 管理中移除仓库（不删除本地文件）。

```bash
forksync remove my-fork
```

### `serve`
启动后台调度服务，定时同步所有仓库。供 Electron UI 调用。

```bash
forksync serve                    # 使用配置的间隔
forksync serve --interval 15m     # 覆盖间隔
forksync serve --json             # JSON 输出状态
```

收到 SIGINT/SIGTERM 优雅退出。

## 配置

配置文件位于 `~/.forksync/config.yaml`：

```yaml
sync:
  default_interval: "30m"    # 同步间隔
  sync_on_startup: true      # 启动时同步
  auto_launch: false         # 开机自启

github:
  token: ""                  # GitHub PAT，提高 API 限流

agent:
  preferred: ""              # 首选 agent（claude/opencode/droid/codex）
  priority:                  # agent 优先级顺序
    - claude
    - opencode
    - droid
    - codex
  timeout: "10m"             # agent 操作超时
  conflict_strategy: "preserve_ours"  # 冲突解决策略
  confirm_before_commit: true         # 提交前确认
  session_ttl: "24h"         # 会话过期时间

notification:
  enabled: true
  on_conflict: true
  on_sync_success: false

proxy:
  enabled: false
  url: ""
```

数据存储在 `~/.forksync/`：
- `config.yaml` — 配置
- `repos.json` — 仓库列表
- `sessions/` — Agent 会话记录（按仓库分目录）

## JSON 契约类型

所有命令支持 `--json` 输出，格式为 `ApiResponse[T]`：

```json
{
  "success": true,
  "data": { ... },
  "error": ""
}
```

| 命令 | Data 类型 | 说明 |
|------|-----------|------|
| `scan` | `ScanData` | `{ repos: ScannedRepo[] }` |
| `add` | `AddData` | `{ repo: Repo }` |
| `status` | `StatusData` | `{ repos: Repo[], agents: AgentInfo[], preferredAgent }` |
| `sync` | `SyncData` | `{ results: SyncResult[] }` |
| `resolve` | `ResolveData` | `{ repoId, conflicts, agentResult }` |
| `resolve --done` | `DoneData` | `{ repoId, allResolved, remainingConflicts }` |
| `resolve --reject` | `RejectData` | `{ repoId, rolledBack }` |
| `agent list` | `AgentListData` | `{ agents: AgentInfo[], preferred }` |
| `agent sessions` | `AgentSessionsData` | `{ sessions: AgentSessionInfo[] }` |
| `remove` | `{ removed }` | `{ removed: string }` |
| `serve` | `ServeStatus` | `{ running, interval, message }` |

### RepoStatus 枚举

| 值 | 含义 |
|----|------|
| `synced` | 已同步 |
| `syncing` | 同步中 |
| `conflict` | 有冲突 |
| `resolving` | Agent 正在解决 |
| `resolved` | 已解决，等待确认 |
| `error` | 出错 |
| `unconfigured` | 未配置 |
| `up_to_date` | 已是最新 |

## 项目结构

```
engine/
├── main.go                  # 入口
├── build.sh                 # 构建脚本
├── cmd/                     # Cobra CLI 命令
│   ├── root.go              # 根命令 + --json + outputJSON[T]
│   ├── scan.go              # 扫描目录
│   ├── add.go               # 添加仓库
│   ├── status.go            # 查看状态（含 agent 检测）
│   ├── sync.go              # 同步仓库
│   ├── resolve.go           # Agent 冲突解决 (--agent / --done / --reject)
│   ├── agent.go             # Agent 管理 (list / sessions / cleanup)
│   ├── remove.go            # 移除仓库
│   └── serve.go             # 后台服务
├── pkg/types/               # 共享类型（JSON 契约）
└── internal/
    ├── config/              # 配置管理 (viper + yaml)
    ├── repo/                # 仓库管理 (JSON 持久化, 线程安全)
    ├── git/                 # Git 操作 (go-git + CLI fallback)
    ├── github/              # GitHub API (fork 检测)
    ├── sync/                # 同步编排 (per-repo mutex, 通知, agent)
    ├── conflict/            # 冲突检测（简化：仅返回文件路径）
    ├── agent/               # Agent 集成
    │   ├── provider.go      # AgentProvider 接口
    │   ├── registry.go      # 自动发现与注册
    │   ├── claude.go        # Claude Code 适配器
    │   ├── opencode.go      # OpenCode 适配器
    │   ├── droid.go         # Droid 适配器
    │   ├── codex.go         # Codex 适配器
    │   └── session/         # 会话管理 (创建/恢复/过期清理)
    ├── notify/              # macOS 系统通知 (osascript)
    └── scheduler/           # 定时调度 (ticker-based)
```

## 支持的 AI Agent

| Agent | Binary | 自治模式 | 会话恢复 |
|-------|--------|----------|----------|
| Claude Code | `claude` | `--dangerously-skip-permissions` | `--resume <id>` |
| OpenCode | `opencode` | `--prompt <text>` | `--continue` |
| Droid | `droid` | `--auto high` | `--resume` |
| Codex | `codex` | `--dangerously-bypass-approvals-and-sandbox` | `resume --last` |

ForkSync 自动检测 PATH 中已安装的 agent，按配置的优先级选择。

## 测试

```bash
go test ./...              # 运行所有测试
go test ./internal/... -v  # 详细输出
```

| 包 | 测试数 |
|----|--------|
| types | 7 |
| config | 7 |
| repo | 24 |
| git | 14 |
| github | 4 |
| conflict | 5 |
| sync | 18 |
| agent | 28 |
| agent/session | 15 |
| **总计** | **122** |

## 依赖

- [cobra](https://github.com/spf13/cobra) — CLI 框架
- [viper](https://github.com/spf13/viper) — 配置管理
- [go-git](https://github.com/go-git/go-git/v5) — Git 操作
- [uuid](https://github.com/google/uuid) — ID 生成
- [sqlite](https://gitlab.com/cznic/sqlite) — 纯 Go SQLite（无 CGO）
- [testify](https://github.com/stretchr/testify) — 测试断言
