import { useEffect } from 'react'
import { useRepos } from '@/contexts/RepoContext'
import { useAgents } from '@/contexts/AgentContext'
import { StatusCard, getStatusConfig } from '@/components/StatusCard'
import { AgentStatusBadge } from '@/components/AgentStatusBadge'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Separator } from '@/components/ui/separator'
import type { Repo, RepoStatus } from '@/types/engine'

export function Dashboard(): JSX.Element {
  const { repos, loading, initialized, error, refresh, syncAll } = useRepos()
  const { agents, preferred, sessions, initialized: agentsInitialized, refreshAgents, refreshSessions } = useAgents()

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

      {/* Recent Activity */}
      <div className="rounded-lg border border-border bg-card p-4">
        <h3 className="mb-3 text-sm font-medium text-muted-foreground">Recent Activity</h3>
        {repos.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            No repositories yet. Go to Repos to add one.
          </p>
        ) : (
          <div className="space-y-2">
            {repos
              .filter((r) => r.lastSync)
              .sort(
                (a, b) =>
                  new Date(b.lastSync!).getTime() - new Date(a.lastSync!).getTime()
              )
              .slice(0, 8)
              .map((repo) => (
                <RepoActivityRow key={repo.id} repo={repo} />
              ))}
            {repos.every((r) => !r.lastSync) && (
              <p className="text-sm text-muted-foreground">No sync activity yet.</p>
            )}
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

function RepoActivityRow({ repo }: { repo: Repo }): JSX.Element {
  const config = getStatusConfig(repo.status)
  const timeAgo = formatTimeAgo(repo.lastSync)

  return (
    <div className="flex items-center justify-between rounded-md px-2 py-1.5 text-sm">
      <div className="flex items-center gap-2">
        <span>{config.icon}</span>
        <span className="font-medium">{repo.name}</span>
        <span className="text-xs text-muted-foreground">
          {repo.status === 'synced' || repo.status === 'up_to_date'
            ? `synced${repo.behindBy ? `, ${repo.behindBy} behind` : ''}`
            : repo.status}
        </span>
      </div>
      <span className="text-xs text-muted-foreground">{timeAgo}</span>
    </div>
  )
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
