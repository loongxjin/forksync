/**
 * IPC Handlers — bridges Electron IPC to EngineClient
 *
 * Registers ipcMain.handle() for each engine command so the renderer
 * can invoke them via contextBridge-exposed API.
 */

import { ipcMain, dialog } from 'electron'
import { EngineClient } from './engine'
import { notifySyncResults } from './notify'

let engine: EngineClient | null = null

function getEngine(): EngineClient {
  if (!engine) {
    engine = new EngineClient()
  }
  return engine
}

export function registerIpcHandlers(): void {
  const e = getEngine()

  ipcMain.handle('engine:status', async () => {
    return e.status()
  })

  ipcMain.handle('engine:syncAll', async () => {
    const result = await e.syncAll()
    if (result.success && result.data?.results) {
      notifySyncResults(result.data.results)
    }
    return result
  })

  ipcMain.handle('engine:syncRepo', async (_event, name: string) => {
    const result = await e.syncRepo(name)
    if (result.success && result.data?.results) {
      notifySyncResults(result.data.results)
    }
    return result
  })

  ipcMain.handle('engine:scan', async (_event, dir: string) => {
    return e.scan(dir)
  })

  ipcMain.handle('engine:add', async (_event, path: string, upstream?: string, branchMapping?: { localBranch: string; remoteBranch: string }) => {
    return e.add(path, upstream, branchMapping)
  })

  ipcMain.handle('engine:remove', async (_event, name: string) => {
    return e.remove(name)
  })

  ipcMain.handle(
    'engine:resolve',
    async (_event, name: string, opts?: { agent?: string; noConfirm?: boolean }) => {
      return e.resolve(name, opts)
    }
  )

  ipcMain.handle('engine:resolveDone', async (_event, name: string) => {
    return e.resolveDone(name)
  })

  ipcMain.handle('engine:resolveReject', async (_event, name: string) => {
    return e.resolveReject(name)
  })

  ipcMain.handle('engine:agentList', async () => {
    return e.agentList()
  })

  ipcMain.handle('engine:agentSessions', async () => {
    return e.agentSessions()
  })

  ipcMain.handle('engine:agentCleanup', async () => {
    return e.agentCleanup()
  })

  ipcMain.handle('engine:history', async (_event, repoName?: string, limit?: number) => {
    return e.history(repoName, limit)
  })

  ipcMain.handle('engine:historyCleanup', async (_event, opts?: { repoName?: string; keepDays?: number }) => {
    return e.historyCleanup(opts)
  })

  ipcMain.handle('engine:configGet', async () => {
    return e.configGet()
  })

  ipcMain.handle('engine:configSet', async (_event, key: string, value: string) => {
    return e.configSet(key, value)
  })

  ipcMain.handle('dialog:openDirectory', async () => {
    try {
      const result = await dialog.showOpenDialog({
        properties: ['openDirectory'],
        title: 'Select Repository Directory'
      })
      return result
    } catch (err) {
      return {
        canceled: true,
        error: err instanceof Error ? err.message : String(err)
      }
    }
  })
}
