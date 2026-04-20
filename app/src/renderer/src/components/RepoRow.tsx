import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import type { Repo, RepoStatus } from '@/types/engine'
import { getStatusConfig, StatusIcon, getStatusColor } from '@/components/StatusCard'
import { IDEOpenButton } from '@/components/IDEOpenButton'
import { isConflictStatus, cn } from '@/lib/utils'
import { ChevronRight, Settings, RotateCw, Trash2 } from 'lucide-react'

interface RepoStatusBadgeProps {
  status: RepoStatus
  className?: string
}

export function RepoStatusBadge({ status, className }: RepoStatusBadgeProps): JSX.Element {
  const config = getStatusConfig(status)

  const variantMap: Record<string, 'success' | 'warning' | 'error' | 'info' | 'muted'> = {
    up_to_date: 'success',
    sync_needed: 'warning',
    syncing: 'warning',
    conflict: 'error',
    resolving: 'info',
    resolved: 'info',
    error: 'error',
    unconfigured: 'muted'
  }

  return (
    <Badge variant={variantMap[status] ?? 'muted'} className={className}>
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
  const isSyncing = repo.status === 'syncing'
  const statusColor = getStatusColor(repo.status)

  return (
    <div
      className={cn(
        'group relative cursor-pointer rounded-lg border border-border bg-card shadow-card',
        'transition-all duration-200 hover:shadow-card-hover hover:-translate-y-px',
        'hover:border-border/80'
      )}
      onClick={(e) => {
        // Don't toggle if clicking action buttons
        const target = e.target as HTMLElement
        if (target.closest('[data-action]')) return
        onToggle()
      }}
    >
      {/* Left status indicator bar */}
      <div
        className="absolute left-0 top-2 bottom-2 w-[3px] rounded-l-lg transition-colors"
        style={{ backgroundColor: statusColor }}
      />

      <div className="p-4 pl-5">
        {/* Row 1: name, branch, status, ahead/behind */}
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2 min-w-0 flex-1">
            <ChevronRight
              size={14}
              className={cn(
                'shrink-0 text-muted-foreground transition-transform duration-200',
                isExpanded && 'rotate-90'
              )}
            />
            <span className="font-semibold text-foreground truncate">{repo.name}</span>
            {repo.branch && (
              <code className="rounded bg-secondary px-1.5 py-0.5 text-xs text-muted-foreground font-mono">
                {repo.branch}
              </code>
            )}
            <RepoStatusBadge status={repo.status} />
            {(repo.aheadBy > 0 || repo.behindBy > 0) && (
              <span className="text-xs text-muted-foreground whitespace-nowrap tabular-nums">
                {repo.aheadBy > 0 && `↑${repo.aheadBy}`}
                {repo.behindBy > 0 && ` ↓${repo.behindBy}`}
              </span>
            )}
          </div>
        </div>

        {/* Row 2: origin URL + action buttons */}
        <div className="flex items-center justify-between mt-1.5">
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

          <div className="flex items-center gap-0.5 opacity-30 transition-opacity duration-150 group-hover:opacity-100 ml-2 shrink-0">
            <button
              data-action
              onClick={() => onSettings(repo.name)}
              className="press-scale rounded-md p-1.5 text-muted-foreground hover:bg-accent hover:text-foreground"
              title={t('postSync.settings')}
            >
              <Settings size={14} />
            </button>
            <IDEOpenButton repoPath={repo.path} />
            {!isConflict && !isSyncing && (
              <button
                data-action
                onClick={() => onSync(repo.name)}
                className="press-scale rounded-md p-1.5 text-muted-foreground hover:bg-accent hover:text-foreground"
                title={t('repos.syncNow')}
              >
                <RotateCw size={14} />
              </button>
            )}
            <button
              data-action
              onClick={() => onRemove(repo.name)}
              className="press-scale rounded-md p-1.5 text-muted-foreground hover:bg-error-muted hover:text-error"
              title={t('repos.remove')}
            >
              <Trash2 size={14} />
            </button>
          </div>
        </div>
      </div>
    </div>
  )
}


