import { useCallback } from 'react'
import { engineApi } from '../lib/api'
import { useSettings } from '../contexts/SettingsContext'
import { useHistory } from '../contexts/HistoryContext'

/**
 * Hook for triggering fire-and-forget AI summarization after sync.
 *
 * Centralizes the AutoSummary config check so callers don't need to
 * repeat the engineConfig?.Sync?.AutoSummary guard.
 *
 * After the summarize request resolves, it refreshes the history list so
 * the newly written record (pending → done) shows up in the UI without
 * needing a page reload. HistoryProvider sits above RepoProvider in the
 * tree (see App.tsx), so this hook can safely call useHistory from within
 * RepoContext consumers.
 */
export function useAutoSummarize() {
  const { engineConfig } = useSettings()
  const { loadHistory } = useHistory()

  const triggerSummarize = useCallback(
    (repoName: string) => {
      if (!engineConfig?.Sync?.AutoSummary) return
      engineApi
        .summarize(repoName)
        .then(() => loadHistory())
        .catch(() => {
          // ignore background summary errors
        })
    },
    [engineConfig?.Sync?.AutoSummary, loadHistory]
  )

  return { triggerSummarize }
}
