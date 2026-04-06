# ForkSync 实现设计文档

**日期**: 2026-04-06
**状态**: Approved
**基于**: [2026-04-06-forksync-design.md](./2026-04-06-forksync-design.md)（产品需求文档）

---

## 设计决策摘要

基于原始需求文档，确认以下实现决策：

| 决策项 | 选择 |
|--------|------|
| 开发路径 | Go 引擎 + Electron UI 并行开发（方案 A：契约先行） |
| Git 操作实现 | 优先 go-git 库，命令行做 fallback |
| AI 适配器 MVP | 先实现 OpenAI 适配器（支持自定义 base_url），接口预留扩展 |
| UI 视觉风格 | 开发者工具风（类似 VS Code / GitHub Desktop，深色主题，信息密度高） |

---

## 1. JSON 契约层（Go 与 Electron 的桥梁）

Go 引擎和 Electron UI 通过 spawn + JSON 通信。所有 CLI 命令支持 `--json` flag 切换输出格式。

### 通用响应格式

```typescript
interface ApiResponse<T> {
  success: boolean;
  data: T;
  error: string;  // 仅在 success=false 时有值
}
```

### 各命令契约

#### `forksync status --json`

```typescript
interface StatusData {
  repos: Repo[];
}

interface Repo {
  id: string;
  name: string;
  path: string;
  origin: string;
  upstream: string;
  branch: string;
  autoSync: boolean;
  syncInterval: string;
  conflictStrategy: string;
  lastSync: string;       // ISO 8601
  status: "synced" | "syncing" | "conflict" | "error" | "unconfigured";
  aheadBy: number;        // 领先 upstream 多少 commit
  behindBy: number;       // 落后 upstream 多少 commit
  errorMessage?: string;
}
```

#### `forksync sync <repo> --json` / `forksync sync --all --json`

```typescript
interface SyncResult {
  repoId: string;
  repoName: string;
  status: "synced" | "conflict" | "up_to_date" | "error";
  commitsPulled: number;
  conflictFiles?: string[];
  errorMessage?: string;
}

interface SyncData {
  results: SyncResult[];
}
```

#### `forksync scan <dir> --json`

```typescript
interface ScanData {
  repos: ScannedRepo[];
}

interface ScannedRepo {
  path: string;
  name: string;
  origin: string;
  isFork: boolean;
  suggestedUpstream?: string;
}
```

#### `forksync add <path> --upstream <url> --json`

```typescript
interface AddData {
  repo: Repo;  // 同 status 中的 Repo
}
```

#### `forksync resolve <repo> --json`

```typescript
interface ResolveData {
  repoId: string;
  conflicts: ConflictFile[];
}

interface ConflictFile {
  path: string;
  oursContent: string;
  theirsContent: string;
  mergedContent?: string;  // AI 建议的合并结果
  aiExplanation?: string;
}
```

#### `forksync resolve <repo> --accept <filepath> --content <content> --json`

```typescript
interface AcceptData {
  repoId: string;
  file: string;
  resolved: boolean;
}
```

#### `forksync resolve <repo> --done --json`

```typescript
interface DoneData {
  repoId: string;
  allResolved: boolean;
  remainingConflicts?: string[];
}
```

### IPC 通信机制

Electron 侧封装 Go 二进制调用：

```typescript
async function callEngine(args: string[]): Promise<ApiResponse<any>> {
  const binary = path.join(app.getPath('exe'), '..', 'Resources', 'forksync');
  // dev 模式下用 go run ./engine/...
  const proc = spawn(binary, args);
  // 收集 stdout，返回 JSON
}
```

Go 侧：所有命令统一通过 `--json` flag 切换输出格式，不加 `--json` 则输出人类可读的彩色终端文本。

---

## 2. Go 引擎模块划分与实现顺序

### 模块依赖图（自底向上）

