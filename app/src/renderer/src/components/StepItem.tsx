import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import type { WorkflowStep, WorkflowStepStatus } from '@/types/engine'
import {
  CheckCircle2,
  XCircle,
  Loader2,
  Circle,
  PauseCircle,
  SkipForward,
  Terminal
} from 'lucide-react'

interface StepItemProps {
  step: WorkflowStep
  status: WorkflowStepStatus
  message?: string
  error?: string
  isLast: boolean
  isNextActive: boolean
  children?: React.ReactNode
}

const STEP_LABELS: Record<WorkflowStep, string> = {
  fetch: 'Fetch upstream',
  merge: 'Merge upstream',
  check_conflicts: 'Check conflicts',
  resolve_strategy: 'Resolve strategy',
  agent_resolve: 'Agent resolve',
  accept_changes: 'Accept changes',
  commit: 'Commit & finalize'
}

export function StepItem({
  step,
  status,
  message,
  error,
  isLast,
  isNextActive,
  children
}: StepItemProps): JSX.Element {
  const { t } = useTranslation()

  const icon = (() => {
    switch (status) {
      case 'running':
        return <Loader2 size={16} className="animate-spin text-warning" />
      case 'success':
        return <CheckCircle2 size={16} className="text-success" />
      case 'failed':
        return <XCircle size={16} className="text-error" />
      case 'waiting':
        return <PauseCircle size={16} className="text-warning" />
      case 'skipped':
        return <SkipForward size={16} className="text-muted-foreground opacity-50" />
      default:
        return <Circle size={16} className="text-muted-foreground opacity-40" />
    }
  })()

  const lineColor = (() => {
    if (status === 'success') return 'bg-success'
    if (status === 'failed') return 'bg-error'
    if (status === 'skipped') return 'bg-muted-foreground/30'
    return 'bg-muted-foreground/20'
  })()

  return (
    <div className="relative flex gap-3">
      {/* Connector line */}
      {!isLast && (
        <div
          className={cn(
            'absolute left-[7px] top-6 w-[2px] h-[calc(100%-8px)] transition-colors',
            lineColor,
            isNextActive && status === 'success' && 'bg-success'
          )}
        />
      )}

      {/* Icon */}
      <div className="relative z-10 flex h-4 w-4 shrink-0 items-center justify-center mt-0.5">
        {icon}
      </div>

      {/* Content */}
      <div className="flex-1 min-w-0 pb-3">
        <div className="flex items-center gap-2">
          <span
            className={cn(
              'text-sm font-medium',
              status === 'pending' && 'text-muted-foreground opacity-60',
              status === 'skipped' && 'text-muted-foreground opacity-50',
              status === 'running' && 'text-foreground',
              status === 'success' && 'text-foreground',
              status === 'failed' && 'text-error',
              status === 'waiting' && 'text-warning'
            )}
          >
            {STEP_LABELS[step] ?? step}
          </span>
          {status === 'running' && (
            <span className="text-xs text-muted-foreground animate-pulse">
              {t('common.processing')}
            </span>
          )}
        </div>

        {/* Message / Error */}
        {message && status !== 'success' && status !== 'skipped' && (
          <p className="text-xs text-muted-foreground mt-0.5">{message}</p>
        )}
        {error && (
          <p className="text-xs text-error mt-0.5">{error}</p>
        )}

        {/* Action buttons or mini terminal */}
        {children}
      </div>
    </div>
  )
}

interface MiniTerminalProps {
  events: { t: string; d?: string; ts: string }[]
  isLive: boolean
  onExpand?: () => void
}

export function MiniTerminal({ events, isLive, onExpand }: MiniTerminalProps): JSX.Element {
  return (
    <div
      className="mt-2 rounded-md border border-border bg-card overflow-hidden cursor-pointer"
      onClick={onExpand}
    >
      <div className="flex items-center justify-between px-2 py-1 border-b border-border bg-muted/30">
        <div className="flex items-center gap-1.5">
          <Terminal size={12} className="text-muted-foreground" />
          <span className="text-[10px] text-muted-foreground font-mono">
            {isLive ? 'Live' : 'Log'}
          </span>
        </div>
        {isLive && (
          <span className="inline-block h-1.5 w-1.5 rounded-full bg-success animate-pulse" />
        )}
      </div>
      <div className="h-40 overflow-y-auto p-2 font-mono text-[11px] leading-relaxed space-y-0.5">
        {events.length === 0 ? (
          <span className="text-muted-foreground opacity-50">Waiting for agent output...</span>
        ) : (
          events.map((ev, i) => (
            <div key={i} className="break-all">
              {ev.d && <span className="text-foreground/80">{ev.d}</span>}
            </div>
          ))
        )}
      </div>
    </div>
  )
}
