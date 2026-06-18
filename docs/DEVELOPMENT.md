# 开发文档

本文档描述 ForkSync 的技术架构和开发流程。

## 技术栈

| 层 | 技术 | 说明 |
|---|---|---|
| 桌面框架 | Wails v2 | Go + 系统 webview，单二进制 |
| 前端 | React 18 + TypeScript + Vite | 在 `frontend/` 目录 |
| UI | Tailwind CSS + shadcn/ui | 组件库 |
| 后端 | Go 1.22 | 引擎逻辑在 `engine/core/` |
| 数据库 | SQLite (modernc) | 同步历史 |
| Git | go-git v5 | 纯 Go git 操作 |

## 项目结构

```
forksync/
├── main.go                 # Wails 入口（嵌入 frontend/dist）
├── app.go                  # 34 个 Wails 绑定方法（引擎能力）
├── ide.go                  # IDE 检测、目录对话框、自启动等绑定方法
├── wails.json              # Wails 配置
├── go.mod / go.work        # Go 模块（根 + engine 子模块）
├── frontend/               # React 前端
│   ├── src/
│   │   ├── App.tsx         # 路由入口
│   │   ├── components/     # UI 组件
│   │   ├── contexts/       # React Context（Repo/History/Agent/Settings）
│   │   ├── hooks/          # 自定义 Hooks
│   │   ├── lib/api.ts      # Wails 绑定包装层
│   │   ├── shared/types/   # 共享 TS 类型定义
│   │   └── wailsjs/        # Wails 自动生成的 Go→TS 绑定
│   ├── vite.config.ts
│   └── package.json
├── engine/                 # Go 引擎（独立子模块）
│   ├── main.go             # 独立 HTTP server 入口（无头模式）
│   ├── go.mod
│   ├── core/               # 引擎核心包
│   │   ├── agent/          # AI Agent 适配器（Claude/OpenCode/Codex）
│   │   ├── app/            # Deps 依赖容器、HTTP handlers、事件桥接
│   │   ├── config/         # 配置管理（viper + YAML）
│   │   ├── eventbus/       # 进程内事件总线
│   │   ├── git/            # git 操作抽象（go-git）
│   │   ├── github/         # GitHub API（upstream 检测）
│   │   ├── history/        # SQLite 历史存储
│   │   ├── logger/         # 结构化日志（slog）
│   │   ├── repo/           # 仓库存储（JSON 文件）
│   │   ├── resolve/        # 冲突解决核心（Resolver）
│   │   ├── scheduler/      # 定时同步调度器
│   │   ├── summarizer/     # AI 摘要生成
│   │   ├── sync/           # 同步流水线（Syncer + StatusRefresher）
│   │   └── workflow/       # 工作流状态机
│   └── pkg/types/          # 公共类型定义
└── build/                  # 打包脚本
    ├── dmg.sh              # macOS DMG 打包（拖拽布局）
    └── windows/installer.nsi  # Windows NSIS 安装脚本
```

## 架构

```
┌────────────────────────────────────────────┐
│            Wails UI (React)                 │
│  Dashboard · Repos · Workflow               │
│  Agent Terminal · History · Settings        │
└────────────────┬───────────────────────────┘
                 │ Wails binding（直接 Go 函数调用，无 HTTP/IPC）
┌────────────────▼───────────────────────────┐
│      Go Engine（同进程，34 个绑定方法）       │
│  app.go + ide.go                            │
│  内部: sync · resolve · agent · history      │
│        scheduler · eventbus · config         │
└────────────────────────────────────────────┘
```

Wails 桌面应用和引擎运行在**同一个进程**内。前端通过 Wails 自动生成的 TypeScript 绑定直接调用 Go 方法，不需要 HTTP server、端口发现、bearer token 或进程间通信。

### 流式通信

Agent 解决冲突的实时输出通过 **Wails Events** 推送：

| 事件名 | 方向 | 说明 |
|---|---|---|
| `resolve:tick` | Go → JS | Agent 有新输出，前端重新读磁盘日志 |
| `resolve:done` | Go → JS | 解决完成，携带 ResolveData |
| `resolve:error` | Go → JS | 解决失败，携带错误信息 |
| `engine:event` | Go → JS | 状态变更（repos_changed / history_changed）|

磁盘 NDJSON 日志是**唯一数据源**，Wails Events 只是通知前端去重新读磁盘文件。

## 开发

