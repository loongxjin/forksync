# ForkSync — 自动同步上游 Fork 仓库工具

**日期**: 2026-04-06  
**状态**: Draft  
**作者**: loongxjin  
**定位**: 开源桌面应用（macOS）

---

## 问题

在 AI 编码时代，开发者经常 fork 开源项目到本地，通过 AI 添加个性化修改后使用。但为了跟随上游代码更新，需要手动同步上游仓库、解决冲突，这个过程繁琐且容易遗忘。

## 解决方案

**ForkSync** 是一个 macOS 桌面应用（Electron + Go），自动检测 fork 仓库的上游更新，智能同步代码，并通过 AI 自动解决合并冲突。需要人工介入时通过系统通知提醒用户。

---

## 架构

```
┌─────────────────────────────────────────────────┐
│              Electron UI (TypeScript)            │
│  Dashboard | Repo List | Conflict Resolver       │
│  Settings  | AI Config | Notifications          │
├─────────────────────────────────────────────────┤
│              IPC (spawn Go binary, JSON I/O)     │
├─────────────────────────────────────────────────┤
│           Go Core Engine (嵌入式二进制)           │
│  Git Sync | Conflict Detector | AI Resolver     │
│  Repo Scan | Upstream Manager  | Scheduler      │
└─────────────────────────────────────────────────┘
```

**核心设计决策**：
- Go 引擎是独立可运行的 CLI，Electron 通过 spawn 调用，JSON 格式通信
- 引擎不依赖 UI，可在终端单独使用
- 前端是引擎的可视化界面

---

## Go 核心引擎

### 仓库配置模型

```json
{
  "id": "uuid",
  "name": "goex",
  "path": "/Users/chenjinlong/GoProjects/goex",
  "origin": "https://github.com/loongxjin/goex",
  "upstream": "https://github.com/nntaoli-project/goex",
  "branch": "main",
  "autoSync": true,
  "syncInterval": "30m",
  "conflictStrategy": "ai_resolve",
  "lastSync": "2026-04-06T10:00:00Z",
  "status": "synced"
}
```

### 同步流程

```
定时触发 / 手动触发
        │
        ▼
  fetch upstream ──► fetch 失败 ──► 记录错误，通知用户
        │
        ▼
  比较 upstream/<branch> 和 local/<branch>
        │
        ├── 无差异 ──► 跳过，等待下次调度
        │
        ▼
  执行 merge upstream/<branch>
        │
        ├── 无冲突 ──► 自动合入，记录日志
        │
        ▼
  检测到冲突
        │
        ├── AI 自动解决（如果已配置 AI 且策略为 ai_resolve）
        │     ├── 成功 ──► 自动合入，通知用户已解决
        │     └── 失败 ──► 通知用户手动介入
        │
        └── 无 AI 配置 ──► 通知用户手动解决
```

### AI 冲突解决

**适配器模式**，支持多个 AI 提供商：

```go
type AIProvider interface {
    ResolveConflicts(ctx context.Context, req ConflictRequest) (*ConflictResolution, error)
}
```

内置适配器：
- `OpenAIAdapter` — GPT-4o（支持自定义 `base_url`，兼容第三方 OpenAI API）
- `AnthropicAdapter` — Claude Sonnet
- `GeminiAdapter` — Google Gemini
- `OllamaAdapter` — 本地模型

**冲突解决 Prompt 策略**：
- **输入**：冲突文件的完整内容，包含 `<<<<<<<`、`=======`、`>>>>>>>` 冲突标记
- **上下文**：用户相对于上游的完整 diff（个性化修改列表）
- **指令**：保留用户的个性化修改，同时合入上游的非冲突新功能；如果无法确定取舍，标记为需要人工介入
- **输出**：解决后的文件内容 + 修改解释

**失败处理**：AI 返回的内容通过以下验证：
1. 去除所有冲突标记（`<<<<<<<`、`=======`、`>>>>>>>`），确认无残留
2. 对代码文件执行基础语法检查（Go/TS/Python/Rust 等通过对应 parser）
3. 执行 `git diff --check` 确认无空白错误
任何验证失败则回退为手动解决，不自动写入。

### CLI 接口

```bash
# 扫描目录下的 fork 仓库
forksync scan <directory>

# 添加仓库到管理列表
forksync add <repo-path> --upstream <upstream-url>

# 同步单个仓库
forksync sync <repo-name>

# 同步所有仓库
forksync sync --all

# 查看所有仓库状态
forksync status

# 交互式解决冲突
forksync resolve <repo-name>

# JSON 输出模式（供 Electron 调用）
forksync status --json
forksync sync --all --json
forksync sync <repo-name> --json
```

所有 `--json` 输出遵循统一格式：
```json
{
  "success": true,
  "data": { ... },
  "error": ""
}
```

---

## Electron UI

### 技术栈
- **Electron** + **React** + **TypeScript**
- **Vite** 构建
- **shadcn/ui** 组件库（基于 Radix UI + Tailwind CSS）

### 页面

#### 1. Dashboard（总览页）
- 三个状态卡片：同步成功数 / 冲突待处理数 / 同步中数量
- 最近活动时间线（同步记录、冲突事件）
- 快捷操作：一键同步全部

#### 2. Repo List（仓库管理页）
- 仓库列表，每行显示：名称、状态指示灯、上次同步时间、upstream 仓库
- 添加仓库：拖拽文件夹到窗口 或 点击"添加"选择目录
- 自动检测：扫描目录下的 git 仓库，通过 GitHub API 识别 fork 关系（对比 `parent` 字段），自动获取 upstream URL
- 状态指示灯：🟢 已同步 / 🟡 同步中 / 🔴 有冲突 / ⚪ 未配置 upstream
- 每行操作：立即同步、查看 diff、打开终端、在 Finder 中显示、移除

