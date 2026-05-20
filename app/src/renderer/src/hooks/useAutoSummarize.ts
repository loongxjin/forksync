import { useCallback } from 'react'
import { engineApi } from '../lib/api'
import { useSettings } from '../contexts/SettingsContext'

/**
 * Hook for triggering fire-and-forget AI summarization after sync.
 *
 * Centralizes the AutoSummary config check so callers don't need to
 * repeat the engineConfig?.Sync?.AutoSummary guard.
 */
export function useAutoSummarize() {
  const { engineConfig } = useSettings()

  const triggerSummarize = useCallback(
    (repoName: string) => {
      if (engineConfig?.Sync?.AutoSummary) {
        engineApi.summarize(repoName).catch(() => {
          // ignore background summary errors
        })
      }
    },
    [engineConfig?.Sync?.AutoSummary]
  )

  return { triggerSummarize }
}
