import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import type { Repo, RepoStatus } from '@/types/engine'
import { getStatusConfig } from '@/components/StatusCard'
import { IDEOpenButton } from '@/components/IDEOpenButton'
import { isConflictStatus } from '@/lib/utils'

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
  isExpanded: boolean
  onToggle: () => void
  onSync: (name: string) => void
  onRemove: (name: string) => void
  onSettings: (name: string) => void
}

export function RepoRow({ repo, isExpanded, onToggle, onSync, onRemove, onSettings }: RepoRowProps): JSX.Element {
  const { t } = useTranslation()
  const isConflict = isConflictStatus(repo.status)

  return (
    <div
      className="group cursor-pointer rounded-lg border border-border bg-card transition-colors hover:bg-accent/30"
      onClick={(e) => {
        // Don't toggle if clicking action buttons
        const target = e.target as HTMLElement
        if (target.closest('[data-action]')) return
        onToggle()
      }}
    >
      <div className="p-4">
        {/* Row 1: name, branch, status, ahead/behind */}
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2 min-w-0 flex-1">
            <span className="text-sm text-muted-foreground">{isExpanded ? '▾' : '▸'}</span>
            <span className="font-semibold text-foreground truncate">{repo.name}</span>
            {repo.branch && (
              <span className="text-xs text-muted-foreground whitespace-nowrap">{repo.branch}</span>
            )}
            <RepoStatusBadge status={repo.status} />
            {(repo.aheadBy > 0 || repo.behindBy > 0) && (
              <span className="text-xs text-muted-foreground whitespace-nowrap">
                {repo.aheadBy > 0 && `↗${repo.aheadBy}`}
                {repo.behindBy > 0 && ` ↙${repo.behindBy}`}
              </span>
            )}
          </div>
        </div>

        {/* Row 2: origin URL + action buttons */}
        <div className="flex items-center justify-between mt-1">
          <div className="min-w-0 flex-1">
            {repo.origin && (
              <p className="truncate text-xs text-muted-foreground">
                {t('repos.origin')} {repo.origin}
              </p>
            )}
            {repo.upstream && (
              <p className="truncate text-xs text-muted-foreground">
                {t('repos.upstream')} {repo.upstream}
              </p>
            )}
            {repo.errorMessage && (
              <p className="mt-1 text-xs text-red-500">{repo.errorMessage}</p>
            )}
          </div>

          <div className="flex items-center gap-1 opacity-0 transition-opacity group-hover:opacity-100 ml-2 shrink-0">
            <button
              data-action
              onClick={() => onSettings(repo.name)}
              className="rounded px-2 py-1 text-xs text-muted-foreground hover:bg-accent hover:text-foreground"
              title={t('postSync.settings')}
            >
              ⚙️
            </button>
            <IDEOpenButton repoPath={repo.path} />
            {!isConflict && (
              <button
                data-action
                onClick={() => onSync(repo.name)}
                className="rounded px-2 py-1 text-xs text-muted-foreground hover:bg-accent hover:text-foreground"
                title={t('repos.syncNow')}
              >
                ⟳
              </button>
            )}
            <button
              data-action
              onClick={() => onRemove(repo.name)}
              className="rounded px-2 py-1 text-xs text-red-400 hover:bg-red-500/10 hover:text-red-500"
              title={t('repos.remove')}
            >
              🗑
            </button>
          </div>
        </div>
      </div>
    </div>
  )
}
