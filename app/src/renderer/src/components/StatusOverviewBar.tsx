import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import type { RepoStatus } from '@/types/engine'

export type FilterStatus = RepoStatus | null

interface StatusOverviewBarProps {
  counts: Record<string, number>
  activeFilter: FilterStatus
  onFilterChange: (status: FilterStatus) => void
}

interface StatusItem {
  key: RepoStatus
  icon: string
  labelKey: string
  colorClass: string
  activeClass: string
}

const STATUS_ITEMS: StatusItem[] = [
  {
    key: 'synced',
    icon: '🟢',
    labelKey: 'status.synced',
    colorClass: 'text-green-600 dark:text-green-400',
    activeClass: 'bg-green-50 dark:bg-green-950/30 border-green-200 dark:border-green-800'
  },
  {
    key: 'syncing',
    icon: '🔄',
    labelKey: 'status.syncing',
    colorClass: 'text-yellow-600 dark:text-yellow-400',
    activeClass: 'bg-yellow-50 dark:bg-yellow-950/30 border-yellow-200 dark:border-yellow-800'
  },
  {
    key: 'conflict',
    icon: '⚡',
    labelKey: 'status.conflict',
    colorClass: 'text-red-600 dark:text-red-400',
    activeClass: 'bg-red-50 dark:bg-red-950/30 border-red-200 dark:border-red-800'
  },
  {
    key: 'error',
    icon: '❌',
    labelKey: 'status.error',
    colorClass: 'text-red-600 dark:text-red-400',
    activeClass: 'bg-red-50 dark:bg-red-950/30 border-red-200 dark:border-red-800'
  }
]

export function StatusOverviewBar({ counts, activeFilter, onFilterChange }: StatusOverviewBarProps): JSX.Element {
  const { t } = useTranslation()

  const getCount = (key: RepoStatus): number => {
    if (key === 'synced') {
      return (counts['synced'] ?? 0) + (counts['up_to_date'] ?? 0)
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
    <div className="flex flex-wrap items-center gap-2 rounded-lg border border-border bg-card p-3">
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
              'flex items-center gap-1.5 rounded-md border px-3 py-1.5 text-sm transition-all',
              isActive
                ? `${item.activeClass} border-b-2`
                : 'border-transparent hover:bg-accent/30',
              isZero && !isActive && 'opacity-40 cursor-not-allowed'
            )}
          >
            <span>{item.icon}</span>
            <span className={cn('font-semibold', !isZero && item.colorClass)}>{count}</span>
            <span className="text-muted-foreground">{t(item.labelKey)}</span>
          </button>
        )
      })}
    </div>
  )
}
