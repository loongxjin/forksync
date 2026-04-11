/**
 * Renderer-side API wrapper
 *
 * Provides typed access to the engine API exposed by the preload script.
 * All methods return ApiResponse<T> from the Go engine.
 */

import type {
  ApiResponse,
  StatusData,
  SyncData,
  ScanData,
  AddData,
  RemoveData,
  ResolveData,
  DoneData,
  RejectData,
  AgentListData,
  AgentSessionsData,
  AgentCleanupData,
  HistoryData,
  BranchMapping,
  EngineConfig,
  ConfigSetData
} from '@/types/engine'
import type { IDEInfo, IDEConfig, IDEOpenResult } from '@/types/ide'

export interface EngineAPI {
  status(): Promise<ApiResponse<StatusData>>
  syncAll(): Promise<ApiResponse<SyncData>>
  syncRepo(name: string): Promise<ApiResponse<SyncData>>
  scan(dir: string): Promise<ApiResponse<ScanData>>
  add(path: string, upstream?: string, branchMapping?: BranchMapping): Promise<ApiResponse<AddData>>
  remove(name: string): Promise<ApiResponse<RemoveData>>
  resolve(
    name: string,
    opts?: { agent?: string; noConfirm?: boolean }
  ): Promise<ApiResponse<ResolveData>>
  resolveDone(name: string): Promise<ApiResponse<DoneData>>
  resolveReject(name: string): Promise<ApiResponse<RejectData>>
  agentList(): Promise<ApiResponse<AgentListData>>
  agentSessions(): Promise<ApiResponse<AgentSessionsData>>
  agentCleanup(): Promise<ApiResponse<AgentCleanupData>>
  history(repoName?: string, limit?: number): Promise<ApiResponse<HistoryData>>
  historyCleanup(opts?: { repoName?: string; keepDays?: number }): Promise<ApiResponse<{ message: string }>>
  /** Open a directory dialog and return selected paths. Returns {canceled, filePaths?, error?} */
  openDirectory(): Promise<{ canceled: boolean; filePaths?: string[]; error?: string }>
  /** Listen for navigation events from main process (notification click-through). Returns unsubscribe fn. */
  onNavigate?: (callback: (path: string) => void) => () => void

  // IDE management
  ideDetect(): Promise<IDEInfo[]>
  ideOpen(repoPath: string, ideId: string): Promise<IDEOpenResult>
  ideGetConfig(): Promise<IDEConfig>
  ideSetDefault(ideId: string | null): Promise<{ success: boolean }>
  ideAddCustom(name: string, cliCommand: string): Promise<{ success: boolean; error?: string }>
  ideRemoveCustom(ideId: string): Promise<{ success: boolean }>

  // Config management
  configGet(): Promise<ApiResponse<EngineConfig>>
  configSet(key: string, value: string): Promise<ApiResponse<ConfigSetData>>
}

/** Typed access to the engine API exposed via preload contextBridge */
export const engineApi: EngineAPI = (window as unknown as { api: EngineAPI }).api
