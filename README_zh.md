# ForkSync

[English](./README.md) | **中文**

<p align="center">
  <strong>自动同步你的 GitHub Fork 仓库。</strong>
</p>

ForkSync 监控你的 fork 仓库，自动拉取上游变更，并通过 AI Agent 解决合并冲突。支持**桌面应用**（Electron + Go）或**命令行工具**两种使用方式。

---

## ✨ 功能特性

- 🔄 **自动同步** — 定期拉取并合并上游变更
- 🤖 **AI 冲突解决** — 集成 Claude Code、OpenCode、Droid 和 Codex，自动解决合并冲突
- 📊 **桌面仪表盘** — 基于 React 的 UI，管理仓库、查看冲突、配置 Agent
- 🔔 **macOS 通知** — 同步成功、冲突或错误时发送系统通知
- 🔍 **目录扫描** — 扫描任意目录，自动发现并批量添加 fork 仓库
- 📝 **同步历史** — 基于 SQLite 的历史记录，支持筛选和清理
- 🌐 **国际化** — 多语言界面支持
- 🖥️ **IDE 集成** — 在 VSCode、Cursor 或 Trae 中打开仓库
- ⚙️ **灵活配置** — YAML 配置文件，支持多种冲突策略和 Agent 偏好设置

---

## 📦 安装

### 下载安装

根据你的平台下载最新版本：

| 平台 | 格式 |
|------|------|
| macOS | `.dmg` |
| Linux | `.AppImage` |
| Windows | `.exe`（NSIS 安装包） |

### 从源码构建

**前置条件：**

- Go 1.22+
- Node.js 18+
- npm 9+
- Git

```bash
# 克隆仓库
git clone https://github.com/loongxjin/forksync.git
cd forksync

# 构建全部（Go 引擎 + Electron 应用）
./build/build.sh
```

构建产物在 `app/dist/` 目录下。

### 仅使用命令行（不需要桌面应用）

```bash
cd engine
go build -o forksync .
./forksync --help
```

---

## 🚀 快速开始

### 1. 配置 GitHub Token（可选）

ForkSync 使用 GitHub API 检测 fork 的上游仓库。在配置文件中设置你的 Token：

```bash
mkdir -p ~/.forksync
```

编辑 `~/.forksync/config.yaml`：

```yaml
github:
  token: "ghp_你的token"
```

> **提示：** Token 是可选的，但强烈建议配置。没有 Token 时，上游检测将依赖 git remote 信息。

### 2. 添加仓库

```bash
# 添加单个仓库
forksync add ~/projects/my-fork

# 扫描目录，发现所有 fork 仓库
forksync scan ~/projects
```

### 3. 同步

```bash
# 同步所有仓库
forksync sync --all

# 同步指定仓库
forksync sync my-fork

# 启动后台同步服务（默认每 30 分钟）
forksync serve
```

### 4. 启动桌面应用

```bash
cd app
npm install
npm run dev
```

Electron 应用启动后会显示仪表盘，展示所有受管仓库、同步状态和冲突提醒。

---

## 🖥️ 桌面应用

基于 **Electron** + **React** + **TypeScript** + **Tailwind CSS** 构建。

### 页面一览

| 页面 | 路由 | 说明 |
|------|------|------|
| **仪表盘** | `/` | 总览：已同步/冲突/同步中数量，最近活动，Agent 状态 |
| **仓库管理** | `/repos` | 管理仓库：添加、扫描目录、同步、移除 |
| **冲突列表** | `/conflicts` | 列出有冲突的仓库，通过 Agent 解决 |
| **冲突详情** | `/conflicts/:repoId` | Diff 查看器、Agent 总结、接受/拒绝解决结果 |
| **同步历史** | `/history` | 同步历史时间线，支持筛选和清理 |
| **设置** | `/settings` | 通用设置、Agent 配置、IDE 偏好 |

### 架构

```
┌───────────────────────────────────┐
│       Electron UI (React)          │
│  仪表盘 · 仓库管理 · 冲突列表      │
│  同步历史 · 设置 · 冲突详情        │
└───────────────┬───────────────────┘
                │ IPC (contextBridge)
┌───────────────▼───────────────────┐
│     EngineClient (TypeScript)      │
│  启动 Go 二进制文件，解析 JSON 输出  │
└───────────────┬───────────────────┘
                │ CLI (--json 参数)
┌───────────────▼───────────────────┐
│        Go CLI 引擎 (Cobra)         │
│  add · remove · scan · sync        │
│  status · resolve · serve          │
│  agent · config · history          │
└───────────────────────────────────┘
```

---

## ⌨️ 命令行参考

