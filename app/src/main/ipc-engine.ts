/**
 * IPC Engine Handlers — engine commands and streaming
 */

import { ipcMain, app } from 'electron'
import { resolve, normalize } from 'path'
import { EngineClient } from './engine'
import { notifySyncResults, updateNotificationConfig } from './notify'
import log from './logger'

// --- Input validation helpers ---

export function assertString(value: unknown, name: string): string {
  if (typeof value !== 'string' || value.length === 0) {
    throw new Error(`Invalid ${name}: expected non-empty string`)
  }
  if (value.length > 4096) {
    throw new Error(`Invalid ${name}: exceeds maximum length`)
  }
  return value
}

export function assertOptionalString(value: unknown, name: string): string | undefined {
  if (value === undefined || value === null) return undefined
  if (typeof value !== 'string') {
    throw new Error(`Invalid ${name}: expected string`)
  }
  if (value.length > 4096) {
    throw new Error(`Invalid ${name}: exceeds maximum length`)
  }
  return value
}

export function assertSafePath(value: unknown, name: string): string {
  const str = assertString(value, name)
  const resolved = resolve(normalize(str))
  if (str.includes('..')) {
    throw new Error(`Invalid ${name}: path traversal detected`)
  }
  return resolved
}

// --- Engine singleton ---

let engine: EngineClient | null = null

export function getEngine(): EngineClient {
  if (!engine) {
    engine = new EngineClient()
  }
  return engine
}

// --- Registration ---

export function registerEngineIpcHandlers(): void {
  const e = getEngine()

  ipcMain.handle('engine:status', async (_event, exclude?: string[]) => {
    return e.status(exclude)
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
    return e.scan(assertSafePath(dir, 'dir'))
  })

  ipcMain.handle('engine:add', async (_event, path: string, upstream?: string, branchMapping?: { localBranch: string; remoteBranch: string }) => {
    return e.add(assertSafePath(path, 'path'), assertOptionalString(upstream, 'upstream'), branchMapping)
  })

  ipcMain.handle('engine:remove', async (_event, name: string) => {
    return e.remove(assertString(name, 'name'))
  })

  ipcMain.handle(
    'engine:resolve',
    async (_event, name: string, opts?: { agent?: string; noConfirm?: boolean; prepare?: boolean; retry?: boolean; manual?: boolean }) => {
      return e.resolve(assertString(name, 'name'), opts)
    }
  )

  ipcMain.handle('engine:resolvePrepare', async (_event, name: string) => {
    return e.resolvePrepare(assertString(name, 'name'))
  })

  ipcMain.handle('engine:resolveAccept', async (_event, name: string) => {
    return e.resolveAccept(assertString(name, 'name'))
  })

  ipcMain.handle('engine:resolveReject', async (_event, name: string) => {
    return e.resolveReject(assertString(name, 'name'))
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

  ipcMain.handle('engine:agentReset', async (_event, name: string) => {
    return e.agentReset(assertString(name, 'name'))
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
    assertString(key, 'key')
    const safeValue = assertString(value, 'value')
    const result = await e.configSet(key, safeValue)
    if (key.startsWith('notification.')) {
      await updateNotificationConfig(e)
    }
    return result
  })

  ipcMain.handle('engine:postSyncList', async (_event, repoName: string) => {
    return e.postSyncList(assertString(repoName, 'repoName'))
  })

  ipcMain.handle('engine:postSyncAdd', async (_event, repoName: string, cmdName: string, cmd: string) => {
    return e.postSyncAdd(assertString(repoName, 'repoName'), assertString(cmdName, 'cmdName'), assertString(cmd, 'cmd'))
  })

  ipcMain.handle('engine:postSyncRemove', async (_event, repoName: string, cmdId: string) => {
    return e.postSyncRemove(assertString(repoName, 'repoName'), assertString(cmdId, 'cmdId'))
  })

  ipcMain.handle('engine:summarize', async (_event, repoName: string) => {
    return e.summarize(assertString(repoName, 'repoName'))
  })

  ipcMain.handle('engine:summarizeRetry', async (_event, repoName: string) => {
    return e.summarizeRetry(assertString(repoName, 'repoName'))
  })

  // --- Agent resolve streaming (fire-and-forget start, push events) ---

  const activeStreams = new Map<string, ReturnType<EngineClient['resolveStream']>>()
  const ipcLog = (msg: string): void => {
    log.debug(`[IPC] ${msg}`)
  }

  ipcMain.on('engine:resolveStream:start', (event, name: string, opts?: { agent?: string; noConfirm?: boolean }) => {
    assertString(name, 'name')
    ipcLog(`resolveStream:start name=${name} opts=${JSON.stringify(opts)}`)
    const existing = activeStreams.get(name)
    if (existing) {
      ipcLog(`killing existing stream for ${name}`)
      existing.kill()
      activeStreams.delete(name)
    }

    const stream = e.resolveStream(name, opts)
    activeStreams.set(name, stream)

    stream.onTick(() => {
      ipcLog(`resolveStream:tick name=${name}`)
      event.sender.send('engine:resolveStream:tick', name)
    })

    stream.onDone((result) => {
      ipcLog(`resolveStream:done name=${name} success=${result.success}`)
      event.sender.send('engine:resolveStream:done', name, result)
      activeStreams.delete(name)
    })

    stream.onError((err: string) => {
      ipcLog(`resolveStream:error name=${name} err=${err}`)
      event.sender.send('engine:resolveStream:error', name, err)
      activeStreams.delete(name)
    })
  })

	ipcMain.handle('engine:readAgentLog', async (_event, repoName: string, sessionId?: string) => {
	    const safeRepoName = assertString(repoName, 'repoName')
	    log.debug('[ipc:readAgentLog]', safeRepoName, 'session:', sessionId || '(none)')
	    const result = await e.readAgentLog(safeRepoName, sessionId)
	    log.debug('[ipc:readAgentLog] result for', repoName, result.events.length, 'events, isRunning:', result.isRunning)
	    return result
	  })

  ipcMain.handle('engine:repoDiff', async (_event, repoName: string) => {
    const safeRepoName = assertString(repoName, 'repoName')
    log.debug('[ipc:repoDiff]', safeRepoName)
    return e.repoDiff(safeRepoName)
  })

  // Initialize notification config from engine
  updateNotificationConfig(e)

  // Cleanup active streams on app quit
  app.on('before-quit', () => {
    for (const [name, stream] of activeStreams) {
      log.debug(`[IPC] cleaning up active stream for ${name} on quit`)
      stream.kill()
      activeStreams.delete(name)
    }
  })
}
