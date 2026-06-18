/**
 * WorkflowSteps — renders the 7-step sync workflow timeline
 *
 * Layout skeleton only. Step content (buttons, resolve details) is delegated
 * to StepContent component.
 */

import { useTranslation } from 'react-i18next'
import { StepItem } from '@/components/StepItem'
import { StepContent, type StepActions } from '@/components/StepContent'
import type { Repo, ResolveData, AgentStreamEvent } from '@shared/types/engine'

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

interface WorkflowStepsProps {
  repo: Repo
  streamEvents?: AgentStreamEvent[]
  isStreamLive?: boolean
  resolveResult?: ResolveData | null
  actions: StepActions
  loading?: boolean
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function WorkflowSteps({
  repo,
  streamEvents = [],
  isStreamLive = false,
  resolveResult,
  actions,
  loading = false
}: WorkflowStepsProps): JSX.Element {
  const { t } = useTranslation()
  const workflow = repo.workflow

  if (!workflow) {
    return (
      <div className="px-4 pb-4 pt-2">
        <p className="text-sm text-muted-foreground">No active workflow.</p>
      </div>
    )
  }

  const steps = workflow.steps

  const getStepProps = (index: number) => {
    const isLast = index === steps.length - 1
    const isNextActive = index < steps.length - 1 &&
      (steps[index + 1].status === 'running' || steps[index + 1].status === 'success' || steps[index + 1].status === 'waiting')
    return { isLast, isNextActive }
  }

  return (
    <div className="px-4 pb-4 pt-3">
      <div className="space-y-0">
        {steps.map((stepRecord, idx) => {
          const { isLast, isNextActive } = getStepProps(idx)

          return (
            <StepItem
              key={stepRecord.step}
              step={stepRecord.step}
              status={stepRecord.status}
              message={stepRecord.message}
              error={stepRecord.error}
              isLast={isLast}
              isNextActive={isNextActive}
            >
              <StepContent
                stepRecord={stepRecord}
                allSteps={steps}
                actions={actions}
                loading={loading}
                resolveResult={resolveResult ?? null}
                streamEvents={streamEvents}
                isStreamLive={isStreamLive}
              />
            </StepItem>
          )
        })}
      </div>

      {/* Workflow overall status */}
      {workflow.status === 'success' && (
        <p className="text-xs text-success mt-2">{t('workflow.completed')}</p>
      )}
      {workflow.status === 'failed' && (
        <p className="text-xs text-error mt-2">{t('workflow.failed')}</p>
      )}
    </div>
  )
}
