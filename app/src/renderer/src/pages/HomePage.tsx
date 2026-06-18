import { useEffect, useState, useRef, useMemo, useCallback, type DragEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { useRepos } from '@/contexts/RepoContext'
import { useAgents } from '@/contexts/AgentContext'
import { useSettings } from '@/contexts/SettingsContext'
import { useResolveStream } from '@/hooks/useResolveStream'
import { useHistory } from '@/contexts/HistoryContext'
import { StatusOverviewBar, type FilterStatus, CONFLICT_FAMILY } from '@/components/StatusOverviewBar'
import { RepoRow } from '@/components/RepoRow'
import { RepoDetailPanel } from '@/components/RepoDetailPanel'
import { WorkflowSteps } from '@/components/WorkflowSteps'
import { AgentTerminalDrawer } from '@/components/AgentTerminalDrawer'
import { DiffDrawer } from '@/components/DiffDrawer'
import { Collapsible, CollapsibleContent } from '@/components/ui/collapsible'
import { Button } from '@/components/ui/button'
import { Separator } from '@/components/ui/separator'
import { AddRepoDialog } from '@/components/AddRepoDialog'
import { ScanDialog } from '@/components/ScanDialog'
import { RepoSettingsDialog } from '@/components/RepoSettingsDialog'
import { engineApi } from '@/lib/api'
import { useAutoSummarize } from '@/hooks/useAutoSummarize'
import { useDragDropAdd } from '@/hooks/useDragDropAdd'
import { useSummaryPolling } from '@/hooks/useSummaryPolling'
import { useLogger } from '@/hooks/useLogger'
import { useToastContext } from '@/contexts/ToastContext'
import { HistoryRow } from '@/components/HistoryRow'
import type { Repo, SyncHistoryRecord } from '@shared/types/engine'
import { RotateCw, RefreshCw, FolderOpen, ChevronDown, ChevronRight, Search, Plus } from 'lucide-react'

export function HomePage(): JSX.Element {
  const { t } = useTranslation()
  const logger = useLogger('HomePage')
  const {
    repos, scannedRepos, loading, initialized, error, refresh, syncAll, syncRepo,
    scan, addRepo, removeRepo, updateRepoStatus, updateRepo,
    startupSyncDone, markStartupSyncDone
  } = useRepos()
  const { showToast } = useToastContext()
  const {
    preferred, loading: agentLoading, error: agentError
  } = useAgents()
  const {
    resolveResults, isStreamLive: getIsStreamLive, getStreamEvents,
    startResolve, loadAgentLog, clearResult, streamResults
  } = useResolveStream()
  const { engineConfig } = useSettings()
  const { triggerSummarize } = useAutoSummarize()
  const {
    records: history, loading: historyLoading, initialized: historyInitialized,
    lastLoadAt, loadHistory, clearHistory, updateRecord
  } = useHistory()

  const hasSyncing = useMemo(() => repos.some((r) => r.status === 'syncing'), [repos])
  const HISTORY_CACHE_MS = 30000

  // Filter state
  const [filterStatus, setFilterStatus] = useState<FilterStatus>(null)

  // Accordion state — supports multiple expanded repos (for SyncAll)
  const [expandedRepoIds, setExpandedRepoIds] = useState<Set<string>>(new Set())

  // Local loading state for resolve/accept/reject operations
  const [localLoading, setLocalLoading] = useState<Record<string, boolean>>({})

  // Dialog states
  const [showAdd, setShowAdd] = useState(false)
  const [showScan, setShowScan] = useState(false)
  const [settingsRepo, setSettingsRepo] = useState<string | null>(null)
  const [scanInitialDir, setScanInitialDir] = useState('')

  // Bump when RepoSettingsDialog closes so RepoDetailPanel reloads commands
  const [commandsVersion, setCommandsVersion] = useState(0)

  // History list expanded: click title to toggle between showing all records vs 3 records
  const [historyExpanded, setHistoryExpanded] = useState(false)

  // Terminal drawer state
  const [terminalDrawerRepo, setTerminalDrawerRepo] = useState<string | null>(null)

  // Diff drawer state
  const [diffDrawerRepo, setDiffDrawerRepo] = useState<string | null>(null)

  const handleSummaryRetry = useCallback(async (record: SyncHistoryRecord): Promise<void> => {
    // Optimistically update status to 'generating' so the polling kicks in
    updateRecord(record.repoName, { summaryStatus: 'generating', summary: '' })
    try {
      const res = await engineApi.summarizeRetry(record.repoName)
      if (!res.success) {
        updateRecord(record.repoName, { summaryStatus: 'failed', summary: '' })
        showToast?.(res.error ?? t('toast.summaryRetryFailed'), 'error')
      } else {
        loadHistory()
      }
    } catch {
      updateRecord(record.repoName, { summaryStatus: 'failed', summary: '' })
      showToast?.(t('toast.summaryRetryFailed'), 'error')
    }
  }, [updateRecord, loadHistory, showToast, t])

  // Initialize
  useEffect(() => {
    if (!initialized) refresh()
  }, [initialized, refresh])

  // Auto-sync on startup (once per app session)
  useEffect(() => {
    if (!initialized || repos.length === 0 || startupSyncDone) return
    if (engineConfig?.Sync?.SyncOnStartup) {
      markStartupSyncDone()
      syncAll()
    }
  }, [initialized, repos.length, engineConfig, syncAll, startupSyncDone, markStartupSyncDone])

  // Load history.
  //
  // NOTE: lastLoadAt is intentionally read via a ref and NOT listed in the
  // dependency array. lastLoadAt is updated BY loadHistory itself (HistoryContext
  // sets it on every SET_RECORDS), so putting it in deps creates a tight feedback
  // loop: loadHistory() → lastLoadAt changes → this effect re-runs → loadHistory()
  // again. With the old CLI backend each call spawned a ~50ms process that hid the
  // storm; the HTTP backend is ~5ms so the loop runs at ~80 req/s during sync
  // (see system.log). The ref lets us apply the 30s cache without re-triggering.
  const lastLoadAtRef = useRef(lastLoadAt)
  lastLoadAtRef.current = lastLoadAt
  useEffect(() => {
    const now = Date.now()
    const shouldSkip =
      historyInitialized && !hasSyncing && now - lastLoadAtRef.current < HISTORY_CACHE_MS
    if (!shouldSkip) loadHistory()
  }, [loadHistory, historyInitialized, hasSyncing])

  // Track which repos have been auto-loaded to prevent repeated loadAgentLog
  // calls when repos changes (e.g. status poll every 3s during sync).
  const autoLoadedRef = useRef<Set<string>>(new Set())

  // When a new workflow starts, reset auto-load so the next sync/re-resolve
  // picks up the new agent log (STREAM_LOAD from loadAgentLog overwrites old events).
  const lastWorkflowStartRef = useRef<Record<string, string>>({})
  useEffect(() => {
    for (const repo of repos) {
      const startedAt = repo.workflow?.startedAt
      if (startedAt && startedAt !== lastWorkflowStartRef.current[repo.name]) {
        lastWorkflowStartRef.current[repo.name] = startedAt
        autoLoadedRef.current.delete(repo.name)
      }
    }
  }, [repos])

  // Auto-load agent logs for repos with active agent resolution.
  // Guarded by autoLoadedRef to fire only once per workflow.
  useEffect(() => {
    if (!initialized) return
    for (const repo of repos) {
      if (autoLoadedRef.current.has(repo.name)) continue

      // Extract the resolve session id from the agent_resolve step so the
      // log is read by session name (precise), not "newest file in the dir".
      const resolveSessionId = repo.workflow?.steps?.find(
        (s) => s.step === 'agent_resolve'
      )?.resolveSessionId ?? ''

      if (repo.status === 'resolving') {
        autoLoadedRef.current.add(repo.name)
        loadAgentLog(repo.name, resolveSessionId)
        continue
      }
      if (repo.status === 'syncing' && repo.workflow) {
        const agentStep = repo.workflow.steps.find(
          (s) => s.step === 'agent_resolve' && s.status === 'running'
        )
        if (agentStep) {
          autoLoadedRef.current.add(repo.name)
          loadAgentLog(repo.name, resolveSessionId)
        }
      }
    }
  }, [initialized, repos, loadAgentLog])

  // Poll for generating summaries (extracted to useSummaryPolling).
  useSummaryPolling(history)

  // Status counts
  const statusCounts = repos.reduce<Record<string, number>>((acc, repo) => {
    acc[repo.status] = (acc[repo.status] ?? 0) + 1
    return acc
  }, {})

  // Filtered repos
  const filteredRepos = useMemo(() => {
    if (!filterStatus) return repos
    if (filterStatus === 'conflict') {
      // Conflict filter includes all conflict-family statuses
      return repos.filter((r) => (CONFLICT_FAMILY as string[]).includes(r.status))
    }
    return repos.filter((r) => r.status === filterStatus)
  }, [repos, filterStatus])

  // Toggle expand for a single repo
  const toggleExpand = useCallback((repoId: string) => {
    setExpandedRepoIds((prev) => {
      const next = new Set(prev)
      if (next.has(repoId)) {
        next.delete(repoId)
      } else {
        next.add(repoId)
      }
      return next
    })
  }, [])

  // Auto-expand repos with active (running/waiting) workflows during SyncAll.
  // Only adds to the set — never collapses a repo the user manually expanded.
  // Collapsed repos are cleaned up when their workflow finishes.
  useEffect(() => {
    setExpandedRepoIds((prev) => {
      const next = new Set(prev)
      let changed = false
      for (const repo of repos) {
        const wf = repo.workflow
        if (!wf) {
          // No active workflow — remove from auto-expand if it was there
          if (next.has(repo.id)) {
            next.delete(repo.id)
            changed = true
          }
          continue
        }
        if (wf.status === 'running' || wf.status === 'waiting') {
          if (!next.has(repo.id)) {
            next.add(repo.id)
            changed = true
          }
        } else if (wf.status === 'success' || wf.status === 'failed') {
          // Workflow finished — remove from auto-expand
          if (next.has(repo.id)) {
            next.delete(repo.id)
            changed = true
          }
        }
      }
      return changed ? next : prev
    })
  }, [repos])

  // Track which repos are being resolved with auto-confirm so the streamResults
  // effect knows to trigger summarization (only for auto-confirm, not pending confirm).
  const autoConfirmRef = useRef<Set<string>>(new Set())

  // Conflict resolution handlers
  const handleResolve = useCallback(async (repo: Repo) => {
    setLocalLoading((prev) => ({ ...prev, [repo.name]: true }))
    try {
      const noConfirm = engineConfig?.Agent?.ConfirmBeforeCommit === false
      if (noConfirm) {
        autoConfirmRef.current.add(repo.name)
      }

      const wfRes = await engineApi.resolvePrepare(repo.name)
      if (!wfRes.success) {
        showToast?.(wfRes.error ?? 'Workflow continue failed', 'error')
        return
      }
      if (wfRes.data?.workflow) {
        updateRepo({ ...repo, status: wfRes.data.status ?? repo.status, workflow: wfRes.data.workflow })
      }

      // Extract resolve session id from the agent_resolve step so the log can
      // be read by session name (not "newest file in the dir").
      const resolveSessionId = wfRes.data?.workflow?.steps?.find(
        (s) => s.step === 'agent_resolve'
      )?.resolveSessionId ?? ''
      clearResult(repo.name)
      await startResolve(repo.name, resolveSessionId, { agent: preferred || undefined, noConfirm })
      setTerminalDrawerRepo(repo.name)
    } catch (err) {
      await refresh().catch(() => {})
      showToast?.(`Agent resolve failed: ${(err as Error).message}`, 'error')
    } finally {
      setLocalLoading((prev) => ({ ...prev, [repo.name]: false }))
    }
  }, [startResolve, clearResult, preferred, updateRepo, refresh, engineConfig, showToast])

  // Keep refresh in a ref to avoid the effect re-triggering when repos change
  // (refresh depends on state.repos, which changes after refresh() itself runs).
  const refreshRef = useRef(refresh)
  refreshRef.current = refresh

  // Keep refresh in a ref to avoid the effect re-triggering when repos change
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
	      refreshRef.current().then(() => {
        logger.log('refresh completed after stream done')
      }).catch((e) => {
        logger.error('refresh failed after stream done', e)
      })
      loadHistory()
    }
  }, [streamResults, loadHistory, engineConfig])

  const handleRetryCommit = useCallback(async (repoName: string) => {
    setLocalLoading((prev) => ({ ...prev, [repoName]: true }))
    try {
      // Route through resolveAccept (the accept-commit endpoint), not
      // resolve({accept:true}). resolve()'s opts has no `accept` field, so
      // {accept:true} was silently dropped and the call landed in agent mode
      // (mode = opts.prepare ? 'prepare' : 'agent'), re-running the agent
      // instead of retrying the commit.
      const res = await engineApi.resolveAccept(repoName)
      if (!res.success) {
        showToast?.(res.error ?? 'Retry commit failed', 'error')
      } else {
        clearResult(repoName)
      }
      await refresh()
      loadHistory()
    } catch (err) {
      showToast?.(`Retry commit failed: ${(err as Error).message}`, 'error')
      await refresh()
    } finally {
      setLocalLoading((prev) => ({ ...prev, [repoName]: false }))
    }
  }, [refresh, loadHistory, showToast, clearResult])

  const handleAccept = useCallback(async (repoName: string) => {
    setLocalLoading((prev) => ({ ...prev, [repoName]: true }))
    try {
      const res = await engineApi.resolveAccept(repoName)
      if (!res.success) {
        showToast?.(res.error ?? 'Accept failed', 'error')
      } else {
        clearResult(repoName)
        triggerSummarize(repoName)
      }
      await refresh()
      loadHistory()
    } catch (err) {
      showToast?.(`Accept failed: ${(err as Error).message}`, 'error')
      await refresh()
    } finally {
      setLocalLoading((prev) => ({ ...prev, [repoName]: false }))
    }
  }, [refresh, loadHistory, showToast, engineConfig, clearResult])

  const handleReject = useCallback(async (repoName: string) => {
    setLocalLoading((prev) => ({ ...prev, [repoName]: true }))
    clearResult(repoName)
    try {
      const res = await engineApi.resolveReject(repoName)
      if (!res.success) {
        showToast?.(res.error ?? 'Reject failed', 'error')
      }
      await refresh()
    } catch (err) {
      showToast?.(`Reject failed: ${(err as Error).message}`, 'error')
      await refresh()
    } finally {
      setLocalLoading((prev) => ({ ...prev, [repoName]: false }))
    }
  }, [refresh, showToast, clearResult])


  const handleViewTerminal = useCallback((repoName: string) => {
    setTerminalDrawerRepo(repoName)
    // Always re-read the disk log on (re)open — this picks up any events
    // that arrived while the drawer was closed, restarts polling if the
    // agent is still running, and restores resolveResults from the done frame.
    const repo = repos.find((r) => r.name === repoName)
    const sid = repo?.workflow?.steps?.find((s) => s.step === 'agent_resolve')?.resolveSessionId ?? ''
    loadAgentLog(repoName, sid)
  }, [loadAgentLog, repos])

  // Repo actions
  const removingRef = useRef<string | null>(null)
  const [removingRepo, setRemovingRepo] = useState<string | null>(null)

  const handleRemove = useCallback(async (name: string) => {
    if (removingRef.current) return
    if (confirm(t('repos.removeConfirm', { name }))) {
      removingRef.current = name
      setRemovingRepo(name)
      try {
        await removeRepo(name)
      } finally {
        removingRef.current = null
        setRemovingRepo(null)
      }
    }
  }, [removeRepo, t])

  // Drag-drop add: folders dropped onto the repo list become repos (git) or
  // open the scan dialog seeded with the directory (non-git).
  const handleScanNeeded = useCallback(
    (dir: string) => {
      setScanInitialDir(dir)
      setShowScan(true)
    },
    []
  )
  const { dragOver, handlers: dragHandlers } = useDragDropAdd(handleScanNeeded)

  // History display
  const displayHistory = historyExpanded ? history : history.slice(0, 3)

  return (
    <div className="space-y-5">
      {/* Error */}
      {error && (
        <div className="rounded-lg border border-error/30 bg-error-muted p-3 text-sm text-error animate-fade-in">
          {error}
        </div>
      )}

      {/* Status Overview */}
      <StatusOverviewBar
        counts={statusCounts}
        activeFilter={filterStatus}
        onFilterChange={setFilterStatus}
      />

      {/* Repo List */}
      <div
        className={`relative ${dragOver ? 'ring-2 ring-primary ring-offset-2 ring-offset-background rounded-xl' : ''}`}
        onDragEnter={dragHandlers.onDragEnter}
        onDragOver={dragHandlers.onDragOver}
        onDragLeave={dragHandlers.onDragLeave}
        onDrop={dragHandlers.onDrop}
      >
        {dragOver && (
          <div className="absolute inset-0 z-40 flex items-center justify-center rounded-xl bg-primary/5 border-2 border-dashed border-primary/30 animate-fade-in">
            <div className="text-center">
              <FolderOpen size={40} className="mx-auto text-primary/60" />
              <p className="mt-2 text-sm font-medium text-primary">{t('repos.dropOverlay')}</p>
            </div>
          </div>
        )}

        {/* Header */}
        <div className="flex items-center justify-between mb-3">
          <h2 className="text-lg font-semibold">
            {filterStatus
              ? `${t('repos.title')} (${filteredRepos.length}/${repos.length})`
              : `${t('repos.title')} (${repos.length})`}
          </h2>
          <div className="flex gap-1.5">
            <Button variant="ghost" size="sm" onClick={syncAll} disabled={loading}>
              <RotateCw size={14} className="mr-1" />
              {t('dashboard.syncAll')}
            </Button>
            <Button variant="ghost" size="sm" onClick={() => setShowScan(true)}>
              <Search size={14} className="mr-1" />
              {t('repos.scan')}
            </Button>
            <Button variant="ghost" size="sm" onClick={() => setShowAdd(true)}>
              <Plus size={14} className="mr-1" />
              {t('repos.addRepo')}
            </Button>
            <Button variant="outline" size="sm" onClick={refresh} disabled={loading}>
              <RefreshCw size={14} className={loading ? 'mr-1 animate-spin' : 'mr-1'} />
              {t('repos.refresh')}
            </Button>
          </div>
        </div>

        {/* Repo rows */}
        {loading && repos.length === 0 && (
          <div className="py-8 text-center text-sm text-muted-foreground">{t('repos.loading')}</div>
        )}

        {!loading && repos.length === 0 && (
          <div className="py-8 text-center">
            <p className="text-sm text-muted-foreground">{t('repos.emptyTitle')}</p>
            <p className="mt-1 text-sm text-muted-foreground">{t('repos.emptyHint')}</p>
          </div>
        )}

        <div className="space-y-2">
          {filteredRepos.map((repo) => {
            const isExpanded = expandedRepoIds.has(repo.id)

            return (
              <div key={repo.id}>
                <RepoRow
                  repo={repo}
                  isExpanded={isExpanded}
                  onToggle={() => toggleExpand(repo.id)}
                  onSync={syncRepo}
                  onRemove={handleRemove}
                  onSettings={setSettingsRepo}
                  removing={removingRepo === repo.name}
                />
                <Collapsible open={isExpanded}>
                  <CollapsibleContent>
                    {repo.workflow ? (
                      <WorkflowSteps
                        repo={repo}
                        streamEvents={getStreamEvents(repo.name)}
                        isStreamLive={getIsStreamLive(repo.name)}
                        resolveResult={resolveResults[repo.name] ?? null}
                        actions={{
                          onResolveWithAgent: () => handleResolve(repo),
                          onOpenIDE: () => engineApi.ideOpen(repo.path, 'default'),
                          onAbort: () => handleReject(repo.name),
                          onAccept: () => handleAccept(repo.name),
                          onReject: () => handleReject(repo.name),
                          onRetryCommit: () => handleRetryCommit(repo.name),
                          onViewTerminal: () => handleViewTerminal(repo.name),
                          onViewDiff: () => setDiffDrawerRepo(repo.name)
                        }}
                        loading={agentLoading || !!localLoading[repo.name]}
                      />
                    ) : (
                      <RepoDetailPanel
                        repo={repo}
                        onEditCommands={() => setSettingsRepo(repo.name)}
                        commandsVersion={commandsVersion}
                      />
                    )}
                  </CollapsibleContent>
                </Collapsible>
              </div>
            )
          })}
        </div>
      </div>

      {/* Agent error */}
      {agentError && (
        <div className="rounded-lg border border-error/30 bg-error-muted p-3 animate-fade-in">
          <p className="text-sm text-error">{agentError}</p>
        </div>
      )}

      <Separator />

      {/* Sync History Timeline */}
      <div>
        <div
          className="flex items-center justify-between mb-3 cursor-pointer select-none"
          onClick={() => history.length > 3 && setHistoryExpanded((v) => !v)}
        >
          <div className="flex items-center gap-2">
            <span className="text-muted-foreground">
              {history.length > 3 ? (
                historyExpanded ? <ChevronDown size={12} /> : <ChevronRight size={12} />
              ) : (
                <ChevronDown size={12} />
              )}
            </span>
            <h3 className="text-sm font-medium text-muted-foreground">{t('dashboard.syncHistory')}</h3>
            {history.length > 0 && (
              <span className="text-xs text-muted-foreground tabular-nums">({history.length})</span>
            )}
          </div>
          {history.length > 0 && (
            <Button
              variant="outline"
              size="sm"
              className="text-xs h-7"
              disabled={historyLoading}
              onClick={async (e) => {
                e.stopPropagation()
                if (confirm(t('dashboard.clearHistoryConfirm'))) {
                  const res = await engineApi.historyCleanup()
                  if (res.success) clearHistory()
                  else alert(res.error || t('dashboard.clearFailed'))
                }
              }}
            >
              {t('dashboard.clear')}
            </Button>
          )}
        </div>

        {historyLoading && history.length === 0 ? (
          <p className="text-sm text-muted-foreground">{t('dashboard.loadingHistory')}</p>
        ) : history.length === 0 ? (
          <p className="text-sm text-muted-foreground">{t('dashboard.noHistory')}</p>
        ) : (
          <div className="space-y-0.5">
            {displayHistory.map((record) => (
              <HistoryRow key={record.id} record={record} onRetry={handleSummaryRetry} />
            ))}
            {!historyExpanded && history.length > 3 && (
              <button
                onClick={() => setHistoryExpanded(true)}
                className="w-full px-2 py-1.5 text-center text-xs text-muted-foreground hover:text-primary transition-colors"
              >
                ··· {t('home.viewMore', { count: history.length - 3 })} ···
              </button>
            )}
          </div>
        )}
      </div>

      {/* Dialogs */}
      <AddRepoDialog open={showAdd} onClose={() => setShowAdd(false)} onAdd={addRepo} />
      <ScanDialog
        open={showScan}
        onClose={() => { setShowScan(false); setScanInitialDir('') }}
        onScan={scan}
        onAdd={addRepo}
        scannedRepos={scannedRepos}
        loading={loading}
        initialDir={scanInitialDir}
      />
      <RepoSettingsDialog
        repoName={settingsRepo ?? ''}
        open={settingsRepo !== null}
        onClose={() => { setSettingsRepo(null); setCommandsVersion((v) => v + 1) }}
      />

      {/* Agent Terminal Drawer */}
      <AgentTerminalDrawer
        open={terminalDrawerRepo !== null}
        onOpenChange={(open) => {
          if (!open) setTerminalDrawerRepo(null)
        }}
        repoName={terminalDrawerRepo ?? ''}
        events={terminalDrawerRepo ? getStreamEvents(terminalDrawerRepo) : []}
        isLive={terminalDrawerRepo ? getIsStreamLive(terminalDrawerRepo) : false}
      />

      {/* Diff Drawer */}
      <DiffDrawer
        open={diffDrawerRepo !== null}
        onOpenChange={(open) => {
          if (!open) setDiffDrawerRepo(null)
        }}
        repoName={diffDrawerRepo ?? ''}
      />
    </div>
  )
}
