import { useEffect, useState, useRef, useMemo, useCallback, type DragEvent } from 'react'
import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'
import { useRepos } from '@/contexts/RepoContext'
import { useAgents } from '@/contexts/AgentContext'
import { useSettings } from '@/contexts/SettingsContext'
import { useHistory } from '@/contexts/HistoryContext'
import { StatusOverviewBar, type FilterStatus } from '@/components/StatusOverviewBar'
import { RepoRow } from '@/components/RepoRow'
import { ConflictInlinePanel } from '@/components/ConflictInlinePanel'
import { RepoDetailPanel } from '@/components/RepoDetailPanel'
import { Collapsible, CollapsibleContent } from '@/components/ui/collapsible'
import { Button } from '@/components/ui/button'
import { Separator } from '@/components/ui/separator'
import { AddRepoDialog } from '@/components/AddRepoDialog'
import { ScanDialog } from '@/components/ScanDialog'
import { RepoSettingsDialog } from '@/components/RepoSettingsDialog'
import { engineApi } from '@/lib/api'
import type { Repo, RepoStatus, ResolveData, SyncHistoryRecord } from '@/types/engine'
import { isConflictStatus } from '@/lib/utils'
import { RotateCw, RefreshCw, FolderOpen, ChevronDown, ChevronRight, CheckCircle2, Zap, XCircle, Search, Plus } from 'lucide-react'

