import { memo } from 'react'
import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import type { Repo, RepoStatus } from '@shared/types/engine'
import { useStatusConfig, StatusIcon, getStatusColor } from '@/components/StatusCard'
import { IDEOpenButton } from '@/components/IDEOpenButton'
import { isConflictStatus, cn } from '@/lib/utils'
import { ChevronRight, Settings, RotateCw, Trash2, Terminal } from 'lucide-react'

interface RepoStatusBadgeProps {
  status: RepoStatus
  className?: string
}

const VARIANT_MAP: Record<string, 'success' | 'warning' | 'error' | 'info' | 'muted'> = {
  up_to_date: 'success',
  sync_needed: 'warning',
  syncing: 'warning',
  waiting: 'warning',
  conflict: 'error',
  resolving: 'info',
  resolved: 'info',
  error: 'error',
  unconfigured: 'muted'
}

export function RepoStatusBadge({ status, className }: RepoStatusBadgeProps): JSX.Element {
  const config = useStatusConfig(status)

  return (
    <Badge variant={VARIANT_MAP[status] ?? 'muted'} className={className}>
      {config.label}
    </Badge>
  )
}

interface RepoRowProps {
  repo: Repo
  isExpanded: boolean
  onToggle: (repoId: string) => void
  onSync: (name: string) => void
  onRemove: (name: string) => void
  onSettings: (name: string) => void
  /** Whether a remove operation is in progress for this repo */
  removing: boolean
}

// Memoized: the parent (HomePage) re-renders on every 3s status poll, but only
// the few repos whose status changed need to re-render. Stable callbacks
// (toggleExpand, onSync, etc.) keep identity equal across renders.
function RepoRowImpl({ repo, isExpanded, onToggle, onSync, onRemove, onSettings, removing }: RepoRowProps): JSX.Element {
  const { t } = useTranslation()
  const isConflict = isConflictStatus(repo.status)
  const isSyncing = repo.status === 'syncing'
  const statusColor = getStatusColor(repo.status)

  return (
    <div
      role="button"
      tabIndex={0}
      aria-expanded={isExpanded}
      onKeyDown={(e) => {
        if (e.key === 'Enter' || e.key === ' ') {
          e.preventDefault()
          onToggle(repo.id)
        }
      }}
      className={cn(
        'group relative cursor-pointer rounded-lg border border-border bg-card shadow-card w-full text-left',
        'transition-all duration-200 hover:shadow-card-hover hover:-translate-y-px',
        'hover:border-border/80 focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none'
      )}
      onClick={(e) => {
        const target = e.target as HTMLElement
        if (target.closest('[data-action]')) return
        onToggle(repo.id)
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
            {repo.postSyncCommands && repo.postSyncCommands.length > 0 && (
              <button
                data-action
                onClick={() => onSettings(repo.name)}
                className="inline-flex items-center gap-0.5 rounded-full border border-border bg-secondary px-1.5 py-0 text-[10px] font-medium text-muted-foreground hover:bg-accent hover:text-foreground transition-colors tabular-nums"
                title={t('postSync.countBadge', { count: repo.postSyncCommands.length })}
                aria-label={t('postSync.countBadge', { count: repo.postSyncCommands.length })}
              >
                <Terminal size={10} />
                <span>{repo.postSyncCommands.length}</span>
              </button>
            )}
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
              <p className="mt-1 text-xs text-error">{repo.errorMessage}</p>
            )}
          </div>

          <div className="flex items-center gap-0.5 opacity-60 transition-opacity duration-150 group-hover:opacity-100 focus-within:opacity-100 ml-2 shrink-0">
              <button
                data-action
                onClick={() => onSettings(repo.name)}
                className="press-scale rounded-md p-1.5 text-muted-foreground hover:bg-accent hover:text-foreground"
                title={t('postSync.settings')}
                aria-label={t('postSync.settings')}
              >
              <Settings size={14} />
            </button>
            <IDEOpenButton repoPath={repo.path} />
            {!isConflict && (
              <button
                data-action
                onClick={() => onSync(repo.name)}
                disabled={isSyncing}
                className="press-scale rounded-md p-1.5 text-muted-foreground hover:bg-accent hover:text-foreground disabled:opacity-50 disabled:cursor-not-allowed"
                title={isSyncing ? t('repos.syncing') : t('repos.syncNow')}
                aria-label={isSyncing ? t('repos.syncing') : t('repos.syncNow')}
              >
                <RotateCw size={14} className={isSyncing ? 'animate-spin' : ''} />
              </button>
            )}
              <button
                data-action
                onClick={() => onRemove(repo.name)}
                disabled={removing}
                className="press-scale rounded-md p-1.5 text-muted-foreground hover:bg-error-muted hover:text-error disabled:opacity-50 disabled:cursor-not-allowed"
                title={t('repos.remove')}
                aria-label={t('repos.remove')}
              >
              <Trash2 size={14} />
            </button>
          </div>
        </div>
      </div>
    </div>
  )
}

export const RepoRow = memo(RepoRowImpl)


