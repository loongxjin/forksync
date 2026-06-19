/**
 * Renderer-side API wrapper — Wails edition.
 *
 * All Go engine methods are bound via auto-generated Wails bindings in
 * wailsjs/go/main/App. Streaming (resolve, events) is bridged via
 * Wails Events (EventsOn/EventsEmit).
 *
 * Wails bindings return plain data (throw on error). We wrap them in
 * the ApiResponse<T> shape the rest of the app expects.
 */

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
  BranchMapping,
  EngineConfig,
  ConfigSetData,
  PostSyncCommandsData,
  AgentStreamEvent,
} from '../shared/types/engine'
import type { IDEInfo, IDEConfig, IDEOpenResult } from '../shared/types/ide'
import { EventsOn } from '../wailsjs/runtime/runtime'
import {
  Status as wailsStatus,
  SyncAll as wailsSyncAll,
  SyncRepo as wailsSyncRepo,
  Scan as wailsScan,
  AddRepo as wailsAddRepo,
  RemoveRepo as wailsRemoveRepo,
  Resolve as wailsResolve,
  ResolveStreamStart as wailsResolveStreamStart,
  RepoDiff as wailsRepoDiff,
  AgentList as wailsAgentList,
  AgentSessions as wailsAgentSessions,
  AgentCleanup as wailsAgentCleanup,
  AgentReset as wailsAgentReset,
  History as wailsHistory,
  HistoryCleanup as wailsHistoryCleanup,
  ConfigGet as wailsConfigGet,
  ConfigSet as wailsConfigSet,
  ReadAgentLog as wailsReadAgentLog,
  PostSyncList as wailsPostSyncList,
  PostSyncAdd as wailsPostSyncAdd,
  PostSyncRemove as wailsPostSyncRemove,
  Summarize as wailsSummarize,
  SummarizeRetry as wailsSummarizeRetry,
  IDEDetect as wailsIDEDetect,
  IDEOpen as wailsIDEOpen,
  IDEGetConfig as wailsIDEGetConfig,
  IDESetDefault as wailsIDESetDefault,
  IDEAddCustom as wailsIDEAddCustom,
  IDERemoveCustom as wailsIDERemoveCustom,
  OpenDirectoryDialog as wailsOpenDirectoryDialog,
  IsGitRepo as wailsIsGitRepo,
  SetLocale as wailsSetLocale,
  SetAutoLaunch as wailsSetAutoLaunch,
} from '../wailsjs/go/main/App'

export interface EngineAPI {
  status(exclude?: string[]): Promise<ApiResponse<StatusData>>
  syncAll(): Promise<ApiResponse<SyncData>>
  syncRepo(name: string): Promise<ApiResponse<SyncData>>
  scan(dir: string): Promise<ApiResponse<ScanData>>
  add(path: string, upstream?: string, branchMapping?: BranchMapping): Promise<ApiResponse<AddData>>
  remove(name: string): Promise<ApiResponse<RemoveData>>
  resolve(name: string, opts?: { agent?: string; noConfirm?: boolean; prepare?: boolean; retry?: boolean; manual?: boolean }): Promise<ApiResponse<ResolveData>>
  resolvePrepare(name: string): Promise<ApiResponse<ResolveData>>
  resolveAccept(name: string): Promise<ApiResponse<AcceptData>>
  resolveReject(name: string): Promise<ApiResponse<RejectData>>
  agentList(): Promise<ApiResponse<AgentListData>>
  agentSessions(): Promise<ApiResponse<AgentSessionsData>>
  agentCleanup(): Promise<ApiResponse<AgentCleanupData>>
  agentReset(name: string): Promise<ApiResponse<AgentResetData>>
  history(repoName?: string, limit?: number): Promise<ApiResponse<HistoryData>>
  historyCleanup(opts?: { repoName?: string; keepDays?: number }): Promise<ApiResponse<{ message: string }>>
  openDirectory(): Promise<{ canceled: boolean; filePaths?: string[]; error?: string }>
  isGitRepo(dirPath: string): Promise<boolean>
  onNavigate?: (callback: (path: string) => void) => () => void
  setLocale(locale: string): Promise<{ success: boolean }>
  ideDetect(): Promise<IDEInfo[]>
  ideOpen(repoPath: string, ideId: string): Promise<IDEOpenResult>
  ideGetConfig(): Promise<IDEConfig>
  ideSetDefault(ideId: string | null): Promise<{ success: boolean }>
  ideAddCustom(name: string, cliCommand: string): Promise<{ success: boolean; error?: string }>
  ideRemoveCustom(ideId: string): Promise<{ success: boolean }>
  configGet(): Promise<ApiResponse<EngineConfig>>
  configSet(key: string, value: string): Promise<ApiResponse<ConfigSetData>>
  postSyncList(repoName: string): Promise<ApiResponse<PostSyncCommandsData>>
  postSyncAdd(repoName: string, name: string, cmd: string): Promise<ApiResponse<PostSyncCommandsData>>
  postSyncRemove(repoName: string, cmdId: string): Promise<ApiResponse<PostSyncCommandsData>>
  summarize(repoName: string): Promise<ApiResponse<{ historyId: number; repoName: string; summary: string; summaryStatus: string }>>
  summarizeRetry(repoName: string): Promise<ApiResponse<{ historyId: number; repoName: string; summary: string; summaryStatus: string }>>
  setAutoLaunch(enabled: boolean): Promise<{ success: boolean; error?: string }>
  repoDiff(repoName: string): Promise<{ success: boolean; diff?: string; error?: string }>
  resolveStreamStart(name: string, opts?: { agent?: string; noConfirm?: boolean }): void
  onResolveStreamTick(callback: (repoName: string) => void): () => void
  onResolveStreamDone(callback: (repoName: string, result: ApiResponse<ResolveData>) => void): () => void
  onResolveStreamError(callback: (repoName: string, error: string) => void): () => void
  readAgentLog(repoName: string, sessionId?: string): Promise<{ events: AgentStreamEvent[]; isRunning: boolean }>
  eventsStart(): void
  eventsStop(): void
  onEventsTick(callback: (type: string) => void): () => void
}

