import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { StepItem, MiniTerminal } from '@/components/StepItem'
import type { Repo, WorkflowStepRecord, AgentStreamEvent } from '@/types/engine'
import { cn } from '@/lib/utils'
import { Bot, Monitor, GitPullRequestClosed, RotateCcw, Terminal } from 'lucide-react'

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
  onViewDiff?: () => void
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
  onViewDiff,
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
              {/* Mini terminal + View Live button for agent_resolve running */}
              {isAgentResolveRunning && (
                <div className="mt-2 space-y-2">
                  <MiniTerminal
                    events={streamEvents}
                    isLive={isStreamLive}
                    onExpand={onViewTerminal}
                  />
                  {onViewTerminal && (
                    <Button
                      onClick={onViewTerminal}
                      size="sm"
                      variant="outline"
                      className="text-xs"
                    >
                      <Terminal size={12} className="mr-1" />
                      View Live Terminal
                    </Button>
                  )}
                </div>
              )}

              {/* View Agent Log button for agent_resolve success/failed */}
              {stepRecord.step === 'agent_resolve' && (stepRecord.status === 'success' || stepRecord.status === 'failed') && onViewTerminal && (
                <div className="mt-1">
                  <Button
                    onClick={onViewTerminal}
                    size="sm"
                    variant="outline"
                    className="text-xs"
                  >
                    <Terminal size={12} className="mr-1" />
                    View Agent Log
                  </Button>
                </div>
              )}

              {/* Waiting step action buttons */}
              {isWaiting && (
                <div className="flex flex-wrap gap-2 mt-2">
                  {stepRecord.step === 'resolve_strategy' && (
                    <>
                      {isStreamLive ? (
                        <div className="flex items-center gap-2">
                          <span className="inline-block h-4 w-4 animate-spin rounded-full border-2 border-primary border-t-transparent" />
                          <span className="text-sm text-muted-foreground">Agent resolving conflicts...</span>
                          {onViewTerminal && (
                            <Button
                              onClick={onViewTerminal}
                              size="sm"
                              variant="outline"
                              className="text-xs ml-2"
                            >
                              <Terminal size={12} className="mr-1" />
                              View Live
                            </Button>
                          )}
                        </div>
                      ) : (
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
                      {onViewDiff && (
                        <Button
                          onClick={onViewDiff}
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
