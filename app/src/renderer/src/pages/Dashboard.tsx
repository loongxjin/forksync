import { useEffect, useState, useRef, useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { useRepos } from '@/contexts/RepoContext'
import { useAgents } from '@/contexts/AgentContext'
import { useSettings } from '@/contexts/SettingsContext'
import { useHistory } from '@/contexts/HistoryContext'
import { StatusCard, getStatusConfig } from '@/components/StatusCard'
import { AgentStatusBadge } from '@/components/AgentStatusBadge'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Separator } from '@/components/ui/separator'
import { engineApi } from '@/lib/api'
import type { Repo, RepoStatus, SyncHistoryRecord } from '@/types/engine'
import type { TFunction } from 'i18next'

export function Dashboard(): JSX.Element {
  const { t } = useTranslation()
  const { repos, loading, initialized, error, refresh, syncAll, syncResults, startupSyncDone, markStartupSyncDone, showToast } = useRepos()
  const { agents, preferred, sessions, initialized: agentsInitialized, refreshAgents, refreshSessions } = useAgents()
  const { engineConfig } = useSettings()
  const { records: history, loading: historyLoading, initialized: historyInitialized, lastLoadAt, loadHistory, clearHistory } = useHistory()
  const hasSyncing = useMemo(() => repos.some((r) => r.status === 'syncing'), [repos])
  const pollTimerRef = useRef<ReturnType<typeof setInterval> | null>(null)
  const syncResultsMountedRef = useRef(false)
  const HISTORY_CACHE_MS = 30000 // 30 seconds

  useEffect(() => {
    if (!initialized) {
      refresh()
    }
  }, [initialized, refresh])

  useEffect(() => {
    if (!agentsInitialized) {
      refreshAgents()
      refreshSessions()
    }
  }, [agentsInitialized, refreshAgents, refreshSessions])

  // Auto-sync on startup (once per app session, only when config enables it)
  useEffect(() => {
    if (
      !startupSyncDone &&
      initialized &&
      repos.length > 0 &&
      engineConfig?.Sync?.SyncOnStartup
    ) {
      markStartupSyncDone()
      syncAll()
    }
  }, [initialized, repos.length, engineConfig, syncAll, startupSyncDone, markStartupSyncDone])

  // Load history on mount: skip if cached recently and no sync is in progress
  useEffect(() => {
    const now = Date.now()
    const shouldSkip =
      historyInitialized &&
      !hasSyncing &&
      history.length > 0 &&
      now - lastLoadAt < HISTORY_CACHE_MS

    if (!shouldSkip) {
      loadHistory()
    }
  }, [loadHistory, historyInitialized, history.length, lastLoadAt, hasSyncing])

  // Reload history after sync operations complete
  useEffect(() => {
    if (!syncResultsMountedRef.current) {
      syncResultsMountedRef.current = true
      return
    }
    loadHistory()
  }, [syncResults, loadHistory])

  // Poll for generating summaries.
  // Re-evaluates whenever history changes: starts polling when generating items exist,
  // stops when all summaries are complete. The pollTimerRef guard prevents duplicate intervals.
  useEffect(() => {
    const hasGenerating = history.some((r) => r.summaryStatus === 'generating' || r.summaryStatus === 'pending')
    if (hasGenerating) {
      if (!pollTimerRef.current) {
        pollTimerRef.current = setInterval(loadHistory, 5000)
      }
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

  // Count repos by status
  const statusCounts = repos.reduce<Record<RepoStatus, number>>(
    (acc, repo) => {
      acc[repo.status] = (acc[repo.status] ?? 0) + 1
      return acc
    },
    {} as Record<RepoStatus, number>
  )

  const conflictRepos = repos.filter(
    (r) => r.status === 'conflict' || r.status === 'resolving' || r.status === 'resolved'
  )
  const activeSessions = sessions.filter((s) => s.status === 'active')

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h2 className="text-xl font-semibold">{t('dashboard.title')}</h2>
        <Button onClick={syncAll} disabled={loading} size="sm">
          {loading ? t('dashboard.syncing') : t('dashboard.syncAll')}
        </Button>
      </div>

      {error && (
        <div className="rounded-md border border-destructive/50 bg-destructive/10 p-3 text-sm text-destructive">
          {error}
        </div>
      )}

      {/* Status Cards */}
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-4">
        <StatusCard
          icon="🟢"
          label={t('dashboard.synced')}
          count={(statusCounts['synced'] ?? 0) + (statusCounts['up_to_date'] ?? 0)}
          color="#22c55e"
        />
        <StatusCard
          icon="🔴"
          label={t('dashboard.conflict')}
          count={statusCounts['conflict'] ?? 0}
          color="#ef4444"
        />
        <StatusCard
          icon="🟡"
          label={t('dashboard.syncing_status')}
          count={statusCounts['syncing'] ?? 0}
          color="#eab308"
        />
        <StatusCard
          icon="❌"
          label={t('dashboard.error_status')}
          count={statusCounts['error'] ?? 0}
          color="#ef4444"
        />
      </div>

      <Separator />

      {/* Agent Status */}
      <div className="rounded-lg border border-border bg-card p-4">
        <h3 className="mb-2 text-sm font-medium text-muted-foreground">{t('dashboard.agentStatus')}</h3>
        <AgentStatusBadge agents={agents} preferred={preferred} />
        {activeSessions.length > 0 && (
          <p className="mt-2 text-xs text-muted-foreground">
            {t('dashboard.activeSessions', { count: activeSessions.length })}
          </p>
        )}
      </div>

      <Separator />

      {/* Sync History Timeline */}
      <div className="rounded-lg border border-border bg-card p-4">
        <div className="flex items-center justify-between mb-3">
          <h3 className="text-sm font-medium text-muted-foreground">{t('dashboard.syncHistory')}</h3>
          {history.length > 0 && (
            <Button variant="outline" size="sm" className="text-xs h-7" disabled={historyLoading}
              onClick={async () => {
                if (confirm(t('dashboard.clearHistoryConfirm'))) {
                  const res = await engineApi.historyCleanup()
                  if (res.success) {
                    clearHistory()
                  } else {
                    alert(res.error || t('dashboard.clearFailed'))
                  }
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
          <div className="space-y-1">
            {history.map((record) => (
              <HistoryRow key={record.id} record={record} onRetry={loadHistory} showToast={showToast} />
            ))}
          </div>
        )}
      </div>

      {/* Conflict Alert */}
      {conflictRepos.length > 0 && (
        <div className="rounded-lg border border-red-500/30 bg-red-500/5 p-4">
          <h3 className="text-sm font-medium text-red-500">
            {t('dashboard.conflictAlert', { count: conflictRepos.length })}
          </h3>
          <div className="mt-2 space-y-1">
            {conflictRepos.map((repo) => (
              <div key={repo.id} className="flex items-center gap-2 text-sm">
                <span>{getStatusConfig(repo.status).icon}</span>
                <span>{repo.name}</span>
                <Badge variant={repo.status === 'conflict' ? 'error' : 'warning'}>
                  {repo.status}
                </Badge>
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  )
}

function HistoryRow({ record, onRetry, showToast }: { record: SyncHistoryRecord; onRetry: () => void; showToast: (message: string, type?: 'info' | 'success' | 'warning' | 'error') => void }): JSX.Element {
  const { t } = useTranslation()
  const config = getHistoryStatusConfig(record.status)
  const timeAgo = formatTimeAgo(record.createdAt, t)
  const [expanded, setExpanded] = useState(false)

  const handleRetry = async (): Promise<void> => {
    try {
      await engineApi.summarizeRetry(record.repoName)
      onRetry()
    } catch {
      showToast(t('toast.summaryRetryFailed'), 'error')
    }
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
    <div className="rounded-md px-2 py-1.5 text-sm">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2 min-w-0">
          <span>{config.icon}</span>
          <span className="font-medium truncate">{record.repoName}</span>
          {record.commitsPulled > 0 && (
            <span className="text-xs text-muted-foreground">
              {record.commitsPulled} commit{record.commitsPulled !== 1 ? 's' : ''}
            </span>
          )}
          {record.agentUsed && (
            <Badge variant="info" className="text-[10px] px-1 py-0">
              🤖 {record.agentUsed}
            </Badge>
          )}
          {record.conflictsFound > 0 && (
            <span className="text-xs text-orange-500">
              {record.conflictsFound} conflict{record.conflictsFound !== 1 ? 's' : ''}
            </span>
          )}
          {record.errorMessage && (
            <span className="truncate text-xs text-red-500" title={record.errorMessage}>
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
              <span className="inline-block h-3 w-3 rounded-full bg-primary/50 animate-pulse" />
              {t('summary.generating')}
            </div>
          ) : record.summaryStatus === 'done' && record.summary ? (
            <div className="text-xs text-muted-foreground leading-relaxed">
              <span className="mr-1">📝</span>
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
              <span className="text-red-500">❌ {t('summary.failed')}</span>
              <button
                onClick={handleRetry}
                className="text-primary hover:underline"
              >
                {t('summary.retry')}
              </button>
            </div>
          ) : null}
        </div>
      )}
    </div>
  )
}

function getHistoryStatusConfig(status: string): { icon: string; color: string } {
  switch (status) {
    case 'synced':
      return { icon: '✅', color: '#22c55e' }
    case 'up_to_date':
      return { icon: '—', color: '#6b7280' }
    case 'conflict':
      return { icon: '⚠️', color: '#f97316' }
    case 'error':
      return { icon: '❌', color: '#ef4444' }
    default:
      return { icon: '•', color: '#6b7280' }
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