#### 3. Conflict Resolver（冲突解决面板）
- 左右分栏对比：用户版本 vs 上游版本，差异高亮显示
- AI 建议区域：显示 AI 合并后的代码预览和修改解释
- 三个操作按钮：接受 AI 建议 / 手动编辑（打开内置编辑器） / 跳过此文件
- 支持逐文件处理，显示进度（共 N 个冲突文件，已处理 M 个）

#### 4. Settings（设置页）
- 通用设置：同步间隔（全局默认）、启动时自动同步、开机自启动
- AI 配置：选择默认 AI 提供商、配置各提供商的 API Key 和模型
- 通知设置：启用/关闭 macOS 系统通知
- 代理设置：HTTP/SOCKS5 代理（用于 GitHub 和 AI API 访问）

### 系统通知

通过 macOS 原生通知中心（Electron `Notification` API）推送：
- 同步成功：可选显示
- 冲突需要人工介入：必须显示，包含仓库名和冲突文件数
- 通知点击后打开应用并跳转到对应冲突面板

---

## 数据存储

```
~/.forksync/
├── config.yaml          # 全局配置
├── repos.json           # 管理的仓库列表和状态
├── logs/                # 同步日志（按日期轮转）
│   └── sync-YYYY-MM-DD.log
└── db/                  # SQLite（仓库元数据、同步历史记录）
    └── forksync.db
```

**config.yaml 结构**：
```yaml
sync:
  default_interval: 30m
  sync_on_startup: true
  auto_launch: false

ai:
  default_provider: openai
  providers:
    openai:
      api_key: sk-xxx
      model: gpt-4o
      base_url: https://api.openai.com/v1
    anthropic:
      api_key: sk-ant-xxx
      model: claude-sonnet-4-20250514
    gemini:
      api_key: xxx
      model: gemini-2.5-pro
    ollama:
      base_url: http://localhost:11434
      model: codellama

notification:
  enabled: true
  on_conflict: true
  on_sync_success: false

proxy:
  enabled: false
  url: socks5://127.0.0.1:7890
```

---

## GitHub API 集成

用于自动识别 fork 关系和获取 upstream 信息：

- **检测 fork**：`GET /repos/{owner}/{repo}` → 检查 `source` 字段是否存在
- **获取 upstream URL**：从 `source.clone_url` 读取
- **认证**：使用 GitHub Personal Access Token（在设置中配置），提高 API 限流
- **支持平台**：初期仅 GitHub，后续可扩展 Gitee、GitLab

---

## 项目结构

```
forksync/
├── README.md
├── LICENSE                 # MIT
├── engine/                 # Go 核心引擎
│   ├── main.go             # 入口
│   ├── cmd/                # CLI 命令（cobra）
│   │   ├── root.go
│   │   ├── scan.go
│   │   ├── add.go
│   │   ├── sync.go
│   │   ├── status.go
│   │   └── resolve.go
│   ├── internal/
│   │   ├── git/            # Git 操作封装（go-git + 命令行）
│   │   ├── sync/           # 同步逻辑
│   │   ├── conflict/       # 冲突检测与解析
│   │   ├── ai/             # AI 适配器（多提供商）
│   │   ├── config/         # 配置管理
│   │   ├── repo/           # 仓库管理
│   │   ├── notify/         # 通知系统
│   │   ├── scheduler/      # 定时调度
│   │   └── github/         # GitHub API 集成
│   ├── go.mod
│   └── go.sum
├── app/                    # Electron 前端
│   ├── main.ts             # Electron main process
│   ├── preload.ts          # preload script
│   ├── renderer/           # React UI
│   │   ├── src/
│   │   │   ├── components/ # UI 组件
│   │   │   ├── pages/      # 页面（Dashboard, Repos, Conflicts, Settings）
│   │   │   ├── hooks/      # React hooks
│   │   │   ├── lib/        # 工具函数
│   │   │   └── styles/     # Tailwind CSS
│   │   └── index.html
│   ├── package.json
│   ├── vite.config.ts
│   └── electron-builder.yml
├── build/                  # 打包脚本
│   └── build.sh            # 编译 Go + 打包 Electron
└── docs/                   # 文档
    ├── getting-started.md
    └── ai-configuration.md
```

---

## 关键依赖

### Go 引擎
- `github.com/go-git/go-git/v5` — Git 操作
- `github.com/spf13/cobra` — CLI 框架
- `github.com/sashabaranov/go-openai` — OpenAI API 客户端
- `github.com/liushuangls/go-anthropic` — Anthropic API 客户端
- `modernc.org/sqlite` — SQLite 驱动（纯 Go，无 CGO）

### Electron 前端
- `electron` — 桌面框架
- `react` + `react-dom` — UI 框架
- `tailwindcss` + `@shadcn/ui` — 样式和组件
- `vite` — 构建工具
- `electron-builder` — 打包分发

---

## 非目标（YAGNI）

以下功能明确不在初期范围内：
- Windows / Linux 支持（仅 macOS）
- GitLab / Gitee 支持（仅 GitHub）
- 自动 push 到 origin（同步仅合入本地，不自动推送）
- 多用户 / 团队协作功能
- 浏览器版本
- 付费 / SaaS 模式
- Git 仓库浏览器（仅管理同步，不做完整的 Git GUI）
