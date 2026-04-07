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
  AgentCleanupData
} from '@/types/engine'

export interface EngineAPI {
  status(): Promise<ApiResponse<StatusData>>
  syncAll(): Promise<ApiResponse<SyncData>>
  syncRepo(name: string): Promise<ApiResponse<SyncData>>
  scan(dir: string): Promise<ApiResponse<ScanData>>
  add(path: string, upstream?: string): Promise<ApiResponse<AddData>>
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
}

/** Typed access to the engine API exposed via preload contextBridge */
export const engineApi: EngineAPI = (window as unknown as { api: EngineAPI }).api
