/**
 * EngineClient — Go engine HTTP/WebSocket communication layer
 *
 * Talks to the embedded Go HTTP server managed by EngineServer (see server.ts).
 * All REST methods return ApiResponse<T> matching the Go engine's JSON contract,
 * preserving the exact public method signatures the IPC layer and renderer
 * depend on. resolveStream uses a WebSocket instead of NDJSON-over-stdout.
 */

import { getEngineServer } from './server'
import log from './logger'
// Electron main bundles Node 20 (no global WebSocket); import ws explicitly.
import { WebSocket } from "ws"
import type {
  ApiResponse,
  StatusData,
  SyncData,
  ScanData,
  AddData,
  RemoveData,
  ResolveData,
  AcceptData,
  RejectData,
  AgentListData,
  AgentSessionsData,
  AgentCleanupData,
  AgentResetData,
  HistoryData,
  EngineConfig,
  ConfigSetData,
  AgentStreamEvent,
  PostSyncCommand
} from '@shared/types/engine'

/** Default timeout for quick commands (status, config, history, etc.) */
const DEFAULT_TIMEOUT_MS = 30 * 1000

/** Timeout for long-running commands (sync, resolve with AI agents) */
const LONG_TIMEOUT_MS = 10 * 60 * 1000

export class EngineClient {
  // -----------------------------------------------------------------------
  // Public API — one method per engine capability. Signatures preserved from
  // the previous CLI-spawn implementation so the IPC layer is unchanged.
  // -----------------------------------------------------------------------

  async status(exclude?: string[]): Promise<ApiResponse<StatusData>> {
    const qs = exclude && exclude.length > 0 ? `?exclude=${encodeURIComponent(exclude.join(','))}` : ''
    return this.get<StatusData>(`/status${qs}`)
  }

  async syncAll(): Promise<ApiResponse<SyncData>> {
    return this.post<SyncData>('/sync/all', undefined, LONG_TIMEOUT_MS)
  }

  async syncRepo(name: string): Promise<ApiResponse<SyncData>> {
    return this.post<SyncData>(`/sync/repos/${encodeURIComponent(name)}`, undefined, LONG_TIMEOUT_MS)
  }

  async scan(dir: string): Promise<ApiResponse<ScanData>> {
    return this.post<ScanData>('/scan', { dir })
  }

  async add(
    repoPath: string,
    upstream?: string,
    branchMapping?: { localBranch: string; remoteBranch: string }
  ): Promise<ApiResponse<AddData>> {
    return this.post<AddData>('/repos', {
      path: repoPath,
      upstream,
      branchMapping
    })
  }

  async remove(name: string): Promise<ApiResponse<RemoveData>> {
    return this.delete<RemoveData>(`/repos/${encodeURIComponent(name)}`)
  }

  async resolve(
    name: string,
    opts?: { agent?: string; noConfirm?: boolean; prepare?: boolean; retry?: boolean; manual?: boolean }
  ): Promise<ApiResponse<ResolveData>> {
    const mode = opts?.prepare ? 'prepare' : 'agent'
    return this.post<ResolveData>(
      `/repos/${encodeURIComponent(name)}/resolve`,
      {
        mode,
        agent: opts?.agent,
        noConfirm: opts?.noConfirm,
        retry: opts?.retry,
        manual: opts?.manual
      },
      LONG_TIMEOUT_MS
    )
  }

  async resolvePrepare(name: string): Promise<ApiResponse<ResolveData>> {
    return this.post<ResolveData>(`/repos/${encodeURIComponent(name)}/resolve`, { mode: 'prepare' }, LONG_TIMEOUT_MS)
  }

  async resolveAccept(name: string): Promise<ApiResponse<AcceptData>> {
    return this.post<AcceptData>(`/repos/${encodeURIComponent(name)}/resolve`, { mode: 'accept' }, LONG_TIMEOUT_MS)
  }

  async resolveReject(name: string): Promise<ApiResponse<RejectData>> {
    return this.post<RejectData>(`/repos/${encodeURIComponent(name)}/resolve`, { mode: 'reject' }, LONG_TIMEOUT_MS)
  }

  async agentList(): Promise<ApiResponse<AgentListData>> {
    return this.get<AgentListData>('/agents')
  }

  async agentSessions(): Promise<ApiResponse<AgentSessionsData>> {
    return this.get<AgentSessionsData>('/agents/sessions')
  }

  async agentCleanup(): Promise<ApiResponse<AgentCleanupData>> {
    return this.post<AgentCleanupData>('/agents/cleanup', undefined)
  }

  async agentReset(name: string): Promise<ApiResponse<AgentResetData>> {
    return this.post<AgentResetData>(`/agents/${encodeURIComponent(name)}/reset`, undefined)
  }

