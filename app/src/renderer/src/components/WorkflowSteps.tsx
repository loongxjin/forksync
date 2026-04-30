import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { StepItem, MiniTerminal } from '@/components/StepItem'
import type { Repo, WorkflowStepRecord, AgentStreamEvent } from '@/types/engine'
import { cn } from '@/lib/utils'
import { Bot, Monitor, GitPullRequestClosed, RotateCcw } from 'lucide-react'

interface WorkflowStepsProps {
  repo: Repo
  streamEvents?: AgentStreamEvent[]
  isStreamLive?: boolean
  onResolveWithAgent?: () => void
  onOpenIDE?: () => void
  onAbort?: () => void
  onAccept?: () => void
  onReject?: () => void
  onRetryCommit?: () => void
  onViewTerminal?: () => void
  loading?: boolean
}

export function WorkflowSteps({
  repo,
  streamEvents = [],
  isStreamLive = false,
  onResolveWithAgent,
  onOpenIDE,
  onAbort,
  onAccept,
  onReject,
  onRetryCommit,
  onViewTerminal,
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
    const step = steps[index]
    const isLast = index === steps.length - 1
    const isNextActive = index < steps.length - 1 &&
      (steps[index + 1].status === 'running' || steps[index + 1].status === 'success' || steps[index + 1].status === 'waiting')
    return { step, isLast, isNextActive }
  }

  return (
    <div className="px-4 pb-4 pt-3">
      <div className="space-y-0">
        {steps.map((stepRecord, idx) => {
          const { isLast, isNextActive } = getStepProps(idx)
          const isAgentResolveRunning = stepRecord.step === 'agent_resolve' && stepRecord.status === 'running'
          const isWaiting = stepRecord.status === 'waiting'

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
              {/* Mini terminal for agent_resolve running */}
              {isAgentResolveRunning && (
                <MiniTerminal
                  events={streamEvents}
                  isLive={isStreamLive}
                  onExpand={onViewTerminal}
                />
              )}

              {/* Waiting step action buttons */}
              {isWaiting && (
                <div className="flex flex-wrap gap-2 mt-2">
                  {stepRecord.step === 'resolve_strategy' && (
                    <>
                      {onResolveWithAgent && (
                        <Button
                          onClick={onResolveWithAgent}
                          disabled={loading}
                          size="sm"
                          variant="default"
                        >
                          <Bot size={14} className="mr-1" />
                          {t('resolvePanel.resolveWithAgent')}
                        </Button>
                      )}
                      {onOpenIDE && (
                        <Button
                          onClick={onOpenIDE}
                          disabled={loading}
                          size="sm"
                          variant="outline"
                        >
                          <Monitor size={14} className="mr-1" />
                          Open in IDE
                        </Button>
                      )}
                      {onAbort && (
                        <Button
                          onClick={onAbort}
                          disabled={loading}
                          size="sm"
                          variant="destructive"
                        >
                          <GitPullRequestClosed size={14} className="mr-1" />
                          Abort Merge
                        </Button>
                      )}
                    </>
                  )}

                  {stepRecord.step === 'accept_changes' && (
                    <>
                      {onAccept && (
                        <Button
                          onClick={onAccept}
                          disabled={loading}
                          size="sm"
                          variant="default"
                        >
                          <RotateCcw size={14} className="mr-1" />
                          Accept & Commit
                        </Button>
                      )}
                      {onViewTerminal && (
                        <Button
                          onClick={onViewTerminal}
                          disabled={loading}
                          size="sm"
                          variant="outline"
                        >
                          View Diff
                        </Button>
                      )}
                      {onReject && (
                        <Button
                          onClick={onReject}
                          disabled={loading}
                          size="sm"
                          variant="destructive"
                        >
                          Reject
                        </Button>
                      )}
                    </>
                  )}

                  {stepRecord.step === 'commit' && (
                    <>
                      {onRetryCommit && (
                        <Button
                          onClick={onRetryCommit}
                          disabled={loading}
                          size="sm"
                          variant="default"
                        >
                          <RotateCcw size={14} className="mr-1" />
                          Retry Commit
                        </Button>
                      )}
                      {onOpenIDE && (
                        <Button
                          onClick={onOpenIDE}
                          disabled={loading}
                          size="sm"
                          variant="outline"
                        >
                          <Monitor size={14} className="mr-1" />
                          Open in IDE
                        </Button>
                      )}
                      {onAbort && (
                        <Button
                          onClick={onAbort}
                          disabled={loading}
                          size="sm"
                          variant="destructive"
                        >
                          <GitPullRequestClosed size={14} className="mr-1" />
                          Abort Merge
                        </Button>
                      )}
                    </>
                  )}
                </div>
              )}
            </StepItem>
          )
        })}
      </div>

      {/* Workflow overall status */}
      {workflow.status === 'success' && (
        <p className="text-xs text-success mt-2">Workflow completed successfully</p>
      )}
      {workflow.status === 'failed' && (
        <p className="text-xs text-error mt-2">Workflow failed</p>
      )}
    </div>
  )
}
