import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import type { RepoStatus } from '@/types/engine'
import { CheckCircle2, Loader2, AlertTriangle, XCircle, ArrowDownToLine, PauseCircle } from 'lucide-react'

export type FilterStatus = RepoStatus | null

interface StatusOverviewBarProps {
  counts: Record<string, number>
  activeFilter: FilterStatus
  onFilterChange: (status: FilterStatus) => void
}

interface StatusItem {
  key: RepoStatus
  labelKey: string
  colorClass: string
  activeClass: string
  icon: React.ReactNode
}

// Conflict-family statuses are grouped under the "conflict" bar item.
export const CONFLICT_FAMILY: RepoStatus[] = ['conflict', 'resolving', 'resolved']

const STATUS_ITEMS: StatusItem[] = [
  {
    key: 'up_to_date',
    labelKey: 'status.upToDate',
    colorClass: 'text-success',
    activeClass: 'bg-success-muted text-success border-success/20',
    icon: <CheckCircle2 size={14} />
  },
  {
    key: 'sync_needed',
    labelKey: 'status.syncNeeded',
    colorClass: 'text-warning',
    activeClass: 'bg-warning-muted text-warning border-warning/20',
    icon: <ArrowDownToLine size={14} />
  },
  {
    key: 'syncing',
    labelKey: 'status.syncing',
    colorClass: 'text-warning',
    activeClass: 'bg-warning-muted text-warning border-warning/20',
    icon: <Loader2 size={14} className="animate-spin" />
  },
  {
    key: 'waiting',
    labelKey: 'status.waiting',
    colorClass: 'text-warning',
    activeClass: 'bg-warning-muted text-warning border-warning/20',
    icon: <PauseCircle size={14} />
  },
  {
    key: 'conflict',
    labelKey: 'status.conflict',
    colorClass: 'text-error',
    activeClass: 'bg-error-muted text-error border-error/20',
    icon: <AlertTriangle size={14} />
  },
  {
    key: 'error',
    labelKey: 'status.error',
    colorClass: 'text-error',
    activeClass: 'bg-error-muted text-error border-error/20',
    icon: <XCircle size={14} />
  }
]

export function StatusOverviewBar({ counts, activeFilter, onFilterChange }: StatusOverviewBarProps): JSX.Element {
  const { t } = useTranslation()

  const getCount = (key: RepoStatus): number => {
    if (key === 'conflict') {
      // Sum up all conflict-family statuses (conflict + resolving + resolved)
      return CONFLICT_FAMILY.reduce((sum, s) => sum + (counts[s] ?? 0), 0)
    }
    return counts[key] ?? 0
  }

  const handleClick = (key: RepoStatus): void => {
    if (activeFilter === key) {
      onFilterChange(null)
    } else {
      onFilterChange(key)
    }
  }

  return (
    <div className="flex flex-wrap items-center gap-1.5">
      {STATUS_ITEMS.map((item) => {
        const count = getCount(item.key)
        const isActive = activeFilter === item.key
        const isZero = count === 0

        return (
          <button
            key={item.key}
            onClick={() => handleClick(item.key)}
            disabled={isZero}
            className={cn(
              'press-scale flex items-center gap-1.5 rounded-lg border px-3 py-1.5 text-sm transition-all duration-150',
              isActive
                ? item.activeClass
                : 'border-transparent text-muted-foreground hover:bg-accent hover:text-foreground',
              isZero && !isActive && 'opacity-30 cursor-not-allowed'
            )}
          >
            <span className={cn(isActive ? '' : 'opacity-60')}>{item.icon}</span>
            <span className={cn('font-semibold tabular-nums', !isZero && !isActive && item.colorClass)}>{count}</span>
            <span className="text-xs">{t(item.labelKey)}</span>
          </button>
        )
      })}
    </div>
  )
}
