import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import type { WorkflowStep, WorkflowStepStatus } from '@/types/engine'
import {
  CheckCircle2,
  XCircle,
  Loader2,
  Circle,
  PauseCircle,
  SkipForward
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
        return <SkipForward size={14} className="text-muted-foreground opacity-50" />
      default:
        return <Circle size={14} className="text-muted-foreground opacity-40" />
    }
  })()

  const lineColor = (() => {
    if (status === 'success') return 'bg-success'
    if (status === 'failed') return 'bg-error'
    if (status === 'skipped') return 'bg-muted-foreground/30'
    return 'bg-muted-foreground/20'
  })()

  return (
    <div className="flex gap-3">
      {/* Left column: icon + connector */}
      <div className="flex flex-col items-center w-5 shrink-0">
        {/* Icon */}
        <div className="relative z-10 h-5 w-5 flex items-center justify-center">
          {icon}
        </div>
        {/* Connector line */}
        {!isLast && (
          <div
            className={cn(
              'w-[2px] flex-1 transition-colors',
              lineColor,
              isNextActive && status === 'success' && 'bg-success'
            )}
          />
        )}
      </div>

      {/* Right column: content */}
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