function ok<T>(data: T): ApiResponse<T> {
  return { success: true, data, error: '' }
}

function fail<T>(err: unknown): ApiResponse<T> {
  return { success: false, data: null as unknown as T, error: String(err) }
}

const engineApi: EngineAPI = {
  async status(exclude) {
    try { return ok(await wailsStatus(exclude ?? [])) } catch (e) { return fail(e) }
  },
  async syncAll() {
    try { return ok(await wailsSyncAll()) } catch (e) { return fail(e) }
  },
  async syncRepo(name) {
    try { return ok(await wailsSyncRepo(name)) } catch (e) { return fail(e) }
  },
  async scan(dir) {
    try { return ok(await wailsScan(dir)) } catch (e) { return fail(e) }
  },
  async add(path, upstream, branchMapping) {
    try { return ok(await wailsAddRepo({ path, upstream, branchMapping })) } catch (e) { return fail(e) }
  },
  async remove(name) {
    try { return ok(await wailsRemoveRepo(name)) } catch (e) { return fail(e) }
  },
  async resolve(name, opts) {
    try { return ok(await wailsResolve(name, {
      mode: opts?.prepare ? 'prepare' : 'agent',
      agent: opts?.agent ?? '',
      noConfirm: opts?.noConfirm ?? false,
      manual: opts?.manual ?? false,
      retry: opts?.retry ?? false,
    })) } catch (e) { return fail(e) }
  },
  async resolvePrepare(name) {
    try { return ok(await wailsResolve(name, { mode: 'prepare', agent: '', noConfirm: false, manual: false, retry: false })) } catch (e) { return fail(e) }
  },
  async resolveAccept(name) {
    try { return ok(await wailsResolve(name, { mode: 'accept', agent: '', noConfirm: false, manual: false, retry: false })) } catch (e) { return fail(e) }
  },
  async resolveReject(name) {
    try { return ok(await wailsResolve(name, { mode: 'reject', agent: '', noConfirm: false, manual: false, retry: false })) } catch (e) { return fail(e) }
  },
  async agentList() {
    try { return ok(await wailsAgentList()) } catch (e) { return fail(e) }
  },
  async agentSessions() {
    try { return ok(await wailsAgentSessions()) } catch (e) { return fail(e) }
  },
  async agentCleanup() {
    try { return ok(await wailsAgentCleanup()) } catch (e) { return fail(e) }
  },
  async agentReset(name) {
    try { return ok(await wailsAgentReset(name)) } catch (e) { return fail(e) }
  },
  async history(repoName, limit) {
    try { return ok(await wailsHistory(repoName ?? '', limit ?? 20)) } catch (e) { return fail(e) }
  },
  async historyCleanup(opts) {
    try { return ok(await wailsHistoryCleanup({ repo: opts?.repoName ?? '', keepDays: opts?.keepDays ?? 0 })) } catch (e) { return fail(e) }
  },
  async openDirectory() {
    try {
      const result = await wailsOpenDirectoryDialog()
      return result as { canceled: boolean; filePaths?: string[]; error?: string }
    } catch (e) { return { canceled: true, error: String(e) } }
  },
  async isGitRepo(dirPath) {
    try { return await wailsIsGitRepo(dirPath) } catch (e) { return false }
  },
  async setLocale(locale) {
    try { return await wailsSetLocale(locale) as { success: boolean } } catch (e) { return { success: false } }
  },
  async ideDetect() {
    try { return await wailsIDEDetect() } catch (e) { return [] }
  },
  async ideOpen(repoPath, ideId) {
    try { return await wailsIDEOpen(repoPath, ideId) } catch (e) { return { success: false, error: String(e) } }
  },
  async ideGetConfig() {
    try { return await wailsIDEGetConfig() } catch (e) { return { defaultIDE: null, detectedIDEs: [], customIDEs: [] } }
  },
  async ideSetDefault(ideId) {
    try { return await wailsIDESetDefault(ideId ?? '') as { success: boolean } } catch (e) { return { success: false } }
  },
  async ideAddCustom(name, cliCommand) {
    try { return await wailsIDEAddCustom(name, cliCommand) as { success: boolean; error?: string } } catch (e) { return { success: false, error: String(e) } }
  },
  async ideRemoveCustom(ideId) {
    try { return await wailsIDERemoveCustom(ideId) as { success: boolean } } catch (e) { return { success: false } }
  },
  async configGet() {
    try { return ok(await wailsConfigGet()) } catch (e) { return fail(e) }
  },
  async configSet(key, value) {
    try { return ok(await wailsConfigSet(key, value)) } catch (e) { return fail(e) }
  },
  async postSyncList(repoName) {
    try { return ok(await wailsPostSyncList(repoName)) } catch (e) { return fail(e) }
  },
  async postSyncAdd(repoName, name, cmd) {
    try { return ok(await wailsPostSyncAdd(repoName, { name, cmd })) } catch (e) { return fail(e) }
  },
  async postSyncRemove(repoName, cmdId) {
    try { return ok(await wailsPostSyncRemove(repoName, { id: cmdId })) } catch (e) { return fail(e) }
  },
  async summarize(repoName) {
    try { return ok(await wailsSummarize(repoName, { retry: false })) } catch (e) { return fail(e) }
  },
  async summarizeRetry(repoName) {
    try { return ok(await wailsSummarizeRetry(repoName)) } catch (e) { return fail(e) }
  },
  async setAutoLaunch(enabled) {
    try { return await wailsSetAutoLaunch(enabled) as { success: boolean; error?: string } } catch (e) { return { success: false, error: String(e) } }
  },
  async repoDiff(name) {
    try {
      const result = await wailsRepoDiff(name)
      return { success: result.success, diff: result.diff, error: result.error }
    } catch (e) { return { success: false, error: String(e) } }
  },
  // Streaming via Wails Events
  resolveStreamStart(name, opts) {
    wailsResolveStreamStart(name, opts?.agent ?? '', opts?.noConfirm ?? false)
  },
  onResolveStreamTick(callback) {
    return EventsOn('resolve:tick', (repoName: string) => {
      callback(repoName)
    })
  },
  onResolveStreamDone(callback) {
    return EventsOn('resolve:done', (repoName: string, result: any) => {
      callback(repoName, { success: true, data: result, error: '' })
    })
  },
  onResolveStreamError(callback) {
    return EventsOn('resolve:error', (repoName: string, error: string) => {
      callback(repoName, error)
    })
  },
  async readAgentLog(repoName, sessionId) {
    try {
      const result = await wailsReadAgentLog(repoName, sessionId ?? '')
      return { events: result.events, isRunning: result.isRunning }
    } catch (e) {
      return { events: [], isRunning: false }
    }
  },
  eventsStart() {},
  eventsStop() {},
  onEventsTick(callback: (type: string) => void) {
    return EventsOn('engine:event', (type) => callback(type))
  },
}

export { engineApi }
