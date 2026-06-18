import { useEffect, useRef } from 'react'
import { useHistory } from '@/contexts/HistoryContext'
import type { SyncHistoryRecord } from '@shared/types/engine'

/**
 * useSummaryPolling — re-fetch history while any summary is still generating.
 *
 * Extracted from HomePage. When at least one history record has
 * summaryStatus 'generating' or 'pending', a 5s interval re-loads history so
 * the UI updates when the agent finishes. The interval is cleared the moment
 * no records are pending (and on unmount).
 *
 * NOTE: this hook will be replaced by the unified WebSocket events channel in
 * Phase 3; for now it preserves the existing fixed-interval behavior exactly.
 */
export function useSummaryPolling(history: SyncHistoryRecord[]): void {
  const { loadHistory } = useHistory()
  // Hold the interval in a ref so the effect can start/stop it without
  // re-subscribing on every history change.
  const pollTimerRef = useRef<ReturnType<typeof setInterval> | null>(null)

  useEffect(() => {
    const hasGenerating = history.some(
      (r) => r.summaryStatus === 'generating' || r.summaryStatus === 'pending'
    )
    if (hasGenerating) {
      if (!pollTimerRef.current) pollTimerRef.current = setInterval(loadHistory, 5000)
    } else if (pollTimerRef.current) {
      clearInterval(pollTimerRef.current)
      pollTimerRef.current = null
    }
    return () => {
      if (pollTimerRef.current) {
        clearInterval(pollTimerRef.current)
        pollTimerRef.current = null
      }
    }
  }, [history, loadHistory])
}