所有命令支持 `--json` 参数输出结构化 JSON（桌面应用使用）。

### `forksync add <路径>`

添加仓库到管理列表。

```bash
forksync add ~/projects/my-fork
forksync add ~/projects/my-fork --upstream https://github.com/upstream/repo.git
```

| 参数 | 说明 |
|------|------|
| `--upstream <url>` | 上游仓库 URL（省略时通过 GitHub API 自动检测） |

### `forksync remove <名称>`

从管理列表中移除仓库（**不会**删除本地仓库）。

```bash
forksync remove my-fork
```

### `forksync scan <目录>`

递归扫描目录，查找 git fork 仓库。

```bash
forksync scan ~/projects
```

### `forksync sync [仓库名]`

同步 fork 仓库与上游。

```bash
forksync sync --all          # 同步所有受管仓库
forksync sync my-fork        # 同步指定仓库
```

| 参数 | 说明 |
|------|------|
| `--all` | 同步所有受管仓库 |

### `forksync status`

查看所有受管仓库的状态。

```bash
forksync status
```

状态图标：🟢 已同步 · 🟡 同步中 · 🔴 冲突 · 🟠 解决中 · ✅ 已解决 · ❌ 错误 · ⚪ 未配置

### `forksync resolve <仓库名>`

通过 AI Agent 解决合并冲突。

```bash
forksync resolve my-fork                     # 交互式解决
forksync resolve my-fork --agent claude      # 指定 Agent
forksync resolve my-fork --no-confirm        # 自动提交（无需确认）
forksync resolve my-fork --accept            # 标记为已解决（接受）
forksync resolve my-fork --reject            # 拒绝并回退
```

| 参数 | 说明 |
|------|------|
| `--agent <名称>` | 指定 Agent：`claude`、`opencode`、`droid`、`codex` |
| `--no-confirm` | 自动提交解决结果，无需用户确认 |
| `--accept` | 接受所有冲突为已解决 |
| `--reject` | 拒绝上次解决结果（通过 `git checkout` 回退） |

### `forksync serve`

启动后台同步服务。

```bash
forksync serve                # 默认：每 30 分钟
forksync serve --interval 15m # 自定义间隔
```

| 参数 | 说明 |
|------|------|
| `--interval <时长>` | 同步间隔（如 `15m`、`1h`、`2h`） |

### `forksync agent`

管理 AI Agent 集成。

```bash
forksync agent list       # 检测已安装的 Agent
forksync agent sessions   # 列出活跃的 Agent 会话
forksync agent cleanup    # 清理过期/失败的会话
```

### `forksync config`

管理 ForkSync 配置。

```bash
forksync config get                          # 显示所有配置值
forksync config set agent.preferred claude   # 设置配置值（点号分隔）
forksync config set sync.default_interval 1h # 设置同步间隔
forksync config keys                        # 列出所有可用的配置键
```

| 子命令 | 说明 |
|--------|------|
| `get` | 显示所有配置值 |
| `set <key> <value>` | 设置配置值（点号分隔的键名） |
| `keys` | 列出所有支持的配置键及其类型 |

### `forksync history [仓库名]`

查看和管理同步历史。

```bash
forksync history                # 查看所有仓库的最近历史
forksync history my-fork        # 查看指定仓库的历史
forksync history --cleanup      # 清理所有历史记录
forksync history --cleanup --keep-days 30  # 保留最近 30 天的记录
```

| 参数 | 说明 |
|------|------|
| `--limit <n>` | 显示记录数量（默认 20） |
| `--cleanup` | 清理同步历史 |
| `--keep-days <n>` | 清理时保留最近 N 天的记录 |

---

## 🤖 AI Agent 支持

ForkSync 集成了四款 AI 编码助手，用于自动解决合并冲突：

| Agent | 可执行文件 | 关键参数 |
|-------|-----------|---------|
| **Claude Code** | `claude` | `--print --dangerously-skip-permissions` |
| **OpenCode** | `opencode` | `run --session` |
| **Droid** | `droid` | `exec --auto high` |
| **Codex** | `codex` | `--dangerously-bypass-approvals-and-sandbox` |

Agent 通过系统 `PATH` 自动发现。你可以在配置文件中设置首选 Agent，或让 ForkSync 自动选择第一个可用的。

### 冲突解决策略

| 策略 | 说明 |
|------|------|
| `preserve_ours` | 保留本地修改，接受非冲突的上游变更 |
| `accept_theirs` | 优先采用上游变更，仅必要时保留本地修改 |
| `balanced` | 智能合并，尽量保留双方的变更 |
| `agent_resolve` | 委托 AI Agent 自动解决 |

