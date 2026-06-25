import { useEffect, useState, useRef, useMemo, useCallback, type DragEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { useRepos } from '@/contexts/RepoContext'
import { useAgents } from '@/contexts/AgentContext'
import { useResolveStream } from '@/hooks/useResolveStream'
import { useHistory } from '@/contexts/HistoryContext'
import { StatusOverviewBar, type FilterStatus, CONFLICT_FAMILY } from '@/components/StatusOverviewBar'
import { RepoRow } from '@/components/RepoRow'
import { RepoDetailPanel } from '@/components/RepoDetailPanel'
import { WorkflowSteps } from '@/components/WorkflowSteps'
import { AgentTerminalDrawer } from '@/components/AgentTerminalDrawer'
import { DiffDrawer } from '@/components/DiffDrawer'
import { EmptyState } from '@/components/EmptyState'
import { LoadingSpinner } from '@/components/LoadingSpinner'
import { Collapsible, CollapsibleContent } from '@/components/ui/collapsible'
import { Button } from '@/components/ui/button'
import { Separator } from '@/components/ui/separator'
import { AddRepoDialog } from '@/components/AddRepoDialog'
import { ScanDialog } from '@/components/ScanDialog'
import { RepoSettingsDialog } from '@/components/RepoSettingsDialog'
import { ConfirmDialog } from '@/components/ConfirmDialog'
import { ErrorBanner } from '@/components/ErrorBanner'
import { engineApi } from '@/lib/api'
import { useAgentLogAutoload } from '@/hooks/useAgentLogAutoload'
import { useAutoExpandWorkflow } from '@/hooks/useAutoExpandWorkflow'
import { useDragDropAdd } from '@/hooks/useDragDropAdd'
import { useHistorySync } from '@/hooks/useHistorySync'
import { useResolveActions } from '@/hooks/useResolveActions'
import { useStartupSync } from '@/hooks/useStartupSync'
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
    scan, addRepo, removeRepo, updateRepoStatus
  } = useRepos()
  const { showToast } = useToastContext()
  const {
    loading: agentLoading, error: agentError, refreshAgents
  } = useAgents()
  const {
    resolveResults, isStreamLive: getIsStreamLive, getStreamEvents,
    startResolve, loadAgentLog, clearResult, streamResults
  } = useResolveStream()
  const {
    records: history, loading: historyLoading,
    loadHistory, clearHistory, updateRecord
  } = useHistory()

  const hasSyncing = useMemo(() => repos.some((r) => r.status === 'syncing'), [repos])

  // Filter state
  const [filterStatus, setFilterStatus] = useState<FilterStatus>(null)

  // Accordion expand state (extracted to useAutoExpandWorkflow: owns the set,
  // the toggle, and the auto-expand/collapse-on-workflow-finish side effect).
  const { expandedRepoIds, toggleExpand } = useAutoExpandWorkflow(repos)

  // Local loading state for resolve/accept/reject operations is now owned by
  // useResolveActions (see below); this declaration was removed.

  // Dialog states
  const [showAdd, setShowAdd] = useState(false)
  const [showScan, setShowScan] = useState(false)
  // Confirm dialog state — replaces native confirm()/alert() with a styled,
  // keyboard-accessible dialog that can be unit-tested. Type is the action to
  // perform on confirm.
  const [confirmAction, setConfirmAction] = useState<'remove' | 'clearHistory' | null>(null)
  const [settingsRepo, setSettingsRepo] = useState<string | null>(null)
  const [settingsSection, setSettingsSection] = useState<'all' | 'postSync'>('all')
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

  // Auto-sync on startup (extracted to useStartupSync).
  useStartupSync()

  // Load history with a 30s cache (extracted to useHistorySync; the ref-not-
  // in-deps invariant that prevents the ~80req/s feedback loop lives there).
  useHistorySync(hasSyncing)

  // Auto-load agent logs for repos with active agent resolution
  // (extracted to useAgentLogAutoload). loadAgentLog is owned by useResolveStream
  // above, so it is passed in rather than re-instantiated inside the hook.
  useAgentLogAutoload({ repos, initialized, loadAgentLog })

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

  // Agent resolve / accept / reject / retry-commit handlers + the
  // stream-results side effect (extracted to useResolveActions). startResolve,
  // clearResult, streamResults come from the useResolveStream instance above so
  // there is a single source of truth for stream state.
  const {
    localLoading, handleResolve, handleAccept, handleReject, handleRetryCommit
  } = useResolveActions(setTerminalDrawerRepo, { startResolve, clearResult, streamResults })

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
  // Store the pending remove target name so the ConfirmDialog onConfirm can
  // proceed with the actual removal.
  const pendingRemoveRef = useRef<string | null>(null)

  const handleRemove = useCallback((name: string) => {
    pendingRemoveRef.current = name
    setConfirmAction('remove')
  }, [])

  const doRemove = useCallback(async () => {
    const name = pendingRemoveRef.current
    if (!name || removingRef.current) return
    removingRef.current = name
    setRemovingRepo(name)
    try {
      await removeRepo(name)
    } finally {
      removingRef.current = null
      setRemovingRepo(null)
    }
  }, [removeRepo])

  // Clear history with toast (replaces alert())
  const handleClearHistory = useCallback(() => {
    setConfirmAction('clearHistory')
  }, [])

  const doClearHistory = useCallback(async () => {
    try {
      const res = await engineApi.historyCleanup()
      if (res.success) {
        clearHistory()
        showToast?.(t('dashboard.clearSuccess'), 'success')
      } else {
        showToast?.(res.error || t('dashboard.clearFailed'), 'error')
      }
    } catch (err) {
      showToast?.(t('dashboard.clearFailed'), 'error')
    }
  }, [clearHistory, showToast, t])

  const handleConfirmClose = useCallback(() => {
    setConfirmAction(null)
  }, [])

  const handleConfirm = useCallback(() => {
    if (confirmAction === 'remove') {
      doRemove()
    } else if (confirmAction === 'clearHistory') {
      doClearHistory()
    }
    setConfirmAction(null)
  }, [confirmAction, doRemove, doClearHistory])

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
        <ErrorBanner message={error} onRetry={refresh} />
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
        {loading && repos.length === 0 && <LoadingSpinner />}

        {!loading && repos.length === 0 && (
          <EmptyState
            title={t('repos.emptyTitle')}
            description={t('repos.emptyHint')}
          />
        )}

        <div className="space-y-2">
          {filteredRepos.map((repo) => {
            const isExpanded = expandedRepoIds.has(repo.id)

            return (
              <div key={repo.id}>
                <RepoRow
                  repo={repo}
                  isExpanded={isExpanded}
                  onToggle={toggleExpand}
                  onSync={syncRepo}
                  onRemove={handleRemove}
                  onSettings={(name, section) => { setSettingsRepo(name); setSettingsSection(section ?? 'all') }}
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
        <ErrorBanner message={agentError} onRetry={refreshAgents} />
      )}

      <Separator />

      {/* Sync History Timeline */}
      <div>
        <div
          className={`flex items-center justify-between mb-3 select-none ${
            history.length > 3 ? 'cursor-pointer' : ''
          }`}
          onClick={() => history.length > 3 && setHistoryExpanded((v) => !v)}
        >
          <div className="flex items-center gap-2">
            {history.length > 3 && (
              <span className="text-muted-foreground">
                {historyExpanded ? <ChevronDown size={12} /> : <ChevronRight size={12} />}
              </span>
            )}
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
              onClick={(e) => {
                e.stopPropagation()
                handleClearHistory()
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
        section={settingsSection}
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

      {/* Confirm Dialog — replaces native confirm()/alert() */}
      <ConfirmDialog
        open={confirmAction !== null}
        title={confirmAction === 'remove' ? t('repos.removeTitle') : t('dashboard.clearHistoryTitle')}
        message={confirmAction === 'remove'
          ? t('repos.removeConfirm', { name: pendingRemoveRef.current ?? '' })
          : t('dashboard.clearHistoryConfirm')}
        confirmLabel={t('common.confirm')}
        confirmVariant={confirmAction === 'remove' ? 'destructive' : 'default'}
        onConfirm={handleConfirm}
        onCancel={handleConfirmClose}
      />
    </div>
  )
}
