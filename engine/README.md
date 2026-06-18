# ForkSync Engine

Go 核心引擎 for ForkSync — 自动同步 fork 仓库的 macOS 桌面应用。

ForkSync 桌面应用（Wails）或外部调用方可 spawn 此二进制文件作为一个本地 HTTP server
（`127.0.0.1:<随机端口>`），通过 REST + WebSocket 通信。

## 构建

```bash
go run .                  # 启动 HTTP server（默认 127.0.0.1:0）
make engine               # 产出 build/forksync

# 生产（交叉编译 + 瘦身）
CGO_ENABLED=0 go build -ldflags "-s -w" -o ../build/forksync .
```

## 运行

```bash
./forksync -addr 127.0.0.1:0
# 启动后 stdout 打印一行 FORKSYNC_HTTP_ADDR=127.0.0.1:<port>
# 调用方读取该行获取端口，然后轮询 GET /healthz 直到就绪
```

收到 SIGINT/SIGTERM 优雅退出（停止 scheduler，关闭 HTTP server）。

## REST API

所有端点（除 `/healthz`、`/repos/{name}/agent-log`、`/repos/{name}/diff`）返回 `ApiResponse<T>` 信封：

```json
{ "success": true, "data": { ... }, "error": "" }
```

### 状态 & 仓库管理
| Method | Route | Body | 返回 |
|---|---|---|---|
| `GET` | `/healthz` | — | `{"ok":true}`（裸 JSON） |
| `GET` | `/version` | — | `ApiResponse<{version,commit,builtAt}>` |
| `GET` | `/status` | query `?exclude=a,b` | `ApiResponse<StatusData>` |
| `POST` | `/scan` | `{dir}` | `ApiResponse<ScanData>` |
| `POST` | `/repos` | `{path,upstream?,branchMapping?}` | `ApiResponse<AddData>` |
| `DELETE` | `/repos/{name}` | — | `ApiResponse<{removed}>` |
| `GET` | `/repos/{name}/diff` | — | 裸 `{success,diff?,error?}` |

### 同步
| Method | Route | 返回 |
|---|---|---|
| `POST` | `/sync/all` | `ApiResponse<SyncData>` |
| `POST` | `/sync/repos/{name}` | `ApiResponse<SyncData>` |

### 冲突解决
| Method | Route | Body / Query | 返回 |
|---|---|---|---|
| `POST` | `/repos/{name}/resolve` | `{mode:"agent"\|"prepare"\|"accept"\|"reject", agent?, noConfirm?, manual?, retry?}` | `ApiResponse<ResolveData\|AcceptData\|RejectData>` |
| `WS` | `/stream/resolve/{name}` | query `?agent=&noConfirm=` | 逐帧 `AgentStreamEvent`，结束 `done`/`error` |

### Agent
| Method | Route | 返回 |
|---|---|---|
| `GET` | `/agents` | `ApiResponse<AgentListData>` |
| `GET` | `/agents/sessions` | `ApiResponse<AgentSessionsData>` |
| `POST` | `/agents/cleanup` | `ApiResponse<{removed}>` |
| `POST` | `/agents/{name}/reset` | `ApiResponse<AgentResetData>` |
| `GET` | `/repos/{name}/agent-log` | 裸 `{events,isRunning}` |

### 历史 & 配置 & 摘要
| Method | Route | Body | 返回 |
|---|---|---|---|
| `GET` | `/history?repo=&limit=` | — | `ApiResponse<HistoryData>` |
| `POST` | `/history/cleanup` | `{repo?,keepDays?}` | `ApiResponse<{message}>` |
| `GET` | `/config` | — | `ApiResponse<EngineConfig>` |
| `PUT` | `/config` | `{key,value}` | `ApiResponse<{key,value}>` |
| `GET/POST/DELETE` | `/repos/{name}/post-sync` | `{name?,cmd?,id?}` | `ApiResponse<{commands}>` |
| `POST` | `/repos/{name}/summarize` | `{retry?}` | `ApiResponse<{historyId,repoName,summary,summaryStatus}>` |

## 配置

配置文件位于 `~/.forksync/config.yaml`。server 启动时读取一次，但 syncer 每次 sync 时会从磁盘重新加载（因此通过设置 UI 修改 `conflict_strategy` 后，下一次 sync 即生效，无需重启）。

```yaml
agent:
  preferred: ""
  priority: [claude, opencode, codex]
  timeout: "10m"
  conflict_strategy: "agent_resolve"  # agent_resolve | preserve_ours | preserve_theirs
  confirm_before_commit: true
  session_ttl: "24h"
sync:
  default_interval: "30m"
  sync_on_startup: true
notification:
  enabled: true
proxy:
  enabled: false
  url: ""
```

## JSON contract types

所有数据结构定义在 `pkg/types/` 和 `app/src/shared/types/engine.ts` 中。
前后端共享同一套结构体标签（`json:"camelCase"`）。

## 项目结构

```
engine/
├── main.go                  # HTTP server 入口
├── core/
│   ├── app/                 # HTTP handlers（替代旧 cmd/ Cobra 命令）
│   │   ├── server.go        #   Server 骨架 + 路由注册 + SIGTERM 优雅关闭
│   │   ├── deps.go          #   依赖装配（config/store/syncer/resolver）
│   │   ├── respond.go       #   响应助手（writeOK/writeErr/writeBare/decodeJSON）
│   │   ├── handlers_repo.go #   status/scan/add/remove/diff
│   │   ├── handlers_sync.go #   syncAll/syncRepo
│   │   ├── handlers_resolve.go  # resolve(agent/prepare/accept/reject) + agent-log
│   │   ├── handlers_stream.go   # WebSocket resolve stream
│   │   └── handlers_misc.go     # agent/history/config/post-sync/summarize
│   ├── sync/                #   同步引擎 + 状态刷新器
│   ├── resolve/             #   冲突解决器
│   ├── agent/               #   Agent 适配器（claude/opencode/codex）+ 流式输出 + 日志
│   │   └── session/         #   Session 管理器
│   ├── history/             #   SQLite 同步历史
│   ├── config/              #   配置文件管理
│   ├── repo/                #   仓库 JSON 存储
│   ├── git/                 #   go-git 操作封装
│   ├── github/              #   GitHub API（fork 检测）
│   ├── summarizer/          #   Agent 摘要生成（同步调用 Executor）
│   ├── workflow/            #   工作流状态机
│   ├── scheduler/           #   后台定时同步
│   ├── notify/              #   桌面通知
│   └── logger/              #   slog 日志（日滚动）
└── pkg/
    ├── types/               #   共享类型（前后端 JSON 契约）
    └── version/             #   版本号
```

## 支持的 AI Agent

| Agent | Binary | 非交互调用 |
|-------|--------|-----------|
| Claude Code | `claude` | `claude --print <prompt>` |
| OpenCode | `opencode` | `opencode run <message>` |
| Codex | `codex` | `codex <prompt>` |

ForkSync 自动检测 PATH 中已安装的 agent，按配置的优先级选择。

## 测试

```bash
go test ./...              # 运行所有测试
go test ./core/app/ -v # HTTP handler 测试
```

## 依赖

- `go-git` — Git 操作
- `modernc.org/sqlite` — 纯 Go SQLite（零 CGO）
- `gorilla/websocket` — WebSocket 服务端
- `viper/mapstructure` — 配置文件解析