---

## ⚙️ 配置

**配置文件路径：** `~/.forksync/config.yaml`

```yaml
sync:
  default_interval: "30m"        # 同步间隔
  sync_on_startup: true          # serve 启动时立即执行同步
  auto_launch: false             # 登录时自动启动

agent:
  preferred: ""                  # 首选 Agent（如 "claude"）
  priority:                      # Agent 优先级顺序
    - claude
    - opencode
    - droid
    - codex
  timeout: "10m"                 # Agent 解决超时时间
  conflict_strategy: "preserve_ours"  # preserve_ours | accept_theirs | balanced | agent_resolve
  confirm_before_commit: true    # 提交前展示 diff
  session_ttl: "24h"             # Agent 会话过期时间

github:
  token: ""                      # GitHub 个人访问令牌

notification:
  enabled: true

proxy:
  enabled: false
  url: ""
```

**数据文件：**

| 文件 | 用途 |
|------|------|
| `~/.forksync/config.yaml` | 用户配置 |
| `~/.forksync/repos.json` | 受管仓库列表 |
| `~/.forksync/sessions/<id>.json` | Agent 会话记录 |
| `~/.forksync/db/forksync.db` | SQLite 同步历史数据库 |
| `~/.forksync/logs/sync-*.log` | 按日轮转的日志文件 |

---

## 🏗️ 项目结构

```
forksync/
├── engine/                      # Go CLI 引擎
│   ├── cmd/                     # Cobra 命令（add, sync, resolve, config, history 等）
│   ├── internal/
│   │   ├── agent/               # AI Agent 适配器 + 注册表
│   │   │   ├── session/         # 会话管理器 + 持久化存储
│   │   │   ├── claude.go        # Claude Code 适配器
│   │   │   ├── opencode.go      # OpenCode 适配器
│   │   │   ├── droid.go         # Droid 适配器
│   │   │   └── codex.go         # Codex 适配器
│   │   ├── config/              # 基于 Viper 的配置管理
│   │   ├── conflict/            # 冲突检测
│   │   ├── git/                 # Git 操作（go-git 库 + CLI 回退）
│   │   ├── github/              # GitHub API 客户端
│   │   ├── history/             # 基于 SQLite 的同步历史存储
│   │   ├── logger/              # 基于文件的日志，按日轮转
│   │   ├── notify/              # macOS 系统通知
│   │   ├── repo/                # 仓库 JSON 存储
│   │   ├── scheduler/           # 后台同步调度器
│   │   └── sync/                # 同步管线
│   └── pkg/types/               # 共享类型（Repo, SyncResult 等）
│
├── app/                         # Electron 桌面应用
│   ├── src/
│   │   ├── main/                # Electron 主进程
│   │   │   ├── index.ts         # 窗口创建
│   │   │   ├── engine.ts        # EngineClient（启动 Go 二进制）
│   │   │   ├── ipc.ts           # IPC 处理器注册
│   │   │   ├── i18n.ts          # 国际化辅助
│   │   │   ├── ide.ts           # IDE 检测与管理（VSCode、Cursor、Trae）
│   │   │   └── notify.ts        # 系统通知（支持点击跳转）
│   │   ├── preload/             # 上下文桥接（window.api）
│   │   └── renderer/            # React UI
│   │       ├── src/
│   │       │   ├── pages/       # 仪表盘、仓库、冲突、历史、设置
│   │       │   ├── components/  # UI 组件（对话框、徽章等）
│   │       │   ├── contexts/    # React Context 状态管理
│   │       │   ├── hooks/       # 自定义 Hooks（useTheme）
│   │       │   └── lib/         # API 封装 + 工具函数
│   │       └── App.tsx          # 根组件 + 路由
│   └── electron-builder.yml     # 打包配置
│
├── build/
│   └── build.sh                 # 统一构建脚本
│
└── docs/                        # 文档
```

---

## 🧪 测试

Go 引擎包含完善的测试覆盖：

```bash
cd engine
go test ./... -v
```

**146 个测试**，分布在 15 个测试文件中，覆盖：
- 仓库存储 CRUD
- 同步管线
- Agent 适配器与 Provider 接口
- 会话管理器与会话存储
- Agent 注册表与发现
- 配置加载与保存
- GitHub API 客户端
- 类型序列化
- Git 操作
- 冲突检测
- 同步历史存储与清理
- 日志记录与日志轮转

---

## 🛠️ 开发

想参与开发？请阅读 [开发指南](./docs/DEVELOPMENT.md)，包含环境搭建、架构说明和开发工作流。

---

## 📝 许可证

本项目基于 MIT 许可证开源。
