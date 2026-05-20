/**
 * Pure helper functions for WorkflowSteps component logic.
 * Extracted for testability — these functions contain no UI or side effects.
 */

import type { ResolveData } from '@shared/types/engine'

interface StepRecord {
  step: string
  status: string
  message?: string
  error?: string
}

/**
 * Determine whether resolve details (agent info, summary, conflicts, diff)
 * should be displayed given the current resolve result.
 */
export function shouldShowResolveDetails(resolveResult: ResolveData | null | undefined): boolean {
  if (!resolveResult) return false
  const { agentResult, conflicts, commitError } = resolveResult
  return !!(
    agentResult?.agentName ||
    agentResult?.summary ||
    (conflicts && conflicts.length > 0) ||
    commitError
  )
}

/**
 * Determine whether to show the resolve detail panel after a given step.
 *
 * Rules:
 * 1. After agent_resolve step finishes (success or failed)
 * 2. After accept_changes step is waiting, but only if agent_resolve hasn't already finished
 *    (avoids showing duplicate details when both paths converge)
 */
export function shouldShowStepDetail(
  step: StepRecord,
  allSteps: StepRecord[]
): boolean {
  const isAgentResolveFinished = step.step === 'agent_resolve' &&
    (step.status === 'success' || step.status === 'failed')

  const anyAgentResolveFinished = allSteps.some(
    s => s.step === 'agent_resolve' && (s.status === 'success' || s.status === 'failed')
  )

  const isAcceptWaitingWithoutAgent = step.step === 'accept_changes' &&
    step.status === 'waiting' &&
    !anyAgentResolveFinished

  return isAgentResolveFinished || isAcceptWaitingWithoutAgent
}
