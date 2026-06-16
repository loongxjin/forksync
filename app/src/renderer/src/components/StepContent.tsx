/**
 * StepContent — renders action buttons and resolve details for a workflow step
 *
 * Extracted from WorkflowSteps to isolate the step × status rendering matrix
 * from the step skeleton layout.
 */

import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { DiffViewer } from '@/components/DiffViewer'
import type { WorkflowStepRecord, ResolveData, AgentStreamEvent } from '@shared/types/engine'
import { cn } from '@/lib/utils'
import { shouldShowStepDetail, shouldShowResolveDetails, filterDiffByFile } from '@/lib/workflow-helpers'
import { Bot, Monitor, GitPullRequestClosed, RotateCcw, Terminal, Eye, FileDiff, X, FileText, AlertTriangle } from 'lucide-react'
import { Markdown } from '@/components/ui/markdown'

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export interface StepActions {
  onResolveWithAgent?: () => void
  onOpenIDE?: () => void
  onAbort?: () => void
  onAccept?: () => void
  onReject?: () => void
  onRetryCommit?: () => void
  onViewTerminal?: () => void
  onViewDiff?: () => void
}

export interface StepContentProps {
  stepRecord: WorkflowStepRecord
  allSteps: WorkflowStepRecord[]
  actions: StepActions
  loading: boolean
  resolveResult: ResolveData | null
  streamEvents: AgentStreamEvent[]
  isStreamLive: boolean
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function StepContent({
  stepRecord,
  allSteps,
  actions,
  loading,
  resolveResult,
  streamEvents,
  isStreamLive
}: StepContentProps): JSX.Element | null {
  const { t } = useTranslation()
  const [selectedFile, setSelectedFile] = useState<string | null>(null)

  const {
    onResolveWithAgent, onOpenIDE, onAbort,
    onAccept, onReject, onRetryCommit,
    onViewTerminal, onViewDiff
  } = actions

  const isAgentResolveRunning = stepRecord.step === 'agent_resolve' && stepRecord.status === 'running'
  const isWaiting = stepRecord.status === 'waiting'
  const showDetails = shouldShowStepDetail(stepRecord, allSteps)

  // Resolve detail data
  const agentResult = resolveResult?.agentResult
  const conflicts = resolveResult?.conflicts ?? []
  const diff = agentResult?.diff
  const showResolveDetails = shouldShowResolveDetails(resolveResult)
  // TRACE
  if (showDetails) {
    console.log('[trace] StepContent', stepRecord.step, stepRecord.status, 'showResolveDetails', showResolveDetails, 'resolveResult', resolveResult ? { conflictsCount: conflicts.length, agentName: agentResult?.agentName, summaryLen: agentResult?.summary?.length, resolvedFiles: agentResult?.resolvedFiles } : null)
  }
  const eventsCount = streamEvents.length

  return (
    <>
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
              <Markdown>{agentResult.summary}</Markdown>
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
              </p>
              <DiffViewer diff={filterDiffByFile(diff, selectedFile)} className="max-h-64" />
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
    </>
  )
}
