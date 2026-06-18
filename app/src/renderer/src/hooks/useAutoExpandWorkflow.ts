import { useCallback, useEffect, useState } from 'react'
import type { Repo } from '@shared/types/engine'

/**
 * useAutoExpandWorkflow — manages which repo accordion rows are expanded.
 *
 * Extracted from HomePage. Owns the expandedRepoIds set and exposes
 * toggleExpand for manual clicks. A side effect auto-expands repos whose
 * workflow is running/waiting (so the user sees live progress during SyncAll)
 * and collapses them again when the workflow finishes (success/failed).
 *
 * The auto-expand only ever ADDS to the set — it never collapses a repo the
 * user manually expanded, so manual and automatic state coexist safely.
 */
export function useAutoExpandWorkflow(repos: Repo[]): {
  expandedRepoIds: Set<string>
  toggleExpand: (repoId: string) => void
} {
  const [expandedRepoIds, setExpandedRepoIds] = useState<Set<string>>(new Set())

  const toggleExpand = useCallback((repoId: string) => {
    setExpandedRepoIds((prev) => {
      const next = new Set(prev)
      if (next.has(repoId)) {
        next.delete(repoId)
      } else {
        next.add(repoId)
      }
      return next
    })
  }, [])

  // Auto-expand repos with active (running/waiting) workflows during SyncAll.
  // Only adds to the set — never collapses a repo the user manually expanded.
  // Collapsed repos are cleaned up when their workflow finishes.
  useEffect(() => {
    setExpandedRepoIds((prev) => {
      const next = new Set(prev)
      let changed = false
      for (const repo of repos) {
        const wf = repo.workflow
        if (!wf) {
          // No active workflow — remove from auto-expand if it was there
          if (next.has(repo.id)) {
            next.delete(repo.id)
            changed = true
          }
          continue
        }
        if (wf.status === 'running' || wf.status === 'waiting') {
          if (!next.has(repo.id)) {
            next.add(repo.id)
            changed = true
          }
        } else if (wf.status === 'success' || wf.status === 'failed') {
          // Workflow finished — remove from auto-expand
          if (next.has(repo.id)) {
            next.delete(repo.id)
            changed = true
          }
        }
      }
      return changed ? next : prev
    })
  }, [repos])

  return { expandedRepoIds, toggleExpand }
}
