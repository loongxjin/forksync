import { Badge } from '@/components/ui/badge'
import type { Repo, RepoStatus } from '@/types/engine'
import { getStatusConfig } from '@/components/StatusCard'

interface RepoStatusBadgeProps {
  status: RepoStatus
  className?: string
}

export function RepoStatusBadge({ status, className }: RepoStatusBadgeProps): JSX.Element {
  const config = getStatusConfig(status)

  const variantMap: Record<string, 'success' | 'warning' | 'error' | 'info' | 'muted'> = {
    synced: 'success',
    up_to_date: 'success',
    syncing: 'warning',
    conflict: 'error',
    resolving: 'info',
    resolved: 'info',
    error: 'error',
    unconfigured: 'muted'
  }

  return (
    <Badge variant={variantMap[status] ?? 'muted'} className={className}>
      <span className="mr-1">{config.icon}</span>
      {config.label}
    </Badge>
  )
}

interface RepoRowProps {
  repo: Repo
  onSync: (name: string) => void
  onRemove: (name: string) => void
  onResolve: (name: string) => void
}

export function RepoRow({ repo, onSync, onRemove, onResolve }: RepoRowProps): JSX.Element {
  return (
    <div className="group flex items-start justify-between rounded-lg border border-border bg-card p-4 transition-colors hover:bg-accent/30">
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-2">
          <span className="font-medium">{repo.name}</span>
          <RepoStatusBadge status={repo.status} />
          {repo.branch && (
            <span className="text-xs text-muted-foreground">📋 {repo.branch}</span>
          )}
          {(repo.aheadBy > 0 || repo.behindBy > 0) && (
            <span className="text-xs text-muted-foreground">
              ↑{repo.aheadBy} ↓{repo.behindBy}
            </span>
          )}
        </div>
        {repo.origin && (
          <p className="mt-1 truncate text-xs text-muted-foreground">
            origin: {repo.origin}
          </p>
        )}
        {repo.upstream && (
          <p className="truncate text-xs text-muted-foreground">
            upstream: {repo.upstream}
          </p>
        )}
        {repo.errorMessage && (
          <p className="mt-1 text-xs text-red-500">{repo.errorMessage}</p>
        )}
      </div>

      <div className="flex items-center gap-1 opacity-0 transition-opacity group-hover:opacity-100">
        <button
          onClick={() => onSync(repo.name)}
          className="rounded px-2 py-1 text-xs text-muted-foreground hover:bg-accent hover:text-foreground"
          title="Sync now"
        >
          🔄 Sync
        </button>
        {(repo.status === 'conflict' || repo.status === 'resolved') && (
          <button
            onClick={() => onResolve(repo.name)}
            className="rounded px-2 py-1 text-xs text-muted-foreground hover:bg-accent hover:text-foreground"
            title="Resolve conflicts"
          >
            ⚡ Resolve
          </button>
        )}
        <button
          onClick={() => onRemove(repo.name)}
          className="rounded px-2 py-1 text-xs text-red-400 hover:bg-red-500/10 hover:text-red-500"
          title="Remove"
        >
          ✕
        </button>
      </div>
    </div>
  )
}
