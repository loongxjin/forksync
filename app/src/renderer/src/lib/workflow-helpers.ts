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
  // Always show details at accept_changes when it's waiting for user
  // confirmation — this is the canonical review step.
  if (step.step === 'accept_changes' && step.status === 'waiting') {
    return true
  }

  // Show at agent_resolve only while accept_changes hasn't reached waiting
  // yet — avoids showing the same details twice.
  const isAgentResolveFinished = step.step === 'agent_resolve' &&
    (step.status === 'success' || step.status === 'failed')
  if (!isAgentResolveFinished) return false

  const anyAcceptWaiting = allSteps.some(
    s => s.step === 'accept_changes' && s.status === 'waiting'
  )
  return !anyAcceptWaiting
}

/**
 * Filter a full `git diff HEAD` output to only the section belonging to the
 * given file path. Splits on `diff --git` boundaries and matches against the
 * a/filePath and b/filePath headers.
 */
export function filterDiffByFile(diff: string, filePath: string): string {
  if (!diff || !filePath) return diff
  // Split on lines that start with "diff --git" (anchored ^)
  const sections = diff.split(/^(?=diff --git )/m)
  const section = sections.find((s) => {
    const nl = s.indexOf('\n')
    const header = nl === -1 ? s : s.slice(0, nl)
    return header.includes(`a/${filePath}`) || header.includes(`b/${filePath}`)
  })
  return section || diff
}
