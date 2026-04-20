import { type RepoStatus } from '@/types/engine'
import { useTranslation } from 'react-i18next'
import { CheckCircle2, Loader2, AlertTriangle, Zap, XCircle, Circle } from 'lucide-react'

interface StatusCardProps {
  icon: React.ReactNode
  label: string
  count: number
  color: string
}

export function StatusCard({ icon, label, count, color }: StatusCardProps): JSX.Element {
  return (
    <div className="flex items-center gap-3 rounded-lg border border-border bg-card p-4 shadow-card">
      <div className="flex h-10 w-10 items-center justify-center">{icon}</div>
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
// Status icons (Lucide-based)
// ---------------------------------------------------------------------------

export function StatusIcon({ status, size = 16 }: { status: RepoStatus; size?: number }): JSX.Element {
  switch (status) {
    case 'up_to_date':
      return <CheckCircle2 size={size} className="text-success" />
    case 'sync_needed':
      return <AlertTriangle size={size} className="text-warning" />
    case 'syncing':
      return <Loader2 size={size} className="animate-spin text-warning" />
    case 'conflict':
      return <AlertTriangle size={size} className="text-error" />
    case 'resolving':
      return <Zap size={size} className="text-warning" />
    case 'resolved':
      return <CheckCircle2 size={size} className="text-success" />
    case 'error':
      return <XCircle size={size} className="text-error" />
    case 'unconfigured':
      return <Circle size={size} className="text-muted-foreground" />
    default:
      return <Circle size={size} className="text-muted-foreground" />
  }
}

// ---------------------------------------------------------------------------
// Status color utilities
// ---------------------------------------------------------------------------

export function getStatusColor(status: RepoStatus): string {
  switch (status) {
    case 'up_to_date':
    case 'resolved':
      return 'hsl(var(--success))'
    case 'sync_needed':
    case 'syncing':
    case 'resolving':
      return 'hsl(var(--warning))'
    case 'conflict':
    case 'error':
      return 'hsl(var(--error))'
    default:
      return 'hsl(var(--muted-foreground))'
  }
}

// ---------------------------------------------------------------------------
// Status helpers
// ---------------------------------------------------------------------------

const STATUS_CONFIG: Record<
  RepoStatus,
  { labelKey: string; color: string }
> = {
  up_to_date: { labelKey: 'status.upToDate', color: 'hsl(var(--success))' },
  sync_needed: { labelKey: 'status.syncNeeded', color: 'hsl(var(--warning))' },
  syncing: { labelKey: 'status.syncing', color: 'hsl(var(--warning))' },
  conflict: { labelKey: 'status.conflict', color: 'hsl(var(--error))' },
  resolving: { labelKey: 'status.resolving', color: 'hsl(var(--warning))' },
  resolved: { labelKey: 'status.resolved', color: 'hsl(var(--success))' },
  error: { labelKey: 'status.error', color: 'hsl(var(--error))' },
  unconfigured: { labelKey: 'status.unconfigured', color: 'hsl(var(--muted-foreground))' }
}

export function getStatusConfig(status: RepoStatus) {
  const { t } = useTranslation()
  const config = STATUS_CONFIG[status] ?? STATUS_CONFIG.unconfigured
  return { label: t(config.labelKey), color: config.color }
}
