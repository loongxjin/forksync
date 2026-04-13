import { type RepoStatus } from '@/types/engine'
import { useTranslation } from 'react-i18next'

interface StatusCardProps {
  icon: string
  label: string
  count: number
  color: string
}

export function StatusCard({ icon, label, count, color }: StatusCardProps): JSX.Element {
  return (
    <div className="flex items-center gap-3 rounded-lg border border-border bg-card p-4">
      <span className="text-2xl">{icon}</span>
      <div>
        <p className="text-2xl font-bold" style={{ color }}>
          {count}
        </p>
        <p className="text-xs text-muted-foreground">{label}</p>
      </div>
    </div>
  )
}

// ---------------------------------------------------------------------------
// Status helpers
// ---------------------------------------------------------------------------

const STATUS_CONFIG: Record<
  RepoStatus,
  { icon: string; labelKey: string; color: string }
> = {
  synced: { icon: '🟢', labelKey: 'status.synced', color: '#22c55e' },
  syncing: { icon: '🟡', labelKey: 'status.syncing', color: '#eab308' },
  conflict: { icon: '🔴', labelKey: 'status.conflict', color: '#ef4444' },
  resolving: { icon: '🟠', labelKey: 'status.resolving', color: '#f97316' },
  resolved: { icon: '✅', labelKey: 'status.resolved', color: '#22c55e' },
  error: { icon: '❌', labelKey: 'status.error', color: '#ef4444' },
  unconfigured: { icon: '⚪', labelKey: 'status.unconfigured', color: '#6b7280' },
  up_to_date: { icon: '⚪', labelKey: 'status.upToDate', color: '#6b7280' }
}

export function getStatusConfig(status: RepoStatus) {
  const { t } = useTranslation()
  const config = STATUS_CONFIG[status] ?? STATUS_CONFIG.unconfigured
  return { icon: config.icon, label: t(config.labelKey), color: config.color }
}
