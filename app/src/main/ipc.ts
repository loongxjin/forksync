/**
 * IPC Handlers — bridges Electron IPC to EngineClient
 *
 * Registers ipcMain.handle() for each engine command so the renderer
 * can invoke them via contextBridge-exposed API.
 */

import { ipcMain } from 'electron'
import { EngineClient } from './engine'

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
    return e.syncAll()
  })

  ipcMain.handle('engine:syncRepo', async (_event, name: string) => {
    return e.syncRepo(name)
  })

  ipcMain.handle('engine:scan', async (_event, dir: string) => {
    return e.scan(dir)
  })

  ipcMain.handle('engine:add', async (_event, path: string, upstream?: string) => {
    return e.add(path, upstream)
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
}