  async history(repoName?: string, limit?: number): Promise<ApiResponse<HistoryData>> {
    const params = new URLSearchParams()
    if (repoName) params.set('repo', repoName)
    if (limit) params.set('limit', String(limit))
    const qs = params.toString() ? `?${params.toString()}` : ''
    return this.get<HistoryData>(`/history${qs}`)
  }

  async historyCleanup(opts?: { repoName?: string; keepDays?: number }): Promise<ApiResponse<{ message: string }>> {
    return this.post<{ message: string }>('/history/cleanup', {
      repo: opts?.repoName,
      keepDays: opts?.keepDays
    })
  }

  async configGet(): Promise<ApiResponse<EngineConfig>> {
    return this.get<EngineConfig>('/config')
  }

  async configSet(key: string, value: string): Promise<ApiResponse<ConfigSetData>> {
    return this.put<ConfigSetData>('/config', { key, value })
  }

  async postSyncList(repoName: string): Promise<ApiResponse<{ commands: PostSyncCommand[] }>> {
    return this.get<{ commands: PostSyncCommand[] }>(`/repos/${encodeURIComponent(repoName)}/post-sync`)
  }

  async postSyncAdd(repoName: string, cmdName: string, cmd: string): Promise<ApiResponse<{ commands: PostSyncCommand[] }>> {
    return this.post<{ commands: PostSyncCommand[] }>(`/repos/${encodeURIComponent(repoName)}/post-sync`, {
      name: cmdName,
      cmd
    })
  }

  async postSyncRemove(repoName: string, cmdId: string): Promise<ApiResponse<{ commands: PostSyncCommand[] }>> {
    return this.delete<{ commands: PostSyncCommand[] }>(`/repos/${encodeURIComponent(repoName)}/post-sync`, { id: cmdId })
  }

  async summarize(
    repoName: string
  ): Promise<ApiResponse<{ historyId: number; repoName: string; summary: string; summaryStatus: string }>> {
    return this.post<{ historyId: number; repoName: string; summary: string; summaryStatus: string }>(
      `/repos/${encodeURIComponent(repoName)}/summarize`,
      { retry: false },
      LONG_TIMEOUT_MS
    )
  }

  async summarizeRetry(
    repoName: string
  ): Promise<ApiResponse<{ historyId: number; repoName: string; summary: string; summaryStatus: string }>> {
    return this.post<{ historyId: number; repoName: string; summary: string; summaryStatus: string }>(
      `/repos/${encodeURIComponent(repoName)}/summarize`,
      { retry: true },
      LONG_TIMEOUT_MS
    )
  }

  /**
   * Open a WebSocket to /stream/resolve/:name and dispatch agent events.
   *
   * Preserves the previous controller interface (onEvent/onDone/onError/kill)
   * so ipc-engine.ts is unchanged. Event framing matches the old NDJSON
   * contract: a `done` frame closes the stream with the final result, an
   * `error` frame surfaces the error, everything else is a live event.
   */
  resolveStream(
    name: string,
    opts?: { agent?: string; noConfirm?: boolean }
  ): {
    onEvent: (cb: (ev: AgentStreamEvent) => void) => void
    onDone: (cb: (result: ApiResponse<ResolveData>) => void) => void
    onError: (cb: (err: string) => void) => void
    kill: () => void
  } {
    const eventCbs: Array<(ev: AgentStreamEvent) => void> = []
    const doneCbs: Array<(result: ApiResponse<ResolveData>) => void> = []
    const errorCbs: Array<(err: string) => void> = []

    let killed = false
    let notified = false
    let ws: WebSocket | null = null

    const notifyEvent = (ev: AgentStreamEvent): void => {
      for (const cb of eventCbs) cb(ev)
    }
    const notifyDone = (result: ApiResponse<ResolveData>): void => {
      if (notified) return
      notified = true
      log.info('[engine:resolveStream] done', name, 'success:', result.success)
      for (const cb of doneCbs) cb(result)
    }
    const notifyError = (err: string): void => {
      if (notified) return
      notified = true
      log.error('[engine:resolveStream] error', name, err)
      for (const cb of errorCbs) cb(err)
    }

    const params = new URLSearchParams()
    if (opts?.agent) params.set('agent', opts.agent)
    if (opts?.noConfirm) params.set('noConfirm', 'true')
    const qs = params.toString() ? `?${params.toString()}` : ''

    getEngineServer()
      .getWsUrl(`/stream/resolve/${encodeURIComponent(name)}${qs}`)
      .then((url) => {
        if (killed) return
        ws = new WebSocket(url)
        ws.on('message', (data: { toString: () => string }) => {
          const text = data.toString()
          if (!text) return
          let parsed: AgentStreamEvent & { success?: boolean; summary?: string; session_id?: string; resolvedFiles?: string[]; diff?: string; agentName?: string }
          try {
            parsed = JSON.parse(text)
          } catch {
            notifyEvent({ t: 'stdout', d: text, ts: new Date().toISOString() })
            return
          }
          if (parsed.t === 'done') {
            notifyDone({
              success: parsed.success ?? true,
              data: {
                repoId: '',
                conflicts: (parsed.resolvedFiles ?? []).map((f: string) => ({ path: f })),
                agentResult: {
                  success: parsed.success ?? true,
                  summary: parsed.summary ?? '',
                  sessionId: parsed.session_id ?? '',
                  agentName: parsed.agentName ?? '',
                  resolvedFiles: parsed.resolvedFiles ?? [],
                  diff: parsed.diff ?? ''
                }
              },
              error: ''
            } as ApiResponse<ResolveData>)
          } else if (parsed.t === 'error') {
            notifyError(parsed.d ?? 'Agent resolve error')
          } else {
            notifyEvent(parsed as AgentStreamEvent)
          }
        })
        ws.on('error', (): void => {
          notifyError('resolve stream WebSocket error')
        })
        ws.on('close', (): void => {
          // Safety net: socket closed without a terminal frame.
          if (!notified) {
            notifyDone({ success: true, data: null as unknown as ResolveData, error: '' })
          }
        })
      })
      .catch((err: unknown) => {
        notifyError(`Failed to open resolve stream: ${(err as Error).message}`)
      })

    return {
      onEvent: (cb) => { eventCbs.push(cb) },
      onDone: (cb) => { doneCbs.push(cb) },
      onError: (cb) => { errorCbs.push(cb) },
      kill: () => {
        killed = true
        try { ws?.close() } catch {}
      }
    }
  }

