import { useEffect, useState, useCallback, useRef } from 'react'
import { useRepos } from '@/contexts/RepoContext'
import { useAgents } from '@/contexts/AgentContext'
import { useSettings } from '@/contexts/SettingsContext'
import { StatusCard, getStatusConfig } from '@/components/StatusCard'
import { AgentStatusBadge } from '@/components/AgentStatusBadge'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Separator } from '@/components/ui/separator'
import { engineApi } from '@/lib/api'
import type { Repo, RepoStatus, SyncHistoryRecord } from '@/types/engine'

export function Dashboard(): JSX.Element {
  const { repos, loading, initialized, error, refresh, syncAll, startupSyncDone, markStartupSyncDone } = useRepos()
  const { agents, preferred, sessions, initialized: agentsInitialized, refreshAgents, refreshSessions } = useAgents()
  const { engineConfig } = useSettings()
  const [history, setHistory] = useState<SyncHistoryRecord[]>([])
  const [historyLoading, setHistoryLoading] = useState(false)

  const loadHistory = useCallback(async () => {
    setHistoryLoading(true)
    try {
      const res = await engineApi.history(undefined, 20)
      if (res.success && res.data) {
        setHistory(res.data.records ?? [])
      }
    } catch {
      // history is best-effort
    } finally {
      setHistoryLoading(false)
    }
  }, [])

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

  // Load history on mount and after sync
  useEffect(() => {
    loadHistory()
  }, [loadHistory, loading])

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
        <h2 className="text-xl font-semibold">Dashboard</h2>
        <Button onClick={syncAll} disabled={loading} size="sm">
          {loading ? 'Syncing...' : 'Sync All'}
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
          label="Synced"
          count={(statusCounts['synced'] ?? 0) + (statusCounts['up_to_date'] ?? 0)}
          color="#22c55e"
        />
        <StatusCard
          icon="🔴"
          label="Conflict"
          count={statusCounts['conflict'] ?? 0}
          color="#ef4444"
        />
        <StatusCard
          icon="🟡"
          label="Syncing"
          count={statusCounts['syncing'] ?? 0}
          color="#eab308"
        />
        <StatusCard
          icon="❌"
          label="Error"
          count={statusCounts['error'] ?? 0}
          color="#ef4444"
        />
      </div>

      <Separator />

      {/* Agent Status */}
      <div className="rounded-lg border border-border bg-card p-4">
        <h3 className="mb-2 text-sm font-medium text-muted-foreground">Agent Status</h3>
        <AgentStatusBadge agents={agents} preferred={preferred} />
        {activeSessions.length > 0 && (
          <p className="mt-2 text-xs text-muted-foreground">
            {activeSessions.length} active session{activeSessions.length !== 1 ? 's' : ''}
          </p>
        )}
      </div>

      <Separator />

      {/* Sync History Timeline */}
      <div className="rounded-lg border border-border bg-card p-4">
        <div className="flex items-center justify-between mb-3">
          <h3 className="text-sm font-medium text-muted-foreground">Sync History</h3>
          {history.length > 0 && (
            <Button variant="outline" size="sm" className="text-xs h-7" disabled={historyLoading}
              onClick={async () => {
                if (confirm('Clear all sync history?')) {
                  const res = await engineApi.historyCleanup()
                  if (res.success) {
                    setHistory([])
                  } else {
                    alert(res.error || 'Failed to clear history')
                  }
                }
              }}
            >
              🗑️ Clear
            </Button>
          )}
        </div>
        {historyLoading && history.length === 0 ? (
          <p className="text-sm text-muted-foreground">Loading history...</p>
        ) : history.length === 0 ? (
          <p className="text-sm text-muted-foreground">No sync history yet.</p>
        ) : (
          <div className="space-y-1">
            {history.map((record) => (
              <HistoryRow key={record.id} record={record} />
            ))}
          </div>
        )}
      </div>

      {/* Conflict Alert */}
      {conflictRepos.length > 0 && (
        <div className="rounded-lg border border-red-500/30 bg-red-500/5 p-4">
          <h3 className="text-sm font-medium text-red-500">
            {conflictRepos.length} repo{conflictRepos.length !== 1 ? 's' : ''} with conflicts
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

function HistoryRow({ record }: { record: SyncHistoryRecord }): JSX.Element {
  const config = getHistoryStatusConfig(record.status)
  const timeAgo = formatTimeAgo(record.createdAt)

  return (
    <div className="flex items-center justify-between rounded-md px-2 py-1.5 text-sm">
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

function formatTimeAgo(dateStr: string | null): string {
  if (!dateStr) return ''
  const date = new Date(dateStr)
  const now = new Date()
  const seconds = Math.floor((now.getTime() - date.getTime()) / 1000)

  if (seconds < 60) return 'just now'
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m ago`
  if (seconds < 86400) return `${Math.floor(seconds / 3600)}h ago`
  return `${Math.floor(seconds / 86400)}d ago`
}
