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
  HistoryData,
  BranchMapping,
  EngineConfig,
  ConfigSetData
} from '../renderer/src/types/engine'
import type { IDEInfo, IDEConfig, IDEOpenResult } from '../renderer/src/types/ide'

const api = {
  status: (): Promise<ApiResponse<StatusData>> => ipcRenderer.invoke('engine:status'),
  syncAll: (): Promise<ApiResponse<SyncData>> => ipcRenderer.invoke('engine:syncAll'),
  syncRepo: (name: string): Promise<ApiResponse<SyncData>> =>
    ipcRenderer.invoke('engine:syncRepo', name),
  scan: (dir: string): Promise<ApiResponse<ScanData>> => ipcRenderer.invoke('engine:scan', dir),
  add: (path: string, upstream?: string, branchMapping?: BranchMapping): Promise<ApiResponse<AddData>> =>
    ipcRenderer.invoke('engine:add', path, upstream, branchMapping),
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
  historyCleanup: (opts?: { repoName?: string; keepDays?: number }): Promise<ApiResponse<{ message: string }>> =>
    ipcRenderer.invoke('engine:historyCleanup', opts),
  configGet: (): Promise<ApiResponse<EngineConfig>> =>
    ipcRenderer.invoke('engine:configGet'),
  configSet: (key: string, value: string): Promise<ApiResponse<ConfigSetData>> =>
    ipcRenderer.invoke('engine:configSet', key, value),
  setAutoLaunch: (enabled: boolean): Promise<{ success: boolean; error?: string }> =>
    ipcRenderer.invoke('app:setAutoLaunch', enabled),
  openDirectory: (): Promise<{ canceled: boolean; filePaths?: string[]; error?: string }> =>
    ipcRenderer.invoke('dialog:openDirectory'),
  isGitRepo: (dirPath: string): Promise<boolean> =>
    ipcRenderer.invoke('fs:isGitRepo', dirPath),
  // Notification click-through navigation
  onNavigate: (callback: (path: string) => void): (() => void) => {
    const handler = (_event: Electron.IpcRendererEvent, path: string): void => callback(path)
    ipcRenderer.on('navigate', handler)
    return () => ipcRenderer.removeListener('navigate', handler)
  },

  // IDE management
  ideDetect: (): Promise<IDEInfo[]> => ipcRenderer.invoke('ide:detect'),
  ideOpen: (repoPath: string, ideId: string): Promise<IDEOpenResult> =>
    ipcRenderer.invoke('ide:open', repoPath, ideId),
  ideGetConfig: (): Promise<IDEConfig> => ipcRenderer.invoke('ide:getConfig'),
  ideSetDefault: (ideId: string | null): Promise<{ success: boolean }> =>
    ipcRenderer.invoke('ide:setDefault', ideId),
  ideAddCustom: (name: string, cliCommand: string): Promise<{ success: boolean; error?: string }> =>
    ipcRenderer.invoke('ide:addCustom', name, cliCommand),
  ideRemoveCustom: (ideId: string): Promise<{ success: boolean }> =>
    ipcRenderer.invoke('ide:removeCustom', ideId)
}

contextBridge.exposeInMainWorld('api', api)
