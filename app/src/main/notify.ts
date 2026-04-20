/**
 * Notification helper — Electron-native notifications with click-through
 *
 * Shows system notifications for sync events and navigates to the
 * relevant page when the user clicks the notification.
 */

import { Notification, BrowserWindow } from 'electron'
import { t } from './i18n'
import type { SyncResult } from '../renderer/src/types/engine'
import type { EngineClient } from './engine'

// Module-level config cache — defaults to enabled
let notificationEnabled = true

/**
 * Refresh the notification config cache from the engine.
 * Called at startup and when config changes.
 */
export async function updateNotificationConfig(engine: EngineClient): Promise<void> {
  try {
    const res = await engine.configGet()
    if (res.success && res.data?.Notification) {
      notificationEnabled = res.data.Notification.Enabled
      console.log('[notify] Config refreshed: enabled =', notificationEnabled)
    }
  } catch (err) {
    console.warn('[notify] Failed to read notification config:', err)
  }
}

/**
 * Show Electron notifications for sync results.
 * - Conflicts → navigate to /conflicts on click
 * - Errors → show error detail
 * - Success → informational only
 */
export function notifySyncResults(results: SyncResult[]): void {
  if (!results || results.length === 0) return

  const conflicts = results.filter(
    (r) => r.status === 'conflict' || r.status === 'resolving'
  )
  const errors = results.filter((r) => r.status === 'error')
  const synced = results.filter(
    (r) => r.status === 'up_to_date'
  )

  // Conflict notification — high priority
  if (conflicts.length > 0) {
    const repoNames = conflicts.map((r) => r.repoName).join(', ')
    const totalFiles = conflicts.reduce(
      (sum, r) => sum + (r.conflictFiles?.length ?? r.conflictsFound ?? 0),
      0
    )
    showNotification({
      title: t('notify.conflictsTitle', { count: conflicts.length }),
      body: t('notify.conflictsBody', { names: repoNames, files: totalFiles }),
      navigateTo: '/conflicts'
    })
  }

  // Error notification
  if (errors.length > 0) {
    const repoNames = errors.map((r) => r.repoName).join(', ')
    showNotification({
      title: t('notify.failedTitle', { count: errors.length }),
      body: t('notify.failedBody', { names: repoNames }),
      navigateTo: '/'
    })
  }

  // Success summary (only if no conflicts/errors, to avoid notification spam)
  if (conflicts.length === 0 && errors.length === 0 && synced.length > 0) {
    const totalCommits = synced.reduce((sum, r) => sum + r.commitsPulled, 0)
    if (totalCommits > 0) {
      showNotification({
        title: t('notify.completeTitle'),
        body: t('notify.completeBody', { count: synced.length, pulled: totalCommits }),
        navigateTo: '/'
      })
    }
  }
}

interface NotifyOptions {
  title: string
  body: string
  navigateTo?: string
}

function showNotification(opts: NotifyOptions): void {
  if (!notificationEnabled) {
    console.log('[notify] Suppressed (disabled):', opts.title)
    return
  }

  if (!Notification.isSupported()) {
    console.warn('[notify] Notifications not supported on this platform')
    return
  }

  console.log('[notify] Showing:', opts.title, '-', opts.body)

  const notification = new Notification({
    title: opts.title,
    body: opts.body
  })

  if (opts.navigateTo) {
    notification.on('click', () => {
      const win = BrowserWindow.getAllWindows()[0]
      if (win) {
        if (win.isMinimized()) win.restore()
        win.focus()
        // Send navigation event to renderer
        win.webContents.send('navigate', opts.navigateTo)
      }
    })
  }

  notification.show()
}
