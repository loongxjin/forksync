import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { DiffViewer } from '@/components/DiffViewer'
import { StepItem } from '@/components/StepItem'
import type { Repo, ResolveData, AgentStreamEvent } from '@/types/engine'
import { cn } from '@/lib/utils'
import { Bot, Monitor, GitPullRequestClosed, RotateCcw, Terminal, Eye, FileDiff, X, FileText, AlertTriangle } from 'lucide-react'

interface WorkflowStepsProps {
  repo: Repo
  streamEvents?: AgentStreamEvent[]
  isStreamLive?: boolean
  resolveResult?: ResolveData | null
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
  resolveResult,
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
  const [selectedFile, setSelectedFile] = useState<string | null>(null)

  if (!workflow) {
    return (
      <div className="px-4 pb-4 pt-2">
        <p className="text-sm text-muted-foreground">No active workflow.</p>
      </div>
    )
  }

  const steps = workflow.steps
  const eventsCount = streamEvents.length

  // Extract resolve detail data
  const agentResult = resolveResult?.agentResult
  const conflicts = resolveResult?.conflicts ?? []
  const diff = agentResult?.diff
  const showResolveDetails = resolveResult && (
    agentResult?.agentName || agentResult?.summary || conflicts.length > 0 || resolveResult?.commitError
  )

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

          // Should we show resolve details after this step?
          const showDetailsAfterAgentResolve =
            stepRecord.step === 'agent_resolve' &&
            (stepRecord.status === 'success' || stepRecord.status === 'failed')
          const agentResolveFinished = steps.some(
            s => s.step === 'agent_resolve' && (s.status === 'success' || s.status === 'failed')
          )
          const showDetailsAfterAcceptWaiting =
            stepRecord.step === 'accept_changes' &&
            stepRecord.status === 'waiting' &&
            !agentResolveFinished

          const showDetails = showDetailsAfterAgentResolve || showDetailsAfterAcceptWaiting

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

              {/* View Agent Log button for agent_resolve success */}
              {stepRecord.step === 'agent_resolve' && stepRecord.status === 'success' && onViewTerminal && (
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

              {/* agent_resolve failed: show retry + abort */}
              {stepRecord.step === 'agent_resolve' && stepRecord.status === 'failed' && (
                <div className="mt-1 flex flex-wrap gap-2">
                  {onViewTerminal && (
                    <Button
                      onClick={onViewTerminal}
                      size="sm"
                      variant="outline"
                      className="text-xs"
                    >
                      <Eye size={14} className="mr-1.5" />
                      {t('workflow.viewAgentLog')}
                    </Button>
                  )}
                  {onResolveWithAgent && (
                    <Button
                      onClick={onResolveWithAgent}
                      disabled={loading}
                      size="sm"
                      variant="default"
                      className="text-xs"
                    >
                      <RotateCcw size={14} className="mr-1.5" />
                      {t('workflow.retryResolve')}
                    </Button>
                  )}
                  {onAbort && (
                    <Button
                      onClick={onAbort}
                      disabled={loading}
                      size="sm"
                      variant="destructive"
                      className="text-xs"
                    >
                      <GitPullRequestClosed size={14} className="mr-1.5" />
                      {t('workflow.abortMerge')}
                    </Button>
                  )}
                </div>
              )}

              {/* Resolve details: agent info, summary, conflict files, diff, commit error */}
              {showDetails && showResolveDetails && (
                <div className="mt-2 ml-1 border-l-2 border-primary/20 pl-3 space-y-3">
                  {/* Agent info */}
                  {agentResult?.agentName && (
                    <p className="text-xs text-muted-foreground">
                      <Bot size={12} className="inline mr-1" />
                      {t('resolvePanel.agent')} <span className="text-foreground font-medium">{agentResult.agentName}</span>
                    </p>
                  )}

                  {/* AI Summary */}
                  {agentResult?.summary && (
                    <div className="rounded-lg bg-primary/5 border border-primary/10 p-3">
                      <p className="text-xs font-medium text-muted-foreground mb-1">
                        <FileText size={12} className="inline mr-1" />
                        {t('home.aiSuggestion')}
                      </p>
                      <p className="text-sm leading-relaxed">{agentResult.summary}</p>
                    </div>
                  )}

                  {/* Conflict files */}
                  {conflicts.length > 0 && (
                    <div>
                      <p className="text-xs font-medium text-muted-foreground mb-1">
                        {t('conflicts.conflictFiles')}
                      </p>
                      <div className="space-y-0.5">
                        {conflicts.map((f) => (
                          <button
                            key={f.path}
                            onClick={() => setSelectedFile(selectedFile === f.path ? null : f.path)}
                            className={cn(
                              'flex items-center gap-2 text-sm w-full text-left px-2 py-1.5 rounded-md transition-colors duration-150',
                              selectedFile === f.path ? 'bg-accent text-foreground' : 'hover:bg-accent/50 text-muted-foreground hover:text-foreground'
                            )}
                          >
                            <AlertTriangle size={12} className="text-error shrink-0" />
                            <span className="truncate font-mono text-xs">{f.path}</span>
                          </button>
                        ))}
                      </div>
                    </div>
                  )}

                  {/* Diff preview */}
                  {diff && selectedFile && (
                    <div>
                      <p className="text-xs font-medium text-muted-foreground mb-1">
                        {t('conflicts.diffPreview')} — {selectedFile}
                        <span className="text-error ml-1">({t('conflicts.fullStagedDiff')})</span>
                      </p>
                      <DiffViewer diff={diff} className="max-h-64" />
                    </div>
                  )}

                  {/* Commit failure warning */}
                  {resolveResult?.commitError && (
                    <div className="rounded-lg bg-error/10 border border-error/20 p-3">
                      <p className="text-xs font-medium text-error flex items-center gap-1.5">
                        <AlertTriangle size={12} />
                        {t('resolvePanel.commitFailed')}
                      </p>
                      <p className="mt-1 text-xs text-error/80 font-mono break-all">{resolveResult.commitError}</p>
                      <p className="mt-1.5 text-xs text-muted-foreground">
                        {t('resolvePanel.commitFailedHint')}
                      </p>
                    </div>
                  )}
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