  /** Read the latest agent log replay (now served by the Go server). */
  async readAgentLog(repoName: string): Promise<{
    events: AgentStreamEvent[]
    isRunning: boolean
  }> {
    const res = await this.getRaw(`/repos/${encodeURIComponent(repoName)}/agent-log`)
    return (await res.json()) as { events: AgentStreamEvent[]; isRunning: boolean }
  }

  /** Get git diff for a repo working tree vs HEAD (now served by the Go server). */
  async repoDiff(repoName: string): Promise<{ success: boolean; diff?: string; error?: string }> {
    const res = await this.getRaw(`/repos/${encodeURIComponent(repoName)}/diff`)
    return (await res.json()) as { success: boolean; diff?: string; error?: string }
  }

  // -----------------------------------------------------------------------
  // Private — HTTP helpers
  // -----------------------------------------------------------------------

  private async baseUrl(): Promise<string> {
    return getEngineServer().getBaseUrl()
  }

  private async request<T>(
    method: string,
    path: string,
    body?: unknown,
    timeout = DEFAULT_TIMEOUT_MS
  ): Promise<ApiResponse<T>> {
    const url = (await this.baseUrl()) + path
    const init: RequestInit = {
      method,
      headers: { 'Content-Type': 'application/json' },
      signal: AbortSignal.timeout(timeout)
    }
    if (body !== undefined) {
      init.body = JSON.stringify(body)
    }
    log.debug(`[engine:http] ${method} ${path}`)
    let res: Response
    try {
      res = await fetch(url, init)
    } catch (err) {
      const e = err as Error
      const msg = e.name === 'TimeoutError' || e.name === 'AbortError'
        ? `Engine request timed out after ${timeout}ms`
        : `Engine request failed: ${e.message}`
      throw new EngineRequestError(msg, e.name)
    }
    const text = await res.text()
    try {
      return JSON.parse(text) as ApiResponse<T>
    } catch (err) {
      throw new EngineParseError(`Failed to parse engine output: ${(err as Error).message}`, text)
    }
  }

  private getRaw(path: string, timeout = DEFAULT_TIMEOUT_MS): Promise<Response> {
    return this.baseUrl().then((base) =>
      fetch(base + path, { signal: AbortSignal.timeout(timeout) })
    )
  }

  private get<T>(path: string, timeout?: number): Promise<ApiResponse<T>> {
    return this.request<T>('GET', path, undefined, timeout)
  }
  private post<T>(path: string, body?: unknown, timeout?: number): Promise<ApiResponse<T>> {
    return this.request<T>('POST', path, body, timeout)
  }
  private put<T>(path: string, body?: unknown, timeout?: number): Promise<ApiResponse<T>> {
    return this.request<T>('PUT', path, body, timeout)
  }
  private delete<T>(path: string, body?: unknown, timeout?: number): Promise<ApiResponse<T>> {
    return this.request<T>('DELETE', path, body, timeout)
  }
}

// ---------------------------------------------------------------------------
// Custom Error Types (kept for callers that may instanceof-check them).
// The old spawn-specific exit-code/stdout fields are removed; fetch has none.
// ---------------------------------------------------------------------------

export class EngineRequestError extends Error {
  readonly code: string
  constructor(message: string, code: string) {
    super(message)
    this.name = 'EngineRequestError'
    this.code = code
  }
}

export class EngineParseError extends Error {
  readonly body: string
  constructor(message: string, body: string) {
    super(message)
    this.name = 'EngineParseError'
    this.body = body
  }
}