### 环境要求

| 工具 | 版本 | 用途 |
|---|---|---|
| Go | 1.22+ | 引擎 + Wails 后端 |
| Node.js | 18+ | 前端构建 |
| Wails CLI | v2.12+ | `go install github.com/wailsapp/wails/v2/cmd/wails@latest` |

Linux 额外需要：`libgtk-3-dev libwebkit2gtk-4.1-dev`

可选（AI Agent 冲突解决功能需要）：

| Agent | Binary | 安装方式 |
|-------|--------|---------|
| Claude Code | `claude` | `npm install -g @anthropic-ai/claude-code` |
| OpenCode | `opencode` | `go install github.com/opencode-ai/opencode@latest` |
| Codex | `codex` | `npm install -g @openai/codex` |

### 启动开发服务器

```bash
make wails-dev
# 或
wails dev
```

Wails dev 模式：
- Vite dev server 提供前端热重载（`http://localhost:5173`）
- Go 代码修改后自动重新编译
- 浏览器 DevTools：右键 → Inspect Element

### 构建生产版本

```bash
make wails          # 编译出 build/bin/forksync.app（macOS）/ ForkSync.exe（Windows）
make wails-dmg      # macOS：编译 + 打包 DMG（拖拽到 Applications）
make wails-nsis     # Windows：编译 + NSIS 安装包
```

## 添加新的引擎能力

1. **Go 端**：在 `app.go` 或 `ide.go` 中添加 `func (a *App) NewMethod(...) (ResultType, error)`
2. **重新生成绑定**：`wails generate module`（或在 `wails dev` 时自动生成）
3. **前端**：在 `frontend/src/lib/api.ts` 的 `engineApi` 对象中添加包装方法
4. **组件**：在 React 组件中调用 `engineApi.newMethod()`

Wails 自动将 Go 返回值序列化为 JSON，前端通过 `ok()` 包装成 `ApiResponse<T>` 格式。

> **注意**：Wails 生成的 TS 模型使用 **小写属性名**（匹配 Go JSON tag），如 `result.events` 而非 `result.Events`。

### 示例：添加 `GET /repos/{name}/log` 能力

1. Go 端：
```go
// app.go
func (a *App) RepoLog(name string) (RepoLogResult, error) {
    r, ok := a.deps.Store.GetByName(name)
    if !ok {
        return RepoLogResult{}, fmt.Errorf("repo %q not found", name)
    }
    logs := readLogs(r.Path)
    return RepoLogResult{Name: name, Logs: logs}, nil
}
```

2. 前端包装：
```typescript
// frontend/src/lib/api.ts
async repoLog(name) {
    try { return ok(await wailsRepoLog(name)) } catch (e) { return fail(e) }
},
```

3. 组件使用：
```tsx
const result = await engineApi.repoLog('my-repo')
```

## 添加新的 Agent 适配器

1. 在 `engine/core/agent/` 创建新文件，实现 `AgentProvider` 接口
2. 实现两个方法：`ResolveConflicts`（非流式）和 `ResolveConflictsWithStream`（流式）
3. 在 `engine/core/agent/registry.go` 的 `defaultAgentOrder` 中注册

## 测试

```bash
# Go 引擎测试
cd engine && go test -race -v ./...

# 前端测试
cd frontend && npx vitest run
```

## 常见问题

### Q: 开发模式下窗口打不开？

确保已安装 Wails CLI：`go install github.com/wailsapp/wails/v2/cmd/wails@latest`，然后用 `wails dev` 启动。

### Q: 如何调试 Go 代码？

`wails dev` 模式下，Go 的 `logger.Info/Warn/Error` 输出到 `~/.forksync/logs/`。也可在终端查看 stdout。

### Q: 如何打开 DevTools？

开发模式下右键 → Inspect Element。

### Q: Tailwind 类不生效？

检查 `frontend/tailwind.config.js` 的 `content` 路径：
```js
content: ['./index.html', './src/**/*.{ts,tsx}']
```

### Q: 修改 Go 类型后 TypeScript 报错？

修改 `engine/pkg/types/` 中的 Go 类型后，运行 `wails generate module` 重新生成 `frontend/src/wailsjs/go/models.ts`。

### Q: WKWebView 不支持 `window.confirm()` / `window.alert()`？

使用应用内的 `ConfirmDialog` 组件替代原生弹窗。WKWebView 会静默拦截这些原生对话框。