```
cmd/ (CLI 入口)
  ├── internal/config/      ← 配置读写
  ├── internal/repo/        ← 仓库管理（依赖 config）
  ├── internal/git/         ← Git 操作（依赖 repo）
  ├── internal/github/      ← GitHub API（识别 fork 关系）
  ├── internal/sync/        ← 同步编排（依赖 git, repo）
  ├── internal/conflict/    ← 冲突检测（依赖 git）
  ├── internal/ai/          ← AI 适配器（依赖 conflict）
  ├── internal/notify/      ← macOS 通知
  └── internal/scheduler/   ← 定时调度（依赖 sync）
```

### 实现阶段

#### 阶段 1：基础层

| 模块 | 职责 | 关键实现 |
|------|------|----------|
| `config/` | 读写 `~/.forksync/config.yaml`，提供全局配置结构体 | viper 加载，环境变量覆盖 |
| `repo/` | 管理 `repos.json`，CRUD 仓库条目 | JSON 文件读写，UUID 生成 |
| `git/` | go-git 封装：open/fetch/merge/diff/log/status | 优先 go-git，fallback exec("git") |

#### 阶段 2：扫描与状态

| 模块 | 职责 |
|------|------|
| `github/` | 调用 GitHub API 检测 fork 关系，获取 upstream URL |
| `cmd/scan` | 扫描目录 → 检测 git 仓库 → 识别 fork |
| `cmd/add` | 添加仓库到管理列表 |
| `cmd/status` | 查询所有仓库状态，输出表格/JSON |

#### 阶段 3：同步与冲突

| 模块 | 职责 |
|------|------|
| `sync/` | 同步编排：fetch → compare → merge → handle conflict |
| `conflict/` | 检测冲突文件，提取 ours/theirs 内容 |
| `ai/` | OpenAI 适配器（MVP），`AIProvider` 接口预留扩展 |
| `cmd/sync` | 单个/全部同步 |
| `cmd/resolve` | 交互式冲突解决 |

#### 阶段 4：调度与通知

| 模块 | 职责 |
|------|------|
| `scheduler/` | 基于仓库配置的 syncInterval 定时触发同步 |
| `notify/` | macOS 系统通知（osascript 调用原生通知中心） |

### 对外接口原则

- 每个模块只暴露一个 interface（或结构体），内部实现不外泄
- 错误用 Go 标准的 `error`，不 panic
- 所有涉及文件系统/git 的操作接受 `context.Context`，支持取消
- `git/` 模块内部封装 go-git fallback 逻辑，调用方不需要知道用的是 go-git 还是命令行

---

## 3. Electron UI 架构

### 项目结构

```
app/
├── main.ts                  # Electron main process（窗口管理、Go 二进制调用）
├── preload.ts               # contextBridge 暴露 IPC API
├── renderer/                # React 应用
│   ├── src/
│   │   ├── main.tsx         # React 入口
│   │   ├── App.tsx          # 路由配置
│   │   ├── lib/
│   │   │   ├── engine.ts    # Go 引擎调用封装（spawn + JSON 解析）
│   │   │   ├── ipc.ts       # Electron IPC 类型定义
│   │   │   └── utils.ts     # 工具函数
│   │   ├── hooks/
│   │   │   ├── useRepos.ts  # 仓库列表状态管理
│   │   │   ├── useSync.ts   # 同步操作
│   │   │   └── useEngine.ts # 引擎连接状态
│   │   ├── components/
│   │   │   ├── ui/          # shadcn/ui 基础组件
│   │   │   ├── Layout.tsx   # 侧边栏 + 主内容区
│   │   │   ├── RepoRow.tsx  # 仓库列表行
│   │   │   ├── StatusBadge.tsx
│   │   │   └── DiffViewer.tsx  # 冲突对比组件
│   │   ├── pages/
│   │   │   ├── Dashboard.tsx    # 总览
│   │   │   ├── Repos.tsx        # 仓库管理
│   │   │   ├── Conflicts.tsx    # 冲突解决
│   │   │   └── Settings.tsx     # 设置
│   │   └── styles/
│   │       └── globals.css      # Tailwind 入口
│   ├── index.html
│   ├── package.json
│   ├── vite.config.ts
│   └── tsconfig.json
├── electron-builder.yml
└── dev-app-update.yml
```

