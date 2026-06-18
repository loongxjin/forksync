import { useCallback, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { engineApi } from '@/lib/api'
import { useRepos } from '@/contexts/RepoContext'
import { useAgents } from '@/contexts/AgentContext'
import { useSettings } from '@/contexts/SettingsContext'
import { useAutoSummarize } from '@/hooks/useAutoSummarize'
import { useHistory } from '@/contexts/HistoryContext'
import { useToastContext } from '@/contexts/ToastContext'
import { useLogger } from '@/hooks/useLogger'
import type { Repo, ApiResponse, ResolveData } from '@shared/types/engine'

/**
 * useResolveActions — the agent-resolve / accept / reject / retry-commit
 * business logic plus its side effects.
 *
 * Extracted from HomePage. Owns:
 *  - localLoading (per-repo spinner state)
 *  - autoConfirmRef (tracks which resolves should auto-summarize on done)
 *  - handleResolve / handleAccept / handleReject / handleRetryCommit
 *  - the stream-results side-effect effect (refresh + loadHistory + summarize)
 *
 * `startResolve`, `clearResult`, and `streamResults` come from the host's
 * useResolveStream instance (passed in, not re-instantiated) so there is a
 * single source of truth for stream state. `onOpenTerminal(repoName)` lets the
 * hook open the terminal drawer without owning UI state.
 */
export function useResolveActions(
  onOpenTerminal: (repoName: string) => void,
  stream: {
    startResolve: (
      repoName: string,
      sessionId: string,
      opts?: { agent?: string; noConfirm?: boolean }
    ) => Promise<void>
    clearResult: (repoName: string) => void
    streamResults: Record<string, ApiResponse<ResolveData> | null>
  }
): {
  localLoading: Record<string, boolean>
  handleResolve: (repo: Repo) => Promise<void>
  handleAccept: (repoName: string) => Promise<void>
  handleReject: (repoName: string) => Promise<void>
  handleRetryCommit: (repoName: string) => Promise<void>
} {
  const { startResolve, clearResult, streamResults } = stream
  const logger = useLogger('useResolveActions')
  const { t } = useTranslation()
  const { updateRepo, refresh } = useRepos()
  const { preferred } = useAgents()
  const { engineConfig } = useSettings()
  const { triggerSummarize } = useAutoSummarize()
  const { loadHistory } = useHistory()
  const { showToast } = useToastContext()

  const [localLoading, setLocalLoading] = useState<Record<string, boolean>>({})

  // Track which repos are being resolved with auto-confirm so the streamResults
  // effect knows to trigger summarization (only for auto-confirm, not pending confirm).
  const autoConfirmRef = useRef<Set<string>>(new Set())

  const handleResolve = useCallback(
    async (repo: Repo) => {
      setLocalLoading((prev) => ({ ...prev, [repo.name]: true }))
      try {
        const noConfirm = engineConfig?.Agent?.ConfirmBeforeCommit === false
        if (noConfirm) {
          autoConfirmRef.current.add(repo.name)
        }

        const wfRes = await engineApi.resolvePrepare(repo.name)
        if (!wfRes.success) {
          showToast?.(wfRes.error ?? t('toast.workflowContinueFailed'), 'error')
          return
        }
        if (wfRes.data?.workflow) {
          updateRepo({
            ...repo,
            status: wfRes.data.status ?? repo.status,
            workflow: wfRes.data.workflow
          })
        }

        // Extract resolve session id from the agent_resolve step so the log can
        // be read by session name (not "newest file in the dir").
        const resolveSessionId =
          wfRes.data?.workflow?.steps?.find((s) => s.step === 'agent_resolve')
            ?.resolveSessionId ?? ''
        clearResult(repo.name)
        await startResolve(repo.name, resolveSessionId, {
          agent: preferred || undefined,
          noConfirm
        })
        onOpenTerminal(repo.name)
      } catch (err) {
        await refresh().catch(() => {})
        showToast?.(t('toast.agentResolveFailed', { message: (err as Error).message }), 'error')
      } finally {
        setLocalLoading((prev) => ({ ...prev, [repo.name]: false }))
      }
    },
    [startResolve, clearResult, preferred, updateRepo, refresh, engineConfig, showToast, onOpenTerminal]
  )

  // Keep refresh in a ref to avoid the effect re-triggering when repos change
  // (refresh depends on state.repos, which changes after refresh() itself runs).
  const refreshRef = useRef(refresh)
  refreshRef.current = refresh

  // Data merging is handled by useResolveStream hook — this effect only handles
  // business side effects (refresh, loadHistory, auto-confirm summarization).
  useEffect(() => {
    let hasNew = false
    for (const [repoName, result] of Object.entries(streamResults)) {
      hasNew = true
      logger.log('stream result for', repoName, 'result:', result ? 'non-null' : 'null')
      setLocalLoading((prev) => ({ ...prev, [repoName]: false }))
      if (result && autoConfirmRef.current.has(repoName)) {
        autoConfirmRef.current.delete(repoName)
        triggerSummarize(repoName)
      }
    }
    if (hasNew) {
      logger.log('calling refresh after stream done')
      refreshRef
        .current()
        .then(() => {
          logger.log('refresh completed after stream done')
        })
        .catch((e) => {
          logger.error('refresh failed after stream done', e)
        })
      loadHistory()
    }
  }, [streamResults, loadHistory, engineConfig, logger, triggerSummarize])

  const handleRetryCommit = useCallback(
    async (repoName: string) => {
      setLocalLoading((prev) => ({ ...prev, [repoName]: true }))
      try {
        // Route through resolveAccept (the accept-commit endpoint), not
        // resolve({accept:true}). resolve()'s opts has no `accept` field, so
        // {accept:true} was silently dropped and the call landed in agent mode
        // (mode = opts.prepare ? 'prepare' : 'agent'), re-running the agent
        // instead of retrying the commit.
        const res = await engineApi.resolveAccept(repoName)
        if (!res.success) {
          showToast?.(res.error ?? t('toast.retryCommitFailed'), 'error')
        } else {
          clearResult(repoName)
        }
        await refresh()
        loadHistory()
      } catch (err) {
        showToast?.(t('toast.retryCommitFailed', { message: (err as Error).message }), 'error')
        await refresh()
      } finally {
        setLocalLoading((prev) => ({ ...prev, [repoName]: false }))
      }
    },
    [refresh, loadHistory, showToast, clearResult]
  )

  const handleAccept = useCallback(
    async (repoName: string) => {
      setLocalLoading((prev) => ({ ...prev, [repoName]: true }))
      try {
        const res = await engineApi.resolveAccept(repoName)
        if (!res.success) {
          showToast?.(res.error ?? t('toast.acceptFailed'), 'error')
        } else {
          clearResult(repoName)
          triggerSummarize(repoName)
        }
        await refresh()
        loadHistory()
      } catch (err) {
        showToast?.(t('toast.acceptFailed', { message: (err as Error).message }), 'error')
        await refresh()
      } finally {
        setLocalLoading((prev) => ({ ...prev, [repoName]: false }))
      }
    },
    [refresh, loadHistory, showToast, clearResult, triggerSummarize]
  )

  const handleReject = useCallback(
    async (repoName: string) => {
      setLocalLoading((prev) => ({ ...prev, [repoName]: true }))
      clearResult(repoName)
      try {
        const res = await engineApi.resolveReject(repoName)
        if (!res.success) {
          showToast?.(res.error ?? t('toast.rejectFailed'), 'error')
        }
        await refresh()
      } catch (err) {
        showToast?.(`Reject failed: ${(err as Error).message}`, 'error')
        await refresh()
      } finally {
        setLocalLoading((prev) => ({ ...prev, [repoName]: false }))
      }
    },
    [refresh, showToast, clearResult]
  )

  return { localLoading, handleResolve, handleAccept, handleReject, handleRetryCommit }
}
