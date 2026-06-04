/**
 * IPC App Handlers — auto-launch, dialogs, filesystem
 */

import { ipcMain, dialog, app } from 'electron'
import { t, reloadTranslations } from './i18n'
import { existsSync, mkdirSync, writeFileSync, unlinkSync, readFileSync } from 'fs'
import { join } from 'path'
import { homedir } from 'os'
import { assertSafePath } from './ipc-engine'

export function registerAppIpcHandlers(): void {
  ipcMain.handle('app:setAutoLaunch', async (_event, enabled: boolean) => {
    try {
      if (process.platform === 'linux') {
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

  ipcMain.handle('locale:change', async (_event, locale: string) => {
    try {
      const configPath = join(app.getPath('userData'), 'locale.txt')
      writeFileSync(configPath, locale, 'utf-8')
      reloadTranslations()
      return { success: true }
    } catch (err) {
      return { success: false, error: err instanceof Error ? err.message : String(err) }
    }
  })
}
