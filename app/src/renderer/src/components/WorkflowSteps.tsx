import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { StepItem } from '@/components/StepItem'
import type { Repo, WorkflowStepRecord, AgentStreamEvent } from '@/types/engine'
import { cn } from '@/lib/utils'
import { Bot, Monitor, GitPullRequestClosed, RotateCcw, Terminal, Eye, FileDiff, X } from 'lucide-react'

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
  const eventsCount = streamEvents.length

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
              {/* View Live Terminal button for agent_resolve running */}
              {isAgentResolveRunning && onViewTerminal && (
                <div className="mt-2 flex items-center gap-2">
                  <Button
                    onClick={onViewTerminal}
                    size="sm"
                    variant="outline"
                    className="text-xs"
                  >
                    <Terminal size={14} className="mr-1.5" />
                    {t('workflow.viewLiveTerminal')}
                  </Button>
                  {eventsCount > 0 && (
                    <Badge variant="secondary" className="text-[10px] px-1.5 py-0">
                      {t('workflow.eventsCount', { count: eventsCount })}
                    </Badge>
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
                    <Eye size={14} className="mr-1.5" />
                    {t('workflow.viewAgentLog')}
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
                          <span className="text-sm text-muted-foreground">{t('workflow.agentResolving')}</span>
                          {onViewTerminal && (
                            <Button
                              onClick={onViewTerminal}
                              size="sm"
                              variant="outline"
                              className="text-xs ml-2"
                            >
                              <Terminal size={14} className="mr-1.5" />
                              {t('workflow.viewLive')}
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
                              <Bot size={14} className="mr-1.5" />
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
                              <Monitor size={14} className="mr-1.5" />
                              {t('workflow.openInIDE')}
                            </Button>
                          )}
                          {onAbort && (
                            <Button
                              onClick={onAbort}
                              disabled={loading}
                              size="sm"
                              variant="destructive"
                            >
                              <GitPullRequestClosed size={14} className="mr-1.5" />
                              {t('workflow.abortMerge')}
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
                          <RotateCcw size={14} className="mr-1.5" />
                          {t('workflow.acceptCommit')}
                        </Button>
                      )}
                      {onViewDiff && (
                        <Button
                          onClick={onViewDiff}
                          disabled={loading}
                          size="sm"
                          variant="outline"
                        >
                          <FileDiff size={14} className="mr-1.5" />
                          {t('workflow.viewDiff')}
                        </Button>
                      )}
                      {onReject && (
                        <Button
                          onClick={onReject}
                          disabled={loading}
                          size="sm"
                          variant="destructive"
                        >
                          <X size={14} className="mr-1.5" />
                          {t('workflow.reject')}
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
                          <RotateCcw size={14} className="mr-1.5" />
                          {t('workflow.retryCommit')}
                        </Button>
                      )}
                      {onOpenIDE && (
                        <Button
                          onClick={onOpenIDE}
                          disabled={loading}
                          size="sm"
                          variant="outline"
                        >
                          <Monitor size={14} className="mr-1.5" />
                          {t('workflow.openInIDE')}
                        </Button>
                      )}
                      {onAbort && (
                        <Button
                          onClick={onAbort}
                          disabled={loading}
                          size="sm"
                          variant="destructive"
                        >
                          <GitPullRequestClosed size={14} className="mr-1.5" />
                          {t('workflow.abortMerge')}
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
        <p className="text-xs text-success mt-2">{t('workflow.completed')}</p>
      )}
      {workflow.status === 'failed' && (
        <p className="text-xs text-error mt-2">{t('workflow.failed')}</p>
      )}
    </div>
  )
}
