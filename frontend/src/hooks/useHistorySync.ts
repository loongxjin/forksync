import { useEffect, useRef } from 'react'
import { useHistory } from '@/contexts/HistoryContext'

// HISTORY_CACHE_MS throttles history reloads when there is nothing actively
// syncing: after a load, skip re-loading for this long even if the effect
// re-runs (e.g. due to other state changes).
const HISTORY_CACHE_MS = 30000

/**
 * useHistorySync — load history on mount and re-load it while a sync is active,
 * with a 30s cache to avoid a feedback loop.
 *
 * Extracted from HomePage. CRITICAL invariant preserved verbatim: lastLoadAt
 * is read through a ref and intentionally NOT listed in the dependency array.
 * lastLoadAt is updated BY loadHistory itself (HistoryContext sets it on every
 * SET_RECORDS), so putting it in deps creates a tight feedback loop:
 * loadHistory() → lastLoadAt changes → effect re-runs → loadHistory() again.
 * With the old CLI backend each call spawned a ~50ms process that hid the
 * storm; the HTTP backend is ~5ms so the loop ran at ~80 req/s during sync
 * (observed in system.log). The ref lets us apply the 30s cache without
 * re-triggering.
 */
export function useHistorySync(hasSyncing: boolean): void {
  const { historyInitialized, lastLoadAt, loadHistory } = useHistory()

  const lastLoadAtRef = useRef(lastLoadAt)
  lastLoadAtRef.current = lastLoadAt

  useEffect(() => {
    const now = Date.now()
    const shouldSkip =
      historyInitialized && !hasSyncing && now - lastLoadAtRef.current < HISTORY_CACHE_MS
    if (!shouldSkip) loadHistory()
  }, [loadHistory, historyInitialized, hasSyncing])
}
