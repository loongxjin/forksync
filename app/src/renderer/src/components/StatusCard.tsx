import { type RepoStatus } from '@/types/engine'

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
  { icon: string; label: string; color: string }
> = {
  synced: { icon: '🟢', label: 'Synced', color: '#22c55e' },
  syncing: { icon: '🟡', label: 'Syncing', color: '#eab308' },
  conflict: { icon: '🔴', label: 'Conflict', color: '#ef4444' },
  resolving: { icon: '🟠', label: 'Resolving', color: '#f97316' },
  resolved: { icon: '✅', label: 'Resolved', color: '#22c55e' },
  error: { icon: '❌', label: 'Error', color: '#ef4444' },
  unconfigured: { icon: '⚪', label: 'Unconfigured', color: '#6b7280' },
  up_to_date: { icon: '⚪', label: 'Up to date', color: '#6b7280' }
}

export function getStatusConfig(status: RepoStatus) {
  return STATUS_CONFIG[status] ?? STATUS_CONFIG.unconfigured
}