export function HomePage(): JSX.Element {
  const { t } = useTranslation()
  const {
    repos, scannedRepos, loading, initialized, error, refresh, syncAll, syncRepo,
    scan, addRepo, removeRepo, updateRepoStatus, syncResults, showToast,
    startupSyncDone, markStartupSyncDone
  } = useRepos()
  const {
    resolve, resolveAccept, resolveReject, preferred, loading: agentLoading, error: agentError
  } = useAgents()
  const { engineConfig } = useSettings()
  const {
    records: history, loading: historyLoading, initialized: historyInitialized,
    lastLoadAt, loadHistory, clearHistory, updateRecord
  } = useHistory()

  const hasSyncing = useMemo(() => repos.some((r) => r.status === 'syncing'), [repos])
  const pollTimerRef = useRef<ReturnType<typeof setInterval> | null>(null)
  const syncResultsMountedRef = useRef(false)
  const HISTORY_CACHE_MS = 30000

  // Filter state
  const [filterStatus, setFilterStatus] = useState<FilterStatus>(null)

  // Accordion state
  const [expandedRepoId, setExpandedRepoId] = useState<string | null>(null)

  // Conflict resolution state
  const [resolveResults, setResolveResults] = useState<Record<string, ResolveData>>({})
  const [localLoading, setLocalLoading] = useState<Record<string, boolean>>({})

  // Dialog states
  const [showAdd, setShowAdd] = useState(false)
  const [showScan, setShowScan] = useState(false)
  const [settingsRepo, setSettingsRepo] = useState<string | null>(null)
  const [scanInitialDir, setScanInitialDir] = useState('')

  // Bump when RepoSettingsDialog closes so RepoDetailPanel reloads commands
  const [commandsVersion, setCommandsVersion] = useState(0)

  // Drag-drop state
  const [dragOver, setDragOver] = useState(false)

  // History list expanded: click title to toggle between showing all records vs 3 records
  const [historyExpanded, setHistoryExpanded] = useState(false)

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

  // Load history
  useEffect(() => {
    const now = Date.now()
    const shouldSkip =
      historyInitialized && !hasSyncing && now - lastLoadAt < HISTORY_CACHE_MS
    if (!shouldSkip) loadHistory()
  }, [loadHistory, historyInitialized, lastLoadAt, hasSyncing])

  // Reload history after sync, and populate resolveResults for auto-resolved repos
  useEffect(() => {
    if (!syncResultsMountedRef.current) {
      syncResultsMountedRef.current = true
      return
    }
    // If any sync result has agent resolution data, populate resolveResults
    // so ConflictInlinePanel can show diff, summary and file list.
    const resolvedSyncs = syncResults.filter(
      (r) => r.status === 'resolved' && r.agentResult
    )
    if (resolvedSyncs.length > 0) {
      setResolveResults((prev) => {
        const next = { ...prev }
        for (const sr of resolvedSyncs) {
          next[sr.repoName] = {
            repoId: sr.repoId,
            conflicts: (sr.pendingConfirm ?? []).map((p) => ({ path: p })),
            agentResult: sr.agentResult
          }
        }
        return next
      })
    }
    loadHistory()
  }, [syncResults, loadHistory])

  // Poll for generating summaries
  useEffect(() => {
    const hasGenerating = history.some((r) => r.summaryStatus === 'generating' || r.summaryStatus === 'pending')
    if (hasGenerating) {
      if (!pollTimerRef.current) pollTimerRef.current = setInterval(loadHistory, 5000)
    } else {
      if (pollTimerRef.current) {
        clearInterval(pollTimerRef.current)
        pollTimerRef.current = null
      }
    }
    return () => {
      if (pollTimerRef.current) {
        clearInterval(pollTimerRef.current)
        pollTimerRef.current = null
      }
    }
  }, [history, loadHistory])

  // Status counts
  const statusCounts = repos.reduce<Record<string, number>>((acc, repo) => {
    acc[repo.status] = (acc[repo.status] ?? 0) + 1
    return acc
  }, {})

  // Filtered repos
  const filteredRepos = useMemo(() => {
    if (!filterStatus) return repos
    if (filterStatus === 'up_to_date') {
      return repos.filter((r) => r.status === 'up_to_date')
    }
    return repos.filter((r) => r.status === filterStatus)
  }, [repos, filterStatus])

  // Toggle expand with accordion behavior
  const toggleExpand = useCallback((repoId: string) => {
    setExpandedRepoId((current) => (current === repoId ? null : repoId))
  }, [])

  // Conflict resolution handlers
  const handleResolve = useCallback(async (repo: Repo) => {
    setLocalLoading((prev) => ({ ...prev, [repo.name]: true }))
    try {
      updateRepoStatus(repo.id, 'resolving')
      const noConfirm = engineConfig?.Agent?.ConfirmBeforeCommit === false
      const result = await resolve(repo.name, { agent: preferred || undefined, noConfirm })
      if (result) {
        setResolveResults((prev) => ({ ...prev, [repo.name]: result }))
      }
      await refresh()
      loadHistory()
    } catch (err) {
      // Agent process crashed/timed out — refresh to recover from optimistic 'resolving' status
      await refresh().catch(() => {})
    } finally {
      setLocalLoading((prev) => ({ ...prev, [repo.name]: false }))
    }
  }, [resolve, preferred, updateRepoStatus, refresh, engineConfig, loadHistory])

  const handleAccept = useCallback(async (repoName: string) => {
    setLocalLoading((prev) => ({ ...prev, [repoName]: true }))
    try {
      const result = await resolveAccept(repoName)
      if (!result) return
      setResolveResults((prev) => {
        const next = { ...prev }
        delete next[repoName]
        return next
      })
      await refresh()
      loadHistory()
      // Fire-and-forget AI summarization after resolving conflicts
      if (engineConfig?.Sync?.AutoSummary) {
        engineApi.summarize(repoName).catch(() => {
          // ignore background summary errors
        })
      }
    } finally {
      setLocalLoading((prev) => ({ ...prev, [repoName]: false }))
    }
  }, [resolveAccept, refresh, loadHistory, engineConfig])

  const handleReject = useCallback(async (repoName: string) => {
    setLocalLoading((prev) => ({ ...prev, [repoName]: true }))
    try {
      const ok = await resolveReject(repoName)
      if (!ok) return
      setResolveResults((prev) => {
        const next = { ...prev }
        delete next[repoName]
        return next
      })
      await refresh()
    } finally {
      setLocalLoading((prev) => ({ ...prev, [repoName]: false }))
    }
  }, [resolveReject, refresh])

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

  // Drag-drop state — use counter to prevent flicker when entering child elements
  const dragCounterRef = useRef(0)

  const handleDragEnter = useCallback((e: DragEvent<HTMLDivElement>) => {
    e.preventDefault()
    e.stopPropagation()
    if (e.dataTransfer.types.includes('Files')) {
      dragCounterRef.current++
      setDragOver(true)
    }
  }, [])

  const handleDragOver = useCallback((e: DragEvent<HTMLDivElement>) => {
    e.preventDefault()
    e.stopPropagation()
  }, [])

  const handleDragLeave = useCallback((e: DragEvent<HTMLDivElement>) => {
    e.preventDefault()
    e.stopPropagation()
    dragCounterRef.current--
    if (dragCounterRef.current === 0) setDragOver(false)
  }, [])

  const handleDrop = useCallback(async (e: DragEvent<HTMLDivElement>) => {
    e.preventDefault()
    e.stopPropagation()
    setDragOver(false)

    const files = e.dataTransfer.files
    for (let i = 0; i < files.length; i++) {
      const file = files[i]
      const path = (file as File & { path?: string }).path
      if (!path) continue
      try {
        const isGit = await window.api.isGitRepo(path)
        if (isGit) {
          await addRepo(path)
        } else {
          setScanInitialDir(path)
          setShowScan(true)
        }
      } catch {
        // silent
      }
    }
  }, [addRepo])

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
        onDragEnter={handleDragEnter}
        onDragOver={handleDragOver}
        onDragLeave={handleDragLeave}
        onDrop={handleDrop}
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
            const isExpanded = expandedRepoId === repo.id
            const isConflict = isConflictStatus(repo.status)

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
                    {isConflict ? (
                      <ConflictInlinePanel
                        repo={repo}
                        resolveResult={resolveResults[repo.name] ?? null}
                        onResolve={() => handleResolve(repo)}
                        onAccept={() => handleAccept(repo.name)}
                        onReject={() => handleReject(repo.name)}
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
    </div>
  )
}

function HistoryRow({ record, onRetry }: { record: SyncHistoryRecord; onRetry: (record: SyncHistoryRecord) => void }): JSX.Element {
  const { t } = useTranslation()
  const config = getHistoryConfig(record.status, t)
  const timeAgo = formatTimeAgo(record.createdAt, t)
  const [expanded, setExpanded] = useState(false)
  const [retrying, setRetrying] = useState(false)

  // Reset retrying when summary status changes away from 'failed'
  useEffect(() => {
    if (record.summaryStatus !== 'failed') {
      setRetrying(false)
    }
  }, [record.summaryStatus])

  const handleRetry = (): void => {
    if (retrying) return
    setRetrying(true)
    onRetry(record)
  }

  // shouldShowFull returns true if the summary has 3 or fewer lines.
  const shouldShowFull = (text: string): boolean => {
    return text.split('\n').length <= 3
  }

  // Determine summary display
  const showSummary = record.summaryStatus === 'generating' || record.summaryStatus === 'pending' ||
    (record.summaryStatus === 'done' && record.summary) ||
    record.summaryStatus === 'failed'

  return (
    <div className="rounded-md px-2 py-1.5 text-sm hover:bg-accent/30 transition-colors duration-150">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2 min-w-0">
          <span className="shrink-0">{config.icon}</span>
          <span className="font-medium truncate">{record.repoName}</span>
          <span className="text-muted-foreground">{config.label}</span>
          {record.commitsPulled > 0 && (
            <span className="text-xs text-muted-foreground tabular-nums">+{record.commitsPulled} commits</span>
          )}
          {record.agentUsed && (
            <span className="text-[10px] px-1.5 py-0.5 rounded-md bg-secondary text-secondary-foreground font-mono">
              {record.agentUsed}
            </span>
          )}
          {record.errorMessage && (
            <span className="truncate text-xs text-error" title={record.errorMessage}>
              {record.errorMessage}
            </span>
          )}
        </div>
        <span className="text-xs text-muted-foreground whitespace-nowrap ml-2">{timeAgo}</span>
      </div>

      {/* AI Summary section */}
      {showSummary && (
        <div className="mt-1 ml-6">
          {record.summaryStatus === 'generating' || record.summaryStatus === 'pending' ? (
            <div className="flex items-center gap-2 text-xs text-muted-foreground">
              <span className="inline-block h-2.5 w-2.5 rounded-full bg-primary animate-pulse" />
              {t('summary.generating')}
            </div>
          ) : record.summaryStatus === 'done' && record.summary ? (
            <div className="text-xs text-muted-foreground leading-relaxed">
              {expanded || shouldShowFull(record.summary) ? (
                <>
                  {record.summary}
                  {!shouldShowFull(record.summary) && (
                    <button
                      onClick={() => setExpanded(false)}
                      className="ml-1 text-primary hover:underline"
                    >
                      {t('summary.collapse')}
                    </button>
                  )}
                </>
              ) : (
                <>
                  {record.summary.split('\n').slice(0, 3).join('\n')}...
                  <button
                    onClick={() => setExpanded(true)}
                    className="ml-1 text-primary hover:underline"
                  >
                    {t('summary.expand')}
                  </button>
                </>
              )}
            </div>
          ) : record.summaryStatus === 'failed' ? (
            <div className="flex items-center gap-2 text-xs">
              <span className="text-error">{t('summary.failed')}</span>
              <button
                onClick={handleRetry}
                disabled={retrying}
                className="text-primary hover:underline disabled:opacity-50"
              >
                {retrying ? t('common.processing') : t('summary.retry')}
              </button>
            </div>
          ) : null}
        </div>
      )}
    </div>
  )
}

function getHistoryConfig(status: string, t: TFunction): { icon: React.ReactNode; label: string } {
  switch (status) {
    case 'synced': return { icon: <CheckCircle2 size={14} className="text-success" />, label: t('status.upToDate') }
    case 'up_to_date': return { icon: <CheckCircle2 size={14} className="text-success" />, label: t('status.upToDate') }
    case 'conflict': return { icon: <Zap size={14} className="text-error" />, label: t('status.conflict') }
    case 'error': return { icon: <XCircle size={14} className="text-error" />, label: t('status.error') }
    default: return { icon: <span className="text-muted-foreground text-xs">•</span>, label: status }
  }
}

function formatTimeAgo(dateStr: string | null, t: TFunction): string {
  if (!dateStr) return ''
  const date = new Date(dateStr)
  const now = new Date()
  const seconds = Math.floor((now.getTime() - date.getTime()) / 1000)

  if (seconds < 60) return t('dashboard.justNow')
  if (seconds < 3600) return t('dashboard.minutesAgo', { count: Math.floor(seconds / 60) })
  if (seconds < 86400) return t('dashboard.hoursAgo', { count: Math.floor(seconds / 3600) })
  return t('dashboard.daysAgo', { count: Math.floor(seconds / 86400) })
}
