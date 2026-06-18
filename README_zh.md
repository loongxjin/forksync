<div align="center">

# ForkSync

**自动同步 GitHub Fork 仓库 — AI 解决合并冲突。**

[English](./README.md) · **中文**

[![Go](https://img.shields.io/badge/Go-1.22+-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![Wails](https://img.shields.io/badge/Wails-2-DF0000?logo=wails&logoColor=white)](https://wails.io/)
[![React](https://img.shields.io/badge/React-18-61DAFB?logo=react&logoColor=black)](https://react.dev/)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](./LICENSE)

</div>

<p align="center">
  <img src="image/README/1776830486988.png" alt="ForkSync 桌面应用" width="720">
</p>

---

## 为什么需要 ForkSync？

维护 Fork 仓库是一件苦差事。上游作者不断发布更新，每次同步都可能遇到合并冲突。你通常面临：

- **忘记同步** — Fork 落后于上游，错过 Bug 修复和新功能
- **手动解决冲突** — 在 `<<<<<<<` 标记中翻来覆去读几个小时
- **放弃重新 Fork** — 丢失你自己的本地修改

**ForkSync 解决了这个问题。** 它自动同步你的 Fork 仓库，并在出现合并冲突时调用 AI 编码助手（Claude Code、OpenCode、Codex）自动解决 — 你再也不用手动处理冲突标记。

## 核心功能

| 功能 | 说明 |
|------|------|
| **自动同步** | 定时拉取并合并上游变更（可自定义间隔） |
| **AI 冲突解决** | 将合并冲突委托给 AI Agent，通过 git 历史感知的 prompt 做智能合并 |
| **工作流引导** | 步骤式工作流：fetch → merge → 检测冲突 → agent 解决 → 审核 → 提交 |
| **实时终端** | agent 运行过程中实时查看 stdout、工具调用和错误输出 |
| **桌面应用** | 精致的 Electron GUI — 仪表盘、工作流步骤、设置页 |
| **HTTP API** | REST + WebSocket 服务，适合程序化访问 |
| **目录扫描** | 递归扫描任意目录，自动发现并批量添加 Fork 仓库 |
| **同步历史** | 基于 SQLite 的历史记录，支持筛选、AI 摘要和清理 |
| **系统通知** | 桌面原生通知，同步成功/冲突/错误即时提醒 |
| **IDE 集成** | 一键在 VSCode、Cursor 或 Trae 中打开仓库 |
| **同步后命令** | 同步成功后自动执行自定义脚本（如 `pip install`、`npm build`） |
| **国际化** | 多语言界面支持（中文 / 英文） |
| **多 Agent 支持** | 可在 Claude Code 和 OpenCode 之间自由切换 |

---

## 安装

### 下载安装

根据你的平台下载最新版本：

| 平台 | 格式 | 链接 |
|------|------|------|
| macOS | `.dmg` | [Releases](https://github.com/loongxjin/forksync/releases) |
| Linux | `.AppImage` | [Releases](https://github.com/loongxjin/forksync/releases) |
| Windows | `.exe` (NSIS) | [Releases](https://github.com/loongxjin/forksync/releases) |

### 从源码构建

```bash
git clone https://github.com/loongxjin/forksync.git
cd forksync

# Wails 构建（单二进制文件, ~18MB）
make wails
# 输出: build/bin/
```

### 仅命令行

```bash
cd engine && go build -o forksync . && ./forksync --help
```

---

## 快速开始

### 1. 配置 GitHub Token（推荐）

```bash
mkdir -p ~/.forksync
```

编辑 `~/.forksync/config.yaml`：

```yaml
github:
  token: "ghp_你的token"
```

> Token 为可选项，但强烈建议配置 — 它可以启用通过 GitHub API 自动检测上游仓库。

### 2. 启动应用

```bash
# 开发模式（热重载）
make wails-dev

# 或构建后运行
make wails && open build/bin/forksync.app
```

Wails 应用将 Go 引擎直接嵌入 — 无需独立 server 进程、无需 HTTP 桥接。所有引擎操作均为原生 Go 函数调用。

---

## AI 冲突解决

这是 ForkSync 最核心的功能。当同步产生合并冲突时，ForkSync 可以自动委托 AI 编码助手处理：

```
┌─────────────┐    冲突发生     ┌───────────────┐    调用       ┌────────────────┐
│   上游变更    │ ──────────────▶ │  ForkSync     │ ────────────▶│  AI Agent      │
│              │                 │  检测到冲突    │              │  (Claude /     │
└─────────────┘                 └───────────────┘              │   OpenCode)    │
                                                                └───────┬────────┘
                                                                        │ 解决完成
                                                                        ▼
                                ┌───────────────┐              ┌────────────────┐
                                │  ForkSync     │ ◀───────────│  验证并暂存     │
                                │  提交变更      │   确认提交   │                │
                                └───────────────┘              └────────────────┘
```

**支持的 Agent：**

| Agent | 可执行文件 | 自动检测 |
|-------|-----------|:-------:|
| Claude Code | `claude` | ✅ |
| OpenCode | `opencode` | ✅ |
| Codex | `codex` | ✅ |

Agent 通过系统 `PATH` 自动发现。在配置文件中设置首选 Agent：

```yaml
agent:
  preferred: "claude"
```

**冲突解决策略：**

| 策略 | 配置项 | 行为 |
|------|--------|------|
| Agent 自动解决 | `conflict_strategy: agent_resolve` | 同步时自动调用 agent 解决冲突 |
| 手动解决 | `conflict_strategy: manual` | 同步暂停，用户在界面选择 agent 或手动解决 |
| 保留本地 | `resolve_strategy: preserve_ours` | agent 被告知保留本地修改，接受上游非冲突变更 |
| 接受上游 | `resolve_strategy: preserve_theirs` | agent 被告知优先采用上游变更 |
| 智能合并 | `resolve_strategy: balanced` | agent 被告知优雅整合双方修改 |

**确认模式：**

| 配置 | 行为 |
|------|------|
| `confirm_before_commit: true` | agent 解决后等待用户审核，手动选择接受或拒绝 |
| `confirm_before_commit: false` | agent 解决后自动提交 |

**同步后命令：** 在桌面应用的仓库设置对话框中按仓库配置（添加/编辑/删除每次成功同步后执行的 shell 命令）。

---

## 桌面应用

基于 **Wails v2** + **React** + **TypeScript** + **Tailwind CSS** + **shadcn/ui** 构建。

| 区域 | 说明 |
|------|------|
| **仪表盘** | 总览：仓库状态、最近同步活动 |
| **仓库列表** | 可展开的仓库卡片，显示工作流步骤或详情面板 |
| **工作流步骤** | 步骤式进度：fetch → merge → 检测冲突 → 解决策略 → agent 解决 → 审核 → 提交 |
| **Agent 终端** | agent 运行过程中实时流式查看输出内容 |
| **AI 摘要** | 解决完成后，在工作流中展示 agent 基于 git 历史的分析摘要 |
| **Diff 查看器** | 审核变更时的左右对照 diff 预览 |
| **同步历史** | 同步历史时间线，支持筛选、AI 摘要和清理 |
| **设置** | 通用设置、Agent 配置、同步后命令、IDE 偏好 |

**架构：**

```
┌────────────────────────────────────────────┐
│            Wails UI (React)                 │
│  仪表盘 · 仓库 · 工作流                      │
│  Agent 终端 · 历史 · 设置                   │
└────────────────┬───────────────────────────┘
                 │ Wails binding（直接 Go 调用）
┌────────────────▼───────────────────────────┐
│      Go 引擎（同进程，无 IPC）                │
│  App 结构体，33 个绑定方法                    │
│  内部:  sync · resolve · agent               │
│         history · scheduler · eventbus       │
│         ide · config · summarize             │
└────────────────────────────────────────────┘
```

Go 引擎**不是独立的 HTTP server** — 全部 33 个方法都是 Go 结构体方法，通过 Wails 自动生成的 TypeScript 绑定暴露给前端。无需 bearer token、端口发现、进程管理。流式输出使用 Wails Events 替代 WebSocket。

---

## 引擎 API

所有引擎操作均可通过 **Wails bindings**（React 前端直接 Go 调用）访问。同时提供独立 HTTP server 用于无头模式。详见 `engine/README.md` 获取 HTTP API 参考。

| 操作 | HTTP 路由 |
|---|---|
| 状态 | `GET /status` |
| 扫描 | `POST /scan` |
| 添加仓库 | `POST /repos` |
| 移除仓库 | `DELETE /repos/{name}` |
| 同步全部 | `POST /sync/all` |
| 同步单个 | `POST /sync/repos/{name}` |
| 解决冲突 | `POST /repos/{name}/resolve` |
| 流式解决 | `WS /stream/resolve/{name}` |
| Agent | `GET /agents` |
| 历史 | `GET /history?repo=&limit=` |
| 配置 | `GET /config` / `PUT /config` |
| 后置同步 | `GET/POST/DELETE /repos/{name}/post-sync` |
| 摘要 | `POST /repos/{name}/summarize` |


