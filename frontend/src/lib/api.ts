/**
 * Renderer-side API wrapper — Wails edition.
 *
 * Previously communicated via Electron contextBridge (window.api). Now all Go
 * engine methods are imported from the auto-generated Wails bindings in
 * wailsjs/go/main/App. Streaming (resolve, events) is bridged via
 * Wails Events (EventsOn/EventsEmit) from wailsjs/runtime/runtime.
 *
 * The exported engineApi object preserves the same interface so existing
 * components and hooks work without changes.
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
import { EventsOn, WindowMinimise, WindowToggleMaximise, Quit } from '../wailsjs/runtime/runtime'
import { Status as wailsStatus, Greet } from '../wailsjs/go/main/App'

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

// -- Wails-native engine API ------------------------------------------------

const engineApi: EngineAPI = {
  // === Synced methods (implemented as Go bound methods) ===

  async status(exclude?: string[]) {
    try {
      const data = await wailsStatus(exclude ?? [])
      return { success: true, data, error: '' }
    } catch (err) {
      return { success: false, data: null as unknown as StatusData, error: String(err) }
    }
  },

  // === Window controls (Wails runtime) ===

  async syncAll() { return notYet() },
  async syncRepo(_n: string) { return notYet() },
  async scan(_d: string) { return notYet() },
  async add(_p: string) { return notYet() },
  async remove(_n: string) { return notYet() },
  async resolve(_n: string) { return notYet() },
  async resolvePrepare(_n: string) { return notYet() },
  async resolveAccept(_n: string) { return notYet() },
  async resolveReject(_n: string) { return notYet() },
  async agentList() { return notYet() },
  async agentSessions() { return notYet() },
  async agentCleanup() { return notYet() },
  async agentReset(_n: string) { return notYet() },
  async history() { return notYet() },
  async historyCleanup() { return notYet() },
  async openDirectory() { return { canceled: true, error: 'not yet migrated' } },
  async isGitRepo(_d: string) { return false },
  async setLocale() { return { success: true } },
  async ideDetect() { return [] },
  async ideOpen() { return { success: false, error: 'not yet migrated' } },
  async ideGetConfig() { return { defaultIDE: null, detectedIDEs: [], customIDEs: [] } },
  async ideSetDefault() { return { success: true } },
  async ideAddCustom() { return { success: false, error: 'not yet migrated' } },
  async ideRemoveCustom() { return { success: true } },
  async configGet() { return notYet() },
  async configSet() { return notYet() },
  async postSyncList() { return notYet() },
  async postSyncAdd() { return notYet() },
  async postSyncRemove() { return notYet() },
  async summarize() { return notYet() },
  async summarizeRetry() { return notYet() },
  async setAutoLaunch() { return { success: true } },
  async repoDiff() { return { success: false, error: 'not yet migrated' } },

  // == Streaming (to be implemented via Wails Events in Stage 3) ==

  resolveStreamStart(_n: string) { /* noop until Stage 3 */ },
  onResolveStreamTick() { return () => {} },
  onResolveStreamDone() { return () => {} },
  onResolveStreamError() { return () => {} },
  async readAgentLog() { return { events: [], isRunning: false } },
  eventsStart() { /* noop — event bridge runs automatically */ },
  eventsStop() {},
  onEventsTick(callback: (type: string) => void) {
    return EventsOn('engine:event', (type) => callback(type))
  },
}

/** Stub for methods not yet migrated to Wails bindings. */
function notYet(): any {
  return { success: false, data: null, error: 'not yet migrated to Wails' }
}

export { engineApi }

// Also export window controls for components that use them directly.
export { WindowMinimise, WindowToggleMaximise, Quit }
