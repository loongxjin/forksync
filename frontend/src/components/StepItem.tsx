import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import type { WorkflowStep, WorkflowStepStatus } from '@shared/types/engine'
import {
  CheckCircle2,
  XCircle,
  Loader2,
  CircleDot,
  Clock,
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

const STEP_LABEL_KEYS: Record<WorkflowStep, string> = {
  fetch: 'workflow.step.fetch',
  merge: 'workflow.step.merge',
  check_conflicts: 'workflow.step.checkConflicts',
  resolve_strategy: 'workflow.step.resolveStrategy',
  agent_resolve: 'workflow.step.agentResolve',
  accept_changes: 'workflow.step.acceptChanges',
  commit: 'workflow.step.commit'
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
        return <Clock size={16} className="text-warning animate-pulse" />
      case 'skipped':
        return <SkipForward size={14} className="text-muted-foreground opacity-50" />
      default:
        return <CircleDot size={16} className="text-muted-foreground/55" />
    }
  })()

  const lineColor = (() => {
    if (status === 'running') return 'bg-warning/60 animate-pulse'
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
        <div className="relative z-10 h-5 w-5 flex items-center justify-center transition-all duration-200">
          {icon}
        </div>
        {/* Connector line */}
        {!isLast && (
          <div
            className={cn(
              'w-[3px] flex-1 transition-colors duration-300',
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
              'text-sm font-medium transition-colors duration-200',
              status === 'pending' && 'text-muted-foreground/60',
              status === 'skipped' && 'text-muted-foreground opacity-50',
              status === 'running' && 'text-foreground',
              status === 'success' && 'text-foreground',
              status === 'failed' && 'text-error',
              status === 'waiting' && 'text-warning'
            )}
          >
            {t(STEP_LABEL_KEYS[step] ?? 'workflow.step.fetch')}
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
