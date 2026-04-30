/**
 * IPC Handlers — bridges Electron IPC to EngineClient
 *
 * Registers ipcMain.handle() for each engine command so the renderer
 * can invoke them via contextBridge-exposed API.
 */

import { ipcMain, dialog, app, BrowserWindow } from 'electron'
import { t } from './i18n'
import { existsSync, mkdirSync, writeFileSync, unlinkSync } from 'fs'
import { join } from 'path'
import { homedir } from 'os'
import { EngineClient } from './engine'
import { notifySyncResults, updateNotificationConfig } from './notify'
import type { AgentStreamEvent } from '../renderer/src/types/engine'

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

  ipcMain.handle('engine:resolveAccept', async (_event, name: string) => {
    return e.resolveAccept(name)
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

  ipcMain.handle('engine:agentReset', async (_event, name: string) => {
    return e.agentReset(name)
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
    const result = await e.configSet(key, value)
    // Refresh notification config if notification settings changed
    if (key.startsWith('notification.')) {
      await updateNotificationConfig(e)
    }
    return result
  })

  ipcMain.handle('engine:postSyncList', async (_event, repoName: string) => {
    return e.postSyncList(repoName)
  })

  ipcMain.handle('engine:postSyncAdd', async (_event, repoName: string, cmdName: string, cmd: string) => {
    return e.postSyncAdd(repoName, cmdName, cmd)
  })

  ipcMain.handle('engine:postSyncRemove', async (_event, repoName: string, cmdId: string) => {
    return e.postSyncRemove(repoName, cmdId)
  })

  ipcMain.handle('engine:summarize', async (_event, repoName: string) => {
    return e.summarize(repoName)
  })

  ipcMain.handle('engine:summarizeRetry', async (_event, repoName: string) => {
    return e.summarizeRetry(repoName)
  })

  ipcMain.handle('engine:workflowContinue', async (_event, name: string, action: string) => {
    return e.workflowContinue(name, action)
  })

  // --- Agent resolve streaming (fire-and-forget start, push events) ---

  const activeStreams = new Map<string, ReturnType<EngineClient['resolveStream']>>()

  ipcMain.on('engine:resolveStream:start', (event, name: string, opts?: { agent?: string; noConfirm?: boolean }) => {
    console.log('[ipc:resolveStream:start]', name, opts)
    // Kill any existing stream for this repo
    const existing = activeStreams.get(name)
    if (existing) {
      console.log('[ipc:resolveStream:start] killing existing stream for', name)
      existing.kill()
      activeStreams.delete(name)
    }

    const stream = e.resolveStream(name, opts)
    activeStreams.set(name, stream)

    stream.onEvent((ev: AgentStreamEvent) => {
      console.log('[ipc:resolveStream:event]', name, ev.t)
      event.sender.send('engine:resolveStream:event', name, ev)
    })

    stream.onDone((result) => {
      console.log('[ipc:resolveStream:done]', name, result.success)
      event.sender.send('engine:resolveStream:done', name, result)
      activeStreams.delete(name)
    })

    stream.onError((err: string) => {
      console.error('[ipc:resolveStream:error]', name, err)
      event.sender.send('engine:resolveStream:error', name, err)
      activeStreams.delete(name)
    })
  })

  ipcMain.handle('engine:readAgentLog', async (_event, repoName: string) => {
    console.log('[ipc:readAgentLog]', repoName)
    return e.readAgentLog(repoName)
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
    return existsSync(join(dirPath, '.git'))
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
