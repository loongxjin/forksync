import { useEffect, useRef } from 'react'
import type { Repo } from '@shared/types/engine'

/**
 * useAgentLogAutoload — auto-load the agent log for repos with active agent
 * resolution, exactly once per workflow run.
 *
 * Extracted from HomePage. Two effects cooperate:
 *  1. When a new workflow starts (startedAt changes), reset the auto-load flag
 *     so the next sync/re-resolve reads the fresh log (loadAgentLog overwrites
 *     old events via STREAM_LOAD).
 *  2. For each repo in 'resolving' status — or 'syncing' with a running
 *     agent_resolve step — call loadAgentLog once (guarded by autoLoadedRef).
 *
 * loadAgentLog is passed in (not pulled internally) because it comes from the
 * useResolveStream hook instance owned by the host; instantiating a second one
 * here would create a duplicate subscription.
 */
export function useAgentLogAutoload(params: {
  repos: Repo[]
  initialized: boolean
  loadAgentLog: (repoName: string, sessionId?: string) => void
}): void {
  const { repos, initialized, loadAgentLog } = params

  // Track which repos have been auto-loaded to prevent repeated loadAgentLog
  // calls when repos changes (e.g. status poll every 3s during sync).
  const autoLoadedRef = useRef<Set<string>>(new Set())

  // When a new workflow starts, reset auto-load so the next sync/re-resolve
  // picks up the new agent log.
  const lastWorkflowStartRef = useRef<Record<string, string>>({})
  useEffect(() => {
    for (const repo of repos) {
      const startedAt = repo.workflow?.startedAt
      if (startedAt && startedAt !== lastWorkflowStartRef.current[repo.name]) {
        lastWorkflowStartRef.current[repo.name] = startedAt
        autoLoadedRef.current.delete(repo.name)
      }
    }
  }, [repos])

  // Auto-load agent logs for repos with active agent resolution.
  // Guarded by autoLoadedRef to fire only once per workflow.
  useEffect(() => {
    if (!initialized) return
    for (const repo of repos) {
      if (autoLoadedRef.current.has(repo.name)) continue

      // Extract the resolve session id from the agent_resolve step so the
      // log is read by session name (precise), not "newest file in the dir".
      const resolveSessionId = repo.workflow?.steps?.find(
        (s) => s.step === 'agent_resolve'
      )?.resolveSessionId ?? ''

      if (repo.status === 'resolving') {
        autoLoadedRef.current.add(repo.name)
        loadAgentLog(repo.name, resolveSessionId)
        continue
      }
      if (repo.status === 'syncing' && repo.workflow) {
        const agentStep = repo.workflow.steps.find(
          (s) => s.step === 'agent_resolve' && s.status === 'running'
        )
        if (agentStep) {
          autoLoadedRef.current.add(repo.name)
          loadAgentLog(repo.name, resolveSessionId)
        }
      }
    }
  }, [initialized, repos, loadAgentLog])
}
