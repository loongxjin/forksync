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

# 3. 查看状态
forksync status

# 4. 同步
forksync sync my-fork
forksync sync --all
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
显示所有已管理仓库的状态、ahead/behind 计数。

```bash
forksync status
forksync status --json
```

状态图标：🟢 synced / 🟡 syncing / 🔴 conflict / ❌ error / ⚪ unconfigured

### `sync [repo-name]`
同步 fork 与 upstream。先 fetch，再 merge。

```bash
forksync sync my-fork          # 同步单个仓库
forksync sync --all            # 同步所有仓库
forksync sync --all --json     # JSON 输出
```

当仓库 `conflictStrategy` 设为 `"ai_resolve"` 且配置了 AI provider 时，冲突会自动尝试 AI 解决。

### `resolve <repo-name>`
解决合并冲突。

```bash
# 查看冲突文件
forksync resolve my-fork

# AI 自动解决（写文件 + git add + commit）
forksync resolve my-fork --ai

# 手动接受解决（写入内容 + git add）
forksync resolve my-fork --accept path/to/file.go --content "resolved content"

# 标记冲突已全部解决（完成 merge commit）
forksync resolve my-fork --done
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

ai:
  default_provider: "openai"
  providers:
    openai:
      api_key: "sk-..."
      model: "gpt-4"
      base_url: ""           # 可选，自定义 API 端点

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
| `status` | `StatusData` | `{ repos: Repo[] }` |
| `sync` | `SyncData` | `{ results: SyncResult[] }` |
| `resolve` | `ResolveData` | `{ repoId, conflicts: ConflictFile[] }` |
| `resolve --accept` | `AcceptData` | `{ repoId, file, resolved }` |
| `resolve --done` | `DoneData` | `{ repoId, allResolved, remainingConflicts }` |
| `remove` | `{ removed }` | `{ removed: string }` |
| `serve` | `ServeStatus` | `{ running, interval, message }` |

### RepoStatus 枚举

| 值 | 含义 |
|----|------|
| `synced` | 已同步 |
| `syncing` | 同步中 |
| `conflict` | 有冲突 |
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
│   ├── status.go            # 查看状态
│   ├── sync.go              # 同步仓库
│   ├── resolve.go           # 解决冲突 (--ai / --accept / --done)
│   ├── remove.go            # 移除仓库
│   └── serve.go             # 后台服务
├── pkg/types/               # 共享类型（JSON 契约）
└── internal/
    ├── config/              # 配置管理 (viper + yaml)
    ├── repo/                # 仓库管理 (JSON 持久化, 线程安全)
    ├── git/                 # Git 操作 (go-git + CLI fallback)
    ├── github/              # GitHub API (fork 检测)
    ├── sync/                # 同步编排 (per-repo mutex, 通知, AI)
    ├── conflict/            # 冲突检测与解析
    ├── ai/                  # AI 冲突解决 (OpenAI + 自定义 base_url)
    ├── notify/              # macOS 系统通知 (osascript)
    └── scheduler/           # 定时调度 (ticker-based)
```

## 测试

```bash
go test ./...              # 运行所有测试
go test ./internal/... -v  # 详细输出
```

| 包 | 测试数 |
|----|--------|
| config | 7 |
| repo | 23 |
| git | 14 |
| github | 21 |
| conflict | 25 |
| sync | 18 |
| **总计** | **108** |

## 依赖

- [cobra](https://github.com/spf13/cobra) — CLI 框架
- [viper](https://github.com/spf13/viper) — 配置管理
- [go-git](https://github.com/go-git/go-git/v5) — Git 操作
- [go-openai](https://github.com/sashabaranov/go-openai) — OpenAI API
- [uuid](https://github.com/google/uuid) — ID 生成
