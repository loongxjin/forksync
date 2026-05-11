/**
 * IPC Handlers — bridges Electron IPC to EngineClient
 *
 * Registers ipcMain.handle() for each engine command so the renderer
 * can invoke them via contextBridge-exposed API.
 */

import { ipcMain, dialog, app, BrowserWindow } from 'electron'
import { t } from './i18n'
import { existsSync, mkdirSync, writeFileSync, unlinkSync, appendFileSync } from 'fs'
import { join, resolve, normalize } from 'path'
import { homedir } from 'os'
import { EngineClient } from './engine'
import { notifySyncResults, updateNotificationConfig } from './notify'
import type { AgentStreamEvent } from '../renderer/src/types/engine'

// --- Input validation helpers ---

function assertString(value: unknown, name: string): string {
  if (typeof value !== 'string' || value.length === 0) {
    throw new Error(`Invalid ${name}: expected non-empty string`)
  }
  if (value.length > 4096) {
    throw new Error(`Invalid ${name}: exceeds maximum length`)
  }
  return value
}

function assertOptionalString(value: unknown, name: string): string | undefined {
  if (value === undefined || value === null) return undefined
  if (typeof value !== 'string') {
    throw new Error(`Invalid ${name}: expected string`)
  }
  if (value.length > 4096) {
    throw new Error(`Invalid ${name}: exceeds maximum length`)
  }
  return value
}

function assertSafePath(value: unknown, name: string): string {
  const str = assertString(value, name)
  const resolved = resolve(normalize(str))
  // Block obvious traversal attempts
  if (str.includes('..')) {
    throw new Error(`Invalid ${name}: path traversal detected`)
  }
  return resolved
}

let engine: EngineClient | null = null

function getEngine(): EngineClient {
  if (!engine) {
    engine = new EngineClient()
  }
  return engine
}

export function registerIpcHandlers(): void {
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
    async (_event, name: string, opts?: { agent?: string; noConfirm?: boolean }) => {
      return e.resolve(assertString(name, 'name'), opts)
    }
  )

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
    // Refresh notification config if notification settings changed
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

  ipcMain.handle('engine:workflowContinue', async (_event, name: string, action: string) => {
    const safeAction = assertString(action, 'action')
    if (!['accept', 'reject', 'abort', 'resolve_with_agent', 'retry_commit', 'continue_manual'].includes(safeAction)) {
      throw new Error(`Invalid action: ${safeAction}`)
    }
    return e.workflowContinue(assertString(name, 'name'), safeAction)
  })

  // --- Agent resolve streaming (fire-and-forget start, push events) ---

  const activeStreams = new Map<string, ReturnType<EngineClient['resolveStream']>>()
  const ipcLogPath = join(homedir(), '.forksync', 'logs', 'electron-resolve-stream.log')
  const ipcLog = (msg: string): void => {
    const ts = new Date().toISOString()
    try { appendFileSync(ipcLogPath, `[${ts}] [IPC] ${msg}\n`) } catch (e) { console.warn('[ipc] failed to write IPC log:', e) }
  }

  ipcMain.on('engine:resolveStream:start', (event, name: string, opts?: { agent?: string; noConfirm?: boolean }) => {
    assertString(name, 'name')
    ipcLog(`resolveStream:start name=${name} opts=${JSON.stringify(opts)}`)
    // Kill any existing stream for this repo
    const existing = activeStreams.get(name)
    if (existing) {
      ipcLog(`killing existing stream for ${name}`)
      existing.kill()
      activeStreams.delete(name)
    }

    const stream = e.resolveStream(name, opts)
    activeStreams.set(name, stream)

    stream.onEvent((ev: AgentStreamEvent) => {
      ipcLog(`resolveStream:event name=${name} type=${ev.t} dataLen=${(ev.d?.length ?? 0)}`)
      event.sender.send('engine:resolveStream:event', name, ev)
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

  ipcMain.handle('engine:readAgentLog', async (_event, repoName: string) => {
    const safeRepoName = assertString(repoName, 'repoName')
    console.log('[ipc:readAgentLog]', safeRepoName)
    const result = await e.readAgentLog(safeRepoName)
    console.log('[ipc:readAgentLog] result for', repoName, result.events.length, 'events, isRunning:', result.isRunning)
    return result
  })

  ipcMain.handle('engine:repoDiff', async (_event, repoName: string) => {
    const safeRepoName = assertString(repoName, 'repoName')
    console.log('[ipc:repoDiff]', safeRepoName)
    return e.repoDiff(safeRepoName)
  })

  ipcMain.handle('app:setAutoLaunch', async (_event, enabled: boolean) => {
    try {
      if (process.platform === 'linux') {
        // Respect $XDG_CONFIG_HOME, fallback to ~/.config
        const configDir = process.env.XDG_CONFIG_HOME || join(homedir(), '.config')
        const autoStartDir = join(configDir, 'autostart')
        const desktopFile = join(autoStartDir, 'forksync.desktop')

        if (enabled) {
          if (!existsSync(autoStartDir)) {
            mkdirSync(autoStartDir, { recursive: true })
          }
          const execPath = process.execPath
          const content = `[Desktop Entry]
Type=Application
Name=ForkSync
Comment=Fork Repository Sync Tool
Exec="${execPath}"
Icon=forksync
Categories=Development;
Terminal=false
Hidden=false
NoDisplay=false
X-GNOME-Autostart-enabled=true
X-KDE-autostart-after=panel
`
          writeFileSync(desktopFile, content, 'utf-8')
        } else {
          if (existsSync(desktopFile)) {
            unlinkSync(desktopFile)
          }
        }
        return { success: true }
      }

      app.setLoginItemSettings({
        openAtLogin: enabled,
        path: process.execPath
      })
      return { success: true }
    } catch (err) {
      return { success: false, error: err instanceof Error ? err.message : String(err) }
    }
  })

  ipcMain.handle('dialog:openDirectory', async () => {
    try {
      const result = await dialog.showOpenDialog({
        properties: ['openDirectory'],
        title: t('mainProcess.selectRepoDir')
      })
      return result
    } catch (err) {
      return {
        canceled: true,
        error: err instanceof Error ? err.message : String(err)
      }
    }
  })

  ipcMain.handle('fs:isGitRepo', async (_event, dirPath: string) => {
    const safePath = assertSafePath(dirPath, 'dirPath')
    return existsSync(join(safePath, '.git'))
  })

  // Linux window control buttons (frameless window)
  ipcMain.on('window:minimize', (event) => {
    const win = BrowserWindow.fromWebContents(event.sender)
    win?.minimize()
  })

  ipcMain.on('window:maximize', (event) => {
    const win = BrowserWindow.fromWebContents(event.sender)
    if (win) {
      if (win.isMaximized()) {
        win.unmaximize()
      } else {
        win.maximize()
      }
    }
  })

  ipcMain.on('window:close', (event) => {
    const win = BrowserWindow.fromWebContents(event.sender)
    win?.close()
  })

  // Initialize notification config from engine
  updateNotificationConfig(e)
}
