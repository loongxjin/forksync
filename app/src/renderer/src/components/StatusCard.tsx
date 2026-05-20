import { type RepoStatus } from '@shared/types/engine'
import { useTranslation } from 'react-i18next'
import { CheckCircle2, Loader2, AlertTriangle, Zap, XCircle, Circle, PauseCircle } from 'lucide-react'

// ---------------------------------------------------------------------------
// Single source of truth: status → color category
// ---------------------------------------------------------------------------

type ColorCategory = 'success' | 'warning' | 'error' | 'muted'

const STATUS_COLOR_CATEGORY: Record<RepoStatus, ColorCategory> = {
  up_to_date: 'success',
  sync_needed: 'warning',
  syncing: 'warning',
  conflict: 'error',
  resolving: 'warning',
  resolved: 'success',
  waiting: 'warning',
  error: 'error',
  unconfigured: 'muted'
}

/** Map color category to HSL CSS variable */
const CATEGORY_HSL: Record<ColorCategory, string> = {
  success: 'hsl(var(--success))',
  warning: 'hsl(var(--warning))',
  error: 'hsl(var(--error))',
  muted: 'hsl(var(--muted-foreground))'
}

/** Map color category to Tailwind text class */
const CATEGORY_TEXT_CLASS: Record<ColorCategory, string> = {
  success: 'text-success',
  warning: 'text-warning',
  error: 'text-error',
  muted: 'text-muted-foreground'
}

// ---------------------------------------------------------------------------
// Status color utility
// ---------------------------------------------------------------------------

export function getStatusColor(status: RepoStatus): string {
  return CATEGORY_HSL[STATUS_COLOR_CATEGORY[status]] ?? CATEGORY_HSL.muted
}

// ---------------------------------------------------------------------------
// StatusCard component
// ---------------------------------------------------------------------------

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
  const colorClass = CATEGORY_TEXT_CLASS[STATUS_COLOR_CATEGORY[status]] ?? CATEGORY_TEXT_CLASS.muted
  switch (status) {
    case 'up_to_date':
      return <CheckCircle2 size={size} className={colorClass} />
    case 'sync_needed':
      return <AlertTriangle size={size} className={colorClass} />
    case 'syncing':
      return <Loader2 size={size} className={`animate-spin ${colorClass}`} />
    case 'conflict':
      return <AlertTriangle size={size} className={colorClass} />
    case 'resolving':
      return <Zap size={size} className={colorClass} />
    case 'resolved':
      return <CheckCircle2 size={size} className={colorClass} />
    case 'waiting':
      return <PauseCircle size={size} className={colorClass} />
    case 'error':
      return <XCircle size={size} className={colorClass} />
    case 'unconfigured':
      return <Circle size={size} className={colorClass} />
    default:
      return <Circle size={size} className={colorClass} />
  }
}

// ---------------------------------------------------------------------------
// Status helpers
// ---------------------------------------------------------------------------

const STATUS_LABEL_KEYS: Record<RepoStatus, string> = {
  up_to_date: 'status.upToDate',
  sync_needed: 'status.syncNeeded',
  syncing: 'status.syncing',
  waiting: 'status.waiting',
  conflict: 'status.conflict',
  resolving: 'status.resolving',
  resolved: 'status.resolved',
  error: 'status.error',
  unconfigured: 'status.unconfigured'
}

export function useStatusConfig(status: RepoStatus) {
  const { t } = useTranslation()
  const labelKey = STATUS_LABEL_KEYS[status] ?? STATUS_LABEL_KEYS.unconfigured
  return { label: t(labelKey), color: getStatusColor(status) }
}