### 核心架构决策

**状态管理**：React Context + useReducer。两个核心 Context：`RepoContext`（仓库列表和状态）、`SettingsContext`（配置）。不引入 Redux 等外部状态库。

**路由**：React Router。
- `/` → Dashboard
- `/repos` → Repos
- `/conflicts/:repoId` → Conflicts
- `/settings` → Settings

**引擎通信**：
```
React 组件
  → 调用 preload 暴露的 API（如 window.api.syncRepo(id)）
    → preload 通过 IPC 发到 main process
      → main process spawn Go 二进制
        → 收集 stdout JSON 返回
```

main process 侧封装 `EngineClient` 类：

```typescript
class EngineClient {
  async status(): Promise<ApiResponse<StatusData>>;
  async sync(repoName?: string): Promise<ApiResponse<SyncData>>;
  async scan(dir: string): Promise<ApiResponse<ScanData>>;
  async add(path: string, upstream: string): Promise<ApiResponse<AddData>>;
  async resolve(repoName: string): Promise<ApiResponse<ResolveData>>;
  async acceptResolve(repoName: string, file: string, content: string): Promise<ApiResponse<AcceptData>>;
  async doneResolve(repoName: string): Promise<ApiResponse<DoneData>>;
}
```

**开发模式 vs 生产模式**：
- **开发模式**：`main.ts` 用 `go run ./engine/... args` 调用 Go 引擎，实时反映 Go 代码修改
- **生产模式**：`main.ts` 用预编译的 `forksync` 二进制（打包在 Electron resources 中）

**视觉风格**：
- 深色主题为默认（可通过系统偏好自动切换）
- 左侧固定侧边栏（图标 + 文字），主内容区占据剩余空间
- 组件间距紧凑，信息密度高，类似 GitHub Desktop 布局
- 状态用颜色编码：绿=已同步、黄=同步中、红=冲突、灰=未配置

---

## 4. 同步流程与 AI 冲突解决

### 同步状态机

```
Repo.status 变迁：
  unconfigured → syncing → synced
                         → conflict → syncing → synced
                         → error → syncing → synced
```

注：`conflict` 状态在用户手动解决后或 AI 解决失败后等待用户介入期间保持不变，
直到用户完成解决操作后再次触发 sync。

### 同步操作核心逻辑

```
1. fetch upstream
   - go-git: repo.Fetch(&git.FetchOptions{RemoteName: "upstream"})
   - fallback: exec("git", "fetch", "upstream")
   - 失败 → 返回 error 状态

2. 计算差异
   - go-git: 比较 upstream/<branch> 和 local/<branch> 的 commit log
   - behindBy == 0 → 返回 up_to_date
   - behindBy > 0 → 继续 merge

3. 执行 merge
   - go-git: repo.Merge(&git.MergeOptions{...})
   - fallback: exec("git", "merge", "upstream/<branch>")
   - 无冲突 → 更新 lastSync，返回 synced
   - 有冲突 → 进入冲突处理

4. 冲突处理（依赖 conflictStrategy 配置）
   - "ai_resolve" → 调用 AI 处理
   - "manual" → 返回 conflict 状态，等待用户
```

### AI 冲突解决

**输入结构**：

```go
type ConflictRequest struct {
    FilePath       string // 冲突文件路径
    ConflictContent string // 包含 <<<<<<< ======= >>>>>>> 标记的原始内容
    UserDiff       string // 用户相对于 upstream 的完整 diff
    Language       string // 文件语言（从扩展名推断）
}
```

**Prompt 策略**：

