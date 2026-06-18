import { useEffect } from 'react'
import { useRepos } from '@/contexts/RepoContext'
import { useSettings } from '@/contexts/SettingsContext'

/**
 * useStartupSync — trigger SyncAll once per app session on startup.
 *
 * Extracted from HomePage. Fires a single SyncAll the first time the repo list
 * is initialized and non-empty, when SyncOnStartup is enabled in config. The
 * startupSyncDone guard (in RepoContext) ensures it never re-fires across
 * re-renders or navigation between routes.
 */
export function useStartupSync(): void {
  const { initialized, repos, startupSyncDone, markStartupSyncDone, syncAll } = useRepos()
  const { engineConfig } = useSettings()

  useEffect(() => {
    if (!initialized || repos.length === 0 || startupSyncDone) return
    if (engineConfig?.Sync?.SyncOnStartup) {
      markStartupSyncDone()
      syncAll()
    }
  }, [initialized, repos.length, engineConfig, syncAll, startupSyncDone, markStartupSyncDone])
}
