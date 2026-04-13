# 开发指南

本文档面向 ForkSync 的开发者，介绍如何搭建开发环境、项目架构和开发工作流。

---

## 目录

- [环境要求](#环境要求)
- [快速开始](#快速开始)
- [项目架构](#项目架构)
- [Go 引擎开发](#go-引擎开发)
- [Electron UI 开发](#electron-ui-开发)
- [Go 与 Electron 的通信](#go-与-electron-的通信)
- [添加新的 CLI 命令](#添加新的-cli-命令)
- [添加新的 UI 页面](#添加新的-ui-页面)
- [添加新的 AI Agent 适配器](#添加新的-ai-agent-适配器)
- [构建与打包](#构建与打包)
- [测试](#测试)
- [常见问题](#常见问题)

---

## 环境要求

| 工具 | 版本 | 用途 |
|------|------|------|
| Go | 1.22+ | Go 引擎编译 |
| Node.js | 18+ | Electron 开发 |
| npm | 9+ | 包管理 |
| Git | 2.0+ | 版本控制 |

可选（AI Agent 冲突解决功能需要）：

| Agent | Binary | 安装方式 |
|-------|--------|---------|
| Claude Code | `claude` | `npm install -g @anthropic-ai/claude-code` |
| OpenCode | `opencode` | `go install github.com/opencode-ai/opencode@latest` |
| Droid | `droid` | 参见官方文档 |
| Codex | `codex` | `npm install -g @openai/codex` |

---

## 快速开始

```bash
# 1. 克隆仓库
git clone https://github.com/loongxjin/forksync.git
cd forksync

# 2. 安装 Electron 依赖
cd app && npm install && cd ..

# 3. 启动开发模式（同时启动 Go 引擎 + Electron 热重载）
cd app && npm run dev
```

开发模式下，Electron 主进程会自动使用 `go run ./engine/...` 调用 Go 引擎，无需手动编译 Go 二进制。

---

## 项目架构

```
forksync/
├── engine/                          # Go CLI 引擎
│   ├── main.go                      # 程序入口
│   ├── build.sh                     # Go 引擎构建脚本
│   ├── cmd/                         # Cobra CLI 命令
│   ├── pkg/types/                   # 共享类型（JSON 契约）
│   └── internal/                    # 内部包
│       ├── agent/                   # AI Agent 适配器
│       │   └── session/             # 会话管理
│       ├── config/                  # 配置管理 (viper)
│       ├── conflict/                # 冲突检测
│       ├── git/                     # Git 操作
│       ├── github/                  # GitHub API
│       ├── notify/                  # 系统通知
│       ├── repo/                    # 仓库存储
│       ├── scheduler/               # 定时调度
│       └── sync/                    # 同步管线
│
├── app/                             # Electron 桌面应用
│   ├── electron.vite.config.ts      # electron-vite 配置
│   ├── electron-builder.yml         # 打包配置
│   ├── package.json                 # 依赖和脚本
│   ├── tailwind.config.js           # Tailwind CSS 配置
│   ├── tsconfig.json                # TypeScript 项目引用
│   ├── tsconfig.node.json           # 主进程 + preload TS 配置
│   ├── tsconfig.web.json            # 渲染进程 TS 配置
│   └── src/
│       ├── main/                    # Electron 主进程
│       │   ├── index.ts             # 窗口创建和生命周期
│       │   ├── engine.ts            # EngineClient（Go 引擎通信）
│       │   └── ipc.ts              # IPC 处理器注册
│       ├── preload/                 # 预加载脚本
│       │   └── index.ts            # contextBridge 暴露 window.api
│       └── renderer/                # React 渲染进程
│           ├── index.html           # HTML 入口
│           └── src/
│               ├── main.tsx         # React 入口
│               ├── App.tsx          # 根组件 + 路由
│               ├── types/           # TypeScript 类型定义
│               │   └── engine.ts    # Go 引擎 JSON 类型映射
│               ├── lib/             # 工具函数
│               │   └── api.ts       # 渲染进程 API 封装
│               ├── hooks/           # 自定义 Hooks
│               ├── contexts/        # React Context 状态管理
│               ├── pages/           # 页面组件
│               ├── components/      # 通用组件
│               │   └── ui/          # shadcn/ui 基础组件
│               └── globals.css      # 全局样式 + CSS 变量
│
├── build/
│   └── build.sh                     # 统一构建脚本
│
└── docs/                            # 文档
```

---

## Go 引擎开发

### 目录结构

| 目录 | 职责 |
|------|------|
| `cmd/` | Cobra CLI 命令，每个文件对应一个子命令 |
| `pkg/types/` | 共享类型定义，Go 和 TypeScript 的 JSON 契约 |
| `internal/config/` | 基于 viper 的配置管理 |
| `internal/repo/` | 仓库 JSON 持久化存储 |
| `internal/git/` | Git 操作（go-git 库 + CLI 回退） |
| `internal/github/` | GitHub API 客户端（fork 检测） |
| `internal/sync/` | 同步编排（fetch → merge → 冲突检测 → 通知） |
| `internal/conflict/` | 冲突文件检测 |
| `internal/agent/` | AI Agent 注册表和适配器 |
| `internal/agent/session/` | Agent 会话管理（创建/恢复/过期清理） |
| `internal/notify/` | macOS 系统通知 |
| `internal/scheduler/` | 定时同步调度器 |

### JSON 输出模式

所有 CLI 命令通过 `--json` 标志输出结构化 JSON。这是 Electron UI 通信的基础：

```go
// cmd/root.go 中的通用 JSON 输出函数
func outputJSON[T any](data T, err error) {
    resp := types.ApiResponse[T]{}
    if err != nil {
        resp.Success = false
        resp.Error = err.Error()
    } else {
        resp.Success = true
        resp.Data = data
    }
    json.NewEncoder(os.Stdout).Encode(resp)
}
```

格式：
```json
{
  "success": true,
  "data": { ... },
  "error": ""
}
```

### 添加新的 CLI 命令

1. 在 `engine/cmd/` 下创建新文件，如 `engine/cmd/mycommand.go`
2. 使用 `cobra.Command` 定义命令
3. 在 `init()` 中注册到 `rootCmd`
4. 实现业务逻辑，使用 `outputJSON()` 或 `outputText()` 输出
5. 在 `engine/pkg/types/` 中定义对应的 Data 类型
6. 运行测试

### 运行与测试

```bash
cd engine

# 运行
go run . status --json

# 测试
go test ./... -v

# 测试特定包
go test ./internal/sync/ -v -run TestSyncAll

# 构建二进制
bash build.sh
# 输出: bins/forksync
```

---

## Electron UI 开发

### 技术栈

| 技术 | 版本 | 用途 |
|------|------|------|
| electron-vite | 2.x | 构建工具（3 bundle：main + preload + renderer） |
| React | 18 | UI 框架 |
| TypeScript | 5.5 | 类型安全 |
| Tailwind CSS | 3.4 | 样式 |
| shadcn/ui | 手动创建 | UI 组件库 |
| Radix UI | - | 无障碍基础组件 |
| react-router-dom | 6.x | 路由（HashRouter） |
| lucide-react | - | 图标 |

### 三进程架构

Electron 应用包含三个独立构建的 bundle：

```
1. Main Process  (src/main/)
   ├── Node.js 环境
   ├── 创建 BrowserWindow
   ├── 注册 IPC 处理器
   └── 通过 EngineClient 调用 Go 二进制

2. Preload (src/preload/)
   ├── 隔离的桥接层
   └── 通过 contextBridge 暴露 window.api

3. Renderer (src/renderer/)
   ├── 浏览器环境（Chromium）
   ├── React UI
   └── 通过 window.api 调用引擎
```

### 开发命令

```bash
cd app

# 安装依赖
npm install

# 开发模式（热重载 + Go 引擎自动 go run）
npm run dev

# 构建
npm run build

# 预览构建产物
npm run preview
```

### UI 组件规范

- **shadcn/ui 组件** 位于 `src/renderer/src/components/ui/`，手动创建（非 CLI 安装）
- 已有组件：`button`、`card`、`badge`、`input`、`label`、`separator`
- 新增组件遵循 shadcn/ui 风格，使用 `class-variance-authority` + `tailwind-merge`
- CSS 变量定义在 `globals.css`，支持 dark mode（`darkMode: 'class'`）

### 路由结构

| 路由 | 页面 | 文件 |
|------|------|------|
| `/` | 仪表盘 | `pages/Dashboard.tsx` |
| `/repos` | 仓库管理 | `pages/Repos.tsx` |
| `/conflicts` | 冲突列表 | `pages/Conflicts.tsx` |
| `/conflicts/:repoId` | 冲突详情 | `pages/ConflictDetail.tsx` |
| `/settings` | 设置 | `pages/Settings.tsx` |

使用 `HashRouter`（非 `BrowserRouter`），因为 Electron 加载本地文件。

### 状态管理

使用 React Context + useReducer，三个 Context：

| Context | 文件 | 状态 |
|---------|------|------|
| `RepoContext` | `contexts/RepoContext.tsx` | 仓库列表、加载状态、同步/添加/删除操作 |
| `AgentContext` | `contexts/AgentContext.tsx` | Agent 列表、会话、冲突解决操作 |
| `SettingsContext` | `contexts/SettingsContext.tsx` | 主题偏好（dark/light/system） |

---

## Go 与 Electron 的通信

### 通信流程

```
React 组件
  → window.api.syncAll()           // 渲染进程
    → ipcRenderer.invoke('engine:syncAll')  // preload 桥接
      → ipcMain handler            // 主进程
        → engineClient.syncAll()   // EngineClient
          → spawn('forksync', ['sync', '--all', '--json'])  // 子进程
            → Go CLI 输出 JSON     // Go 引擎
          ← 解析 JSON 响应
        ← ApiResponse<SyncData>
      ← 返回给渲染进程
    ← 更新 React 状态
  ← UI 更新
```

### IPC 通道列表

| 通道 | EngineClient 方法 | Go 命令 |
|------|-------------------|---------|
| `engine:status` | `status()` | `forksync status --json` |
| `engine:syncAll` | `syncAll()` | `forksync sync --all --json` |
| `engine:syncRepo` | `syncRepo(name)` | `forksync sync <name> --json` |
| `engine:scan` | `scan(dir)` | `forksync scan <dir> --json` |
| `engine:add` | `add(path, upstream?)` | `forksync add <path> --json` |
| `engine:remove` | `remove(name)` | `forksync remove <name> --json` |
| `engine:resolve` | `resolve(name, opts?)` | `forksync resolve <name> --json` |
| `engine:resolveAccept` | `resolveAccept(name)` | `forksync resolve <name> --accept --json` |
| `engine:resolveReject` | `resolveReject(name)` | `forksync resolve <name> --reject --json` |
| `engine:agentList` | `agentList()` | `forksync agent list --json` |
| `engine:agentSessions` | `agentSessions()` | `forksync agent sessions --json` |
| `engine:agentCleanup` | `agentCleanup()` | `forksync agent cleanup --json` |

### TypeScript 类型映射

`app/src/renderer/src/types/engine.ts` 定义了所有与 Go 引擎对应的 TypeScript 类型：

```typescript
// Go: types.ApiResponse[T]  →  TypeScript: ApiResponse<T>
interface ApiResponse<T> {
  success: boolean
  data: T
  error: string
}

// Go: types.Repo  →  TypeScript: Repo
interface Repo {
  id: string
  name: string
  path: string
  branch: string
  upstream: string
  status: RepoStatus
  ahead: number
  behind: number
  // ...
}
```

修改 Go 类型时，必须同步更新 TypeScript 定义。

### 开发模式 vs 生产模式

`EngineClient` 自动检测运行环境：

```typescript
// 开发模式：使用 go run（无需预编译）
const cmd = 'go'
const args = ['run', './engine/...', '--json', ...commandArgs]

// 生产模式：使用打包的二进制
const cmd = join(process.resourcesPath, 'forksync')
const args = ['--json', ...commandArgs]
```

判断逻辑：`app.isPackaged` — 开发时为 `false`，打包后为 `true`。

---

## 添加新的 CLI 命令

完整示例：添加一个 `forksync log` 命令。

### 1. Go 引擎

```go
// engine/cmd/log.go
package cmd

import "github.com/spf13/cobra"

var logCmd = &cobra.Command{
    Use:   "log",
    Short: "Show sync history",
}

func init() {
    rootCmd.AddCommand(logCmd)
}
```

### 2. 定义类型

```go
// engine/pkg/types/log.go
package types

type LogEntry struct {
    RepoName  string `json:"repoName"`
    Timestamp string `json:"timestamp"`
    Action    string `json:"action"`
    Result    string `json:"result"`
}

type LogData struct {
    Entries []LogEntry `json:"entries"`
}
```

### 3. TypeScript 类型

```typescript
// app/src/renderer/src/types/engine.ts — 追加
export interface LogEntry {
  repoName: string
  timestamp: string
  action: string
  result: string
}

export interface LogData {
  entries: LogEntry[]
}
```

### 4. EngineClient 方法

```typescript
// app/src/main/engine.ts — 追加
async getLog(): Promise<ApiResponse<LogData>> {
  return this.exec<LogData>(['log'])
}
```

### 5. IPC 通道

```typescript
// app/src/main/ipc.ts — 追加
ipcMain.handle('engine:log', (_e) => engineClient.getLog())
```

### 6. Preload 桥接

```typescript
// app/src/preload/index.ts — 追加
log: () => ipcRenderer.invoke('engine:log'),
```

### 7. 渲染进程 API

```typescript
// app/src/renderer/src/lib/api.ts — 追加
export const getLog = () => window.api.log()
```

---

## 添加新的 UI 页面

### 1. 创建页面组件

```tsx
// app/src/renderer/src/pages/LogPage.tsx
import { useEffect, useState } from 'react'
import { getLog } from '../lib/api'
import type { LogEntry } from '../types/engine'

export default function LogPage() {
  const [entries, setEntries] = useState<LogEntry[]>([])

  useEffect(() => {
    getLog().then(res => {
      if (res.success) setEntries(res.data.entries)
    })
  }, [])

  return (
    <div>
      <h1 className="text-2xl font-bold">Sync Log</h1>
      {/* 渲染日志列表 */}
    </div>
  )
}
```

### 2. 注册路由

```tsx
// app/src/renderer/src/App.tsx — 在 Routes 中添加
<Route path="/log" element={<LogPage />} />
```

### 3. 添加导航链接

```tsx
// app/src/renderer/src/components/Sidebar.tsx — 在 navLinks 中添加
{ path: '/log', label: 'Sync Log', icon: FileText }
```

---

## 添加新的 AI Agent 适配器

### 1. 创建适配器

```go
// engine/internal/agent/myagent.go
package agent

import "context"

type MyAgent struct{}

func (a *MyAgent) Name() string { return "myagent" }

func (a *MyAgent) IsAvailable() bool {
    _, err := exec.LookPath("myagent")
    return err == nil
}

func (a *MyAgent) StartSession(ctx context.Context, workDir string) (string, error) {
    // 启动 agent 会话，返回 session ID
}

func (a *MyAgent) ResolveConflicts(ctx context.Context, sessionID string, prompt string) (string, error) {
    // 调用 agent 解决冲突，返回输出
}

func (a *MyAgent) EndSession(ctx context.Context, sessionID string) error {
    // 结束会话
}
```

### 2. 注册到 Registry

```go
// engine/internal/agent/registry.go — 在 NewRegistry() 中添加
r.Register(&MyAgent{})
```

### 3. 更新配置

```yaml
# agent.priority 中添加新 agent 名称
agent:
  priority:
    - claude
    - myagent  # 新增
```

---

## 构建与打包

### 开发模式

```bash
cd app && npm run dev
```

- 自动热重载（renderer 进程）
- Go 引擎通过 `go run ./engine/...` 实时调用
- 无需手动编译 Go 二进制

### 生产构建

```bash
# 完整构建（Go 引擎 + Electron 打包）
./build/build.sh

# 或分步执行：
cd engine && CGO_ENABLED=0 go build -o ../build/forksync . && cd ..
cd app && npm ci && npm run build && npx electron-builder
```

构建产物在 `app/dist/`。

### Go 引擎单独构建

```bash
cd engine
bash build.sh
# 输出: bins/forksync (darwin/arm64)
```

### electron-builder 配置

`app/electron-builder.yml` 关键配置：

```yaml
appId: com.forksync.app
extraResources:
  - from: ../build/forksync    # Go 二进制
    to: forksync
mac:
  target: dmg
linux:
  target: AppImage
win:
  target: nsis
```

---

## 测试

### Go 引擎测试

```bash
cd engine

# 运行所有测试
go test ./... -v

# 运行特定包
go test ./internal/sync/ -v

# 运行单个测试
go test ./internal/agent/ -v -run TestClaudeAdapter

# 查看覆盖率
go test ./... -coverprofile=coverage.out
go tool cover -html=coverage.out
```

**测试覆盖（122 个测试）：**

| 包 | 测试数 | 说明 |
|----|--------|------|
| `pkg/types` | 7 | 类型序列化 |
| `internal/config` | 7 | 配置加载/保存 |
| `internal/repo` | 24 | 仓库 CRUD |
| `internal/git` | 14 | Git 操作 |
| `internal/github` | 4 | GitHub API |
| `internal/conflict` | 5 | 冲突检测 |
| `internal/sync` | 18 | 同步管线 |
| `internal/agent` | 28 | Agent 适配器 + Provider + Registry |
| `internal/agent/session` | 15 | 会话管理 + 存储 |

### 手动测试 Electron UI

```bash
cd app && npm run dev
```

然后在 Electron 窗口中手动测试各功能。

---

## 常见问题

### Q: 开发模式下 Go 引擎调用失败？

检查当前工作目录是否为项目根目录。`go run ./engine/...` 需要在项目根目录执行。`npm run dev` 会从 `app/` 目录启动，electron-vite 会正确设置工作目录。

### Q: 如何调试 Go 引擎的 JSON 输出？

直接在终端运行命令：

```bash
cd engine && go run . status --json | jq .
```

### Q: 如何调试 Electron 主进程？

在 `electron-vite dev` 模式下，主进程的 `console.log` 会输出到终端（不是 DevTools）。

### Q: 如何打开 DevTools？

开发模式下 DevTools 自动打开。如果没有，在应用菜单中选择 View → Toggle Developer Tools。

### Q: Tailwind 类不生效？

检查 `tailwind.config.js` 的 `content` 路径是否包含你的新文件：

```js
content: ['./src/renderer/src/**/*.{ts,tsx}']
```

### Q: 添加新 shadcn/ui 组件？

本项目手动创建 UI 组件（不使用 shadcn CLI）。参考已有组件（如 `button.tsx`）的风格，手动创建到 `src/renderer/src/components/ui/` 目录。

### Q: 修改 Go 类型后 TypeScript 报错？

修改 `engine/pkg/types/` 中的 Go 类型后，必须同步更新 `app/src/renderer/src/types/engine.ts` 中的 TypeScript 类型。两者必须保持一致。

### Q: Go 引擎的 `serve` 命令输出格式不同？

`serve` 命令直接输出裸 JSON，不包裹在 `ApiResponse[T]` 中。EngineClient 的 `serve` 方法已处理此差异。

### Q: `remove` 命令的 JSON 双重包裹？

Go 引擎的 `remove` 命令会将结果双重包裹在 `ApiResponse` 中。EngineClient 使用 `unwrapDouble()` 方法处理此问题。这是已知的不一致，后续会修复。
