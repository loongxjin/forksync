/**
 * Preload — exposes type-safe engine API to renderer via contextBridge
 */

import { contextBridge, ipcRenderer } from 'electron'

const api = {
  status: (): Promise<void> => ipcRenderer.invoke('engine:status'),
  syncAll: (): Promise<void> => ipcRenderer.invoke('engine:syncAll'),
  syncRepo: (name: string): Promise<void> => ipcRenderer.invoke('engine:syncRepo', name),
  scan: (dir: string): Promise<void> => ipcRenderer.invoke('engine:scan', dir),
  add: (path: string, upstream?: string): Promise<void> =>
    ipcRenderer.invoke('engine:add', path, upstream),
  remove: (name: string): Promise<void> => ipcRenderer.invoke('engine:remove', name),
  resolve: (
    name: string,
    opts?: { agent?: string; noConfirm?: boolean }
  ): Promise<void> => ipcRenderer.invoke('engine:resolve', name, opts),
  resolveDone: (name: string): Promise<void> =>
    ipcRenderer.invoke('engine:resolveDone', name),
  resolveReject: (name: string): Promise<void> =>
    ipcRenderer.invoke('engine:resolveReject', name),
  agentList: (): Promise<void> => ipcRenderer.invoke('engine:agentList'),
  agentSessions: (): Promise<void> => ipcRenderer.invoke('engine:agentSessions'),
  agentCleanup: (): Promise<void> => ipcRenderer.invoke('engine:agentCleanup')
}

contextBridge.exposeInMainWorld('api', api)
