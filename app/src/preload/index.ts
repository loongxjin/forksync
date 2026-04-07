/**
 * Preload — exposes type-safe engine API to renderer via contextBridge
 */

import { contextBridge, ipcRenderer } from 'electron'
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
  HistoryData
} from '../renderer/src/types/engine'

const api = {
  status: (): Promise<ApiResponse<StatusData>> => ipcRenderer.invoke('engine:status'),
  syncAll: (): Promise<ApiResponse<SyncData>> => ipcRenderer.invoke('engine:syncAll'),
  syncRepo: (name: string): Promise<ApiResponse<SyncData>> =>
    ipcRenderer.invoke('engine:syncRepo', name),
  scan: (dir: string): Promise<ApiResponse<ScanData>> => ipcRenderer.invoke('engine:scan', dir),
  add: (path: string, upstream?: string): Promise<ApiResponse<AddData>> =>
    ipcRenderer.invoke('engine:add', path, upstream),
  remove: (name: string): Promise<ApiResponse<RemoveData>> =>
    ipcRenderer.invoke('engine:remove', name),
  resolve: (
    name: string,
    opts?: { agent?: string; noConfirm?: boolean }
  ): Promise<ApiResponse<ResolveData>> => ipcRenderer.invoke('engine:resolve', name, opts),
  resolveDone: (name: string): Promise<ApiResponse<DoneData>> =>
    ipcRenderer.invoke('engine:resolveDone', name),
  resolveReject: (name: string): Promise<ApiResponse<RejectData>> =>
    ipcRenderer.invoke('engine:resolveReject', name),
  agentList: (): Promise<ApiResponse<AgentListData>> => ipcRenderer.invoke('engine:agentList'),
  agentSessions: (): Promise<ApiResponse<AgentSessionsData>> =>
    ipcRenderer.invoke('engine:agentSessions'),
  agentCleanup: (): Promise<ApiResponse<AgentCleanupData>> =>
    ipcRenderer.invoke('engine:agentCleanup'),
  history: (repoName?: string, limit?: number): Promise<ApiResponse<HistoryData>> =>
    ipcRenderer.invoke('engine:history', repoName, limit),
  // Notification click-through navigation
  onNavigate: (callback: (path: string) => void): (() => void) => {
    const handler = (_event: Electron.IpcRendererEvent, path: string): void => callback(path)
    ipcRenderer.on('navigate', handler)
    return () => ipcRenderer.removeListener('navigate', handler)
  }
}

contextBridge.exposeInMainWorld('api', api)