```
System: 你是一个代码合并助手。你的任务是在保留用户个性化修改的同时，
合入上游的非冲突更新。如果无法确定某个冲突块的取舍，在输出中标记
[NEEDS_MANUAL_REVIEW]。

User:
文件: {FilePath}
语言: {Language}

用户的个性化修改（相对 upstream 的 diff）：
{UserDiff}

冲突文件内容：
{ConflictContent}

请输出完整的解决后文件内容，并在文件末尾附上修改说明。
```

**输出结构**：

```go
type ConflictResolution struct {
    MergedContent string // AI 返回的解决后内容
    Explanation   string // 修改说明
    NeedsReview   bool   // 是否包含 [NEEDS_MANUAL_REVIEW] 标记
}
```

**验证链**（AI 输出写入前必须全部通过）：

1. 冲突标记清除检查：`strings.Contains(merged, "<<<<<<<")` → 失败
2. 语法检查：根据语言调用对应检查器（Go: `go vet`，TS: `tsc --noEmit`，Python: `python -m py_compile`），失败则不写入
3. `git diff --check`：无空白错误

任一验证失败 → 回退为手动解决，通知用户。

### 并发安全

- 同一个仓库同一时间只允许一个 sync 操作（用 `sync.Mutex` 按 repo ID 加锁）
- 调度器触发同步前检查锁状态，避免重复执行
- `context.WithTimeout` 设置单次同步超时（默认 5 分钟）

---

## 5. 调度、通知与构建打包

### 定时调度器

基于 Go 内置 `time.Ticker` + 优先队列：

```go
type Scheduler struct {
    mu      sync.Mutex
    queue   []ScheduleEntry  // 按 nextRun 排序
    ticker  *time.Ticker
    syncFn  func(repoID string) error  // 注入同步函数
}

type ScheduleEntry struct {
    RepoID   string
    NextRun  time.Time
    Interval time.Duration
}
```

**行为**：
- 应用启动时加载所有 `autoSync=true` 的仓库，计算下次同步时间
- Ticker 每 1 分钟检查一次队列头部，到时间则触发同步并重置 NextRun
- 同步操作在独立 goroutine 中执行，不阻塞调度循环
- 调度器可通过 CLI 命令启停（供 Electron 控制）

### macOS 通知

**Go 侧**（CLI 模式）：

```go
func notify(title, body string) error {
    script := fmt.Sprintf(
        `display notification "%s" with title "%s"`,
        body, title,
    )
    return exec.Command("osascript", "-e", script).Run()
}
```

**Electron 侧**（桌面模式）：

```typescript
new Notification({ title, body, silent: false })
  .on('click', () => { /* 跳转到对应冲突面板 */ })
  .show();
```

**通知规则**：

| 事件 | 通知 | 点击行为 |
|------|------|----------|
| 同步成功（有新 commit） | 可选 | 跳转到该仓库详情 |
| 冲突需人工介入 | 必须 | 跳转到冲突解决面板 |
| AI 解决成功 | 可选 | 跳转到该仓库详情 |
| 同步失败 | 必须 | 跳转到该仓库详情 |

### 构建与打包

**Go 引擎构建**：

```bash
cd engine && CGO_ENABLED=0 go build -o ../build/forksync .
```

**Electron 打包**：

```bash
cd app && npm ci && npm run build && npx electron-builder --mac
```

**完整构建脚本 `build/build.sh`**：

```bash
#!/bin/bash
set -e

# 1. 编译 Go 引擎
echo "Building Go engine..."
cd engine
CGO_ENABLED=0 go build -o ../build/forksync .
cd ..

# 2. 安装前端依赖 + 构建
echo "Building Electron app..."
cd app
npm ci
npm run build

# 3. 打包
echo "Packaging..."
npx electron-builder --mac

echo "Done! DMG in app/dist/"
```

**开发环境**：
- Go 侧：`go run ./engine/ <command>` 直接运行
- Electron 侧：`npm run dev`（Vite dev server + Electron）
- 联调：Electron 开发模式检测 Go 二进制不存在时自动用 `go run`
