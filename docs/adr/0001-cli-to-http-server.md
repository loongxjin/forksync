# ADR 0001: Replace CLI subprocess with embedded HTTP server

## Status
**Superseded** (2026-06-18) — The HTTP server layer was replaced by Wails
v2 bound methods. The engine now runs in-process with the desktop UI;
the standalone HTTP server (`engine/main.go`) remains only for headless use.
This ADR is retained as a historical record of the CLI → HTTP transition.

## Context
Before this ADR, the Electron main process communicated with the Go engine
by spawning a `forksync <subcommand>` CLI binary for every operation and
parsing its stdout (single-line JSON for most commands, NDJSON for streaming
agent resolve output). This model worked, but had several limitations:

- **Fast operations were slow**: spawning a Go subprocess (~50ms) dominated
  the cost of trivial reads like `history --limit 20` and `status`.
- **Request storms were invisible**: the renderer's `useEffect` polling
  loops were naturally rate-limited by spawn latency; once the CLI became an
  HTTP fetch (~5ms), excessive polling became a real perf problem (100+
  duplicate requests per sync).
- **No runtime config reload**: each spawn read fresh config, so settings
  changed via the UI took effect immediately. An embedded HTTP server
  snapshots config at boot; without explicit reload, settings like
  `conflict_strategy` would appear nonfunctional until restart.

## Decision
Replace the Cobra CLI with an embedded HTTP server that serves the same
capabilities over REST + WebSocket, while keeping the Electron main/renderer
boundary unchanged.

- **Go**: deleted `engine/cmd/` (15 Cobra commands); new `engine/internal/app/`
  package exposes REST handlers (one per former CLI command) plus a WebSocket
  endpoint for streaming agent resolve output. The binary is no longer a CLI;
  it is a long-lived server bound to `127.0.0.1:<random-port>`, announcing
  its address to stdout as `FORKSYNC_HTTP_ADDR=127.0.0.1:<port>`.
- **Electron**: new `server.ts` spawns the binary, reads the address line,
  polls `/healthz`, and exposes a base URL. `engine.ts` rewritten from
  `spawn + readline` to `fetch + WebSocket`, preserving all public method
  signatures so the IPC layer and renderer are untouched.

### HTTP API design
| Capability | Route | Returns |
|---|---|---|
| Health / version | `GET /healthz`, `GET /version` | bare `{ok}`, `ApiResponse<version>` |
| Status, scan, add, remove | `GET /status`, `POST /scan`, `POST /repos`, `DELETE /repos/{name}` | `ApiResponse<*Data>` |
| Sync | `POST /sync/all`, `POST /sync/repos/{name}` | `ApiResponse<SyncData>` |
| Resolve (agent/prepare/accept/reject) | `POST /repos/{name}/resolve` | `ApiResponse<*Data>` |
| Resolve stream (agent streaming output) | `GET /stream/resolve/{name}` (WebSocket) | `AgentStreamEvent` frames |
| Agent log replay | `GET /repos/{name}/agent-log` | bare `{events, isRunning}` |
| Repo diff | `GET /repos/{name}/diff` | bare `{success, diff?, error?}` |
| Agents, history, config, post-sync, summarize | `GET/POST/PUT /agents/*`, `/history*`, `/config`, `/repos/{name}/post-sync`, `/repos/{name}/summarize` | `ApiResponse<*Data>` |

### Config hot-reload
Syncer calls `reloadConfigAndSessionMgr()` at the top of every `executeSync`,
lazily constructing a session manager if `agent_resolve` is now enabled. This
restores the old CLI behavior where settings changed via the UI took effect
on the next sync without a restart.

## Consequences

### Positive
- **Latency**: `/history?limit=20` went from ~50ms (spawn) to ~5ms (fetch).
- **Observability**: structured logging replaced unstructured process stdout.
- **Simplicity**: one long-lived server process instead of N short-lived CLI
  processes; easier to add middleware (caching, body size limits).
- **Testability**: HTTP handlers are testable with `httptest`; the old CLI
  commands could only be tested end-to-end via subprocess.

### Negative
- **Startup dependency**: Electron must wait for the Go server to be ready
  before issuing engine calls. Boot order changed to `start server → register
  IPC handlers → create window`.
- **Config snapshot at boot**: the server reads config once at startup;
  runtime changes only propagate because `executeSync` explicitly reloads.
  Future endpoints not on the sync path (e.g. `handleStatus`) still use the
  boot snapshot — a known limitation tracked for a follow-up ADR.
- **WebSocket library**: Electron's main process (Node 20) has no global
  `WebSocket`, so the `ws` npm package was added. `resolveStream` now uses
  `ws`'s emitter API instead of the browser `WebSocket` API.
- **Log format change**: NDJSON-on-stdout stream is replaced by structured
  Go logger events. Agent log replay (`/repos/{name}/agent-log`) returns
  canonical JSON, preserving backward compatibility with the renderer's
  `useResolveStream` fallback poller.

### Mitigated
- **Config staleness bug**: initially, flipping `conflict_strategy` to
  `agent_resolve` in the settings UI had no effect until app restart because
  the Syncer's session manager was snapshotted at boot. Fixed by
  `reloadConfigAndSessionMgr()` (commit `e071037`).
- **History request storm**: the old CLI's spawn latency masked a tight
  effect dependency loop in HomePage. Fixed by reading `lastLoadAt` through a
  ref (commit before the ADR).
- **WS client crash**: `new WebSocket()` threw `ReferenceError` in the
  Electron main process. Fixed by importing the `ws` npm package.

## Alternatives considered
1. **Embed Go via native addon** — high build complexity, fragile ABI.
2. **Rewrite engine in TypeScript** — massive effort, risk of git-behavior
   drift from go-git.
3. **Keep CLI + add HTTP shim** — preserves the worst of both models.
4. **Replace with gRPC** — adds protobuf toolchain for no benefit on a single
   local connection.
