import { useEffect } from 'react'
import { engineApi } from '@/lib/api'
import { useRepos } from '@/contexts/RepoContext'
import { useHistory } from '@/contexts/HistoryContext'

/**
 * useEngineEvents — open the single long-lived /stream/events WebSocket and
 * react to push events instead of polling.
 *
 * On 'ready' (initial connect) and 'repos_changed', refresh the repo list.
 * On 'ready' and 'history_changed', reload history. This replaces the
 * renderer's fixed-interval status (3s), conflict (5s), and history (5s)
 * polls: the engine pushes exactly when state changes, so the renderer does
 * zero polling during idle periods and reacts instantly during sync.
 *
 * Installed once at the App root (below the providers). The socket is closed
 * automatically on app quit (main process); on visibility regain the browser
 * WS reconnects by itself — if it doesn't, the next engineApi call still works
 * (polling is gone but direct fetches on user action remain).
 */
export function useEngineEvents(): void {
  const { refreshSilent } = useRepos()
  const { loadHistory } = useHistory()

  useEffect(() => {
    // Open the stream.
    engineApi.eventsStart()

    const off = engineApi.onEventsTick((type) => {
      if (type === 'ready' || type === 'repos_changed') {
        // refreshSilent (SET_REPOS_SILENT, no guard) instead of refresh
        // (SET_REPOS, guarded by refreshingRef). During sync the engine
        // publishes many repos_changed events in quick succession; refresh's
        // guard would drop them while a slow GET /status is in flight, so
        // workflow progress wouldn't show live.
        refreshSilent()
      }
      if (type === 'ready' || type === 'history_changed') {
        loadHistory()
      }
    })

    return () => {
      off()
      // Closing the socket on unmount prevents duplicate streams if the
      // provider subtree remounts. The main process no-ops if already closed.
      engineApi.eventsStop()
    }
  }, [refreshSilent, loadHistory])
}
