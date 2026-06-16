/**
 * useResolveStream — unified resolve state management hook
 *
 * Merges two mutually exclusive resolve data paths into a single
 * `resolveResults` interface:
 *
 * - Path A (auto-resolve): RepoContext.syncResults entries with agentResult
 * - Path B (manual resolve): IPC NDJSON stream → streamResults
 *
 * Hides all stream lifecycle management (IPC listeners, polling,
 * watermarks) behind a clean data + operations interface.
 */

import { useReducer, useCallback, useRef, useEffect, useMemo } from 'react'
import type { ResolveData, AgentStreamEvent, SyncResult } from '@shared/types/engine'
import { engineApi } from '@/lib/api'
import { useRepos } from '@/contexts/RepoContext'
import { useLogger } from '@/hooks/useLogger'

// ---------------------------------------------------------------------------
// Stream State & Reducer (exported for testing)
// ---------------------------------------------------------------------------

export interface StreamState {
  streamEvents: Record<string, AgentStreamEvent[]>
  streamLive: Record<string, boolean>
  streamResults: Record<string, ResolveData | null>
  resolveResults: Record<string, ResolveData>
}

export type StreamAction =
  | { type: 'STREAM_START'; repoName: string }
  | { type: 'STREAM_EVENT'; repoName: string; event: AgentStreamEvent }
  | { type: 'STREAM_DONE'; repoName: string; result?: ResolveData | null }
  | { type: 'STREAM_LOAD'; repoName: string; events: AgentStreamEvent[]; isRunning: boolean }
  | { type: 'STREAM_CLEAR'; repoName: string }
  | { type: 'MERGE_SYNC_RESULTS'; resolved: { repoName: string; data: ResolveData }[] }

export const initialStreamState: StreamState = {
  streamEvents: {},
  streamLive: {},
  streamResults: {},
  resolveResults: {}
}

export function streamReducer(state: StreamState, action: StreamAction): StreamState {
  switch (action.type) {
    case 'STREAM_START':
      return {
        ...state,
        streamLive: { ...state.streamLive, [action.repoName]: true },
        streamEvents: { ...state.streamEvents, [action.repoName]: [] }
      }
    case 'STREAM_EVENT': {
      const existing = state.streamEvents[action.repoName] ?? []
      return {
        ...state,
        streamEvents: {
          ...state.streamEvents,
          [action.repoName]: [...existing, action.event]
        }
      }
    }
    case 'STREAM_DONE': {
      const { [action.repoName]: _, ...restLive } = state.streamLive
      const newStreamResults = action.result !== undefined
        ? { ...state.streamResults, [action.repoName]: action.result }
        : state.streamResults
      // Merge into resolveResults if result is non-null
      const newResolveResults = (action.result !== undefined && action.result)
        ? { ...state.resolveResults, [action.repoName]: action.result }
        : state.resolveResults
      return {
        ...state,
        streamLive: restLive,
        streamResults: newStreamResults,
        resolveResults: newResolveResults
      }
    }
    case 'STREAM_LOAD': {
      const nextLive = { ...state.streamLive }
      if (action.isRunning) {
        nextLive[action.repoName] = true
      } else {
        delete nextLive[action.repoName]
      }
      return {
        ...state,
        streamEvents: { ...state.streamEvents, [action.repoName]: action.events },
        streamLive: nextLive
      }
    }
    case 'STREAM_CLEAR': {
      const nextEvents = { ...state.streamEvents }
      delete nextEvents[action.repoName]
      const { [action.repoName]: _l, ...restLive } = state.streamLive
      const nextResolve = { ...state.resolveResults }
      delete nextResolve[action.repoName]
      return {
        ...state,
        streamEvents: nextEvents,
        streamLive: restLive,
        resolveResults: nextResolve
      }
    }
    case 'MERGE_SYNC_RESULTS': {
      if (action.resolved.length === 0) return state
      const next = { ...state.resolveResults }
      for (const { repoName, data } of action.resolved) {
        next[repoName] = data
      }
      return { ...state, resolveResults: next }
    }
    default:
      return state
  }
}

// ---------------------------------------------------------------------------
// Hook Interface
// ---------------------------------------------------------------------------

export interface ResolveStreamHook {
  /** Merged resolve results from both sync and stream paths */
  resolveResults: Record<string, ResolveData>
  /** Whether a repo is currently being resolved via stream */
  isStreamLive: (repoName: string) => boolean
  /** Get stream events for a repo */
  getStreamEvents: (repoName: string) => AgentStreamEvent[]
  /** Trigger manual resolve: prepare + stream start */
  startResolve: (repoName: string, opts?: { agent?: string; noConfirm?: boolean }) => Promise<void>
  /** Load existing agent log with optional polling */
  loadAgentLog: (repoName: string) => Promise<void>
  /** Clear stream state and resolve result for a repo */
  clearResult: (repoName: string) => void
  /** Raw stream results (for HomePage side effects) */
  streamResults: Record<string, ResolveData | null>
}

// ---------------------------------------------------------------------------
// Hook Implementation
// ---------------------------------------------------------------------------

export function useResolveStream(): ResolveStreamHook {
  const logger = useLogger('ResolveStream')
  const { syncResults } = useRepos()
  const [state, dispatch] = useReducer(streamReducer, initialStreamState)

  const ipcSetupRef = useRef(false)
  const pollTimersRef = useRef<Map<string, ReturnType<typeof setInterval>>>(new Map())
  const ipcEventCountRef = useRef<Record<string, number>>({})
  const pollWatermarkRef = useRef<Record<string, number>>({})
  const syncResultsMountedRef = useRef(false)

  // Path A: syncResults → resolveResults
  useEffect(() => {
    if (!syncResultsMountedRef.current) {
      syncResultsMountedRef.current = true
      return
    }
    const resolvedSyncs = syncResults.filter(
      (r: SyncResult) => r.status === 'resolved' && r.agentResult
    )
    if (resolvedSyncs.length > 0) {
      dispatch({
        type: 'MERGE_SYNC_RESULTS',
        resolved: resolvedSyncs.map((sr: SyncResult) => ({
          repoName: sr.repoName,
          data: {
            repoId: sr.repoId,
            conflicts: (sr.pendingConfirm ?? []).map((p: string) => ({ path: p })),
            agentResult: sr.agentResult,
            commitError: sr.commitError
          }
        }))
      })
    }
  }, [syncResults])

  // IPC listeners — set up once
  useEffect(() => {
    if (ipcSetupRef.current) return
    ipcSetupRef.current = true

    const unsubEvent = engineApi.onResolveStreamEvent((repoName, event) => {
      logger.log('stream event received', repoName, event.t)
      ipcEventCountRef.current[repoName] = (ipcEventCountRef.current[repoName] ?? 0) + 1
      dispatch({ type: 'STREAM_EVENT', repoName, event })
    })

    const unsubDone = engineApi.onResolveStreamDone((repoName, apiRes) => {
      logger.log('stream done received', repoName, apiRes.success)
      // TRACE: log what ResolveData the IPC carried
      if (apiRes.data) {
        logger.log('[trace] resolveResult data', repoName, 'conflictsCount', apiRes.data.conflicts?.length, 'resolvedFiles', apiRes.data.agentResult?.resolvedFiles, 'agentName', apiRes.data.agentResult?.agentName, 'summaryLen', apiRes.data.agentResult?.summary?.length)
      }
      const timer = pollTimersRef.current.get(repoName)
      if (timer) { clearInterval(timer); pollTimersRef.current.delete(repoName) }
      delete ipcEventCountRef.current[repoName]
      dispatch({ type: 'STREAM_DONE', repoName, result: apiRes.success ? apiRes.data : null })
    })

    const unsubError = engineApi.onResolveStreamError((repoName, error) => {
      logger.error('stream error received', repoName, error)
      const timer = pollTimersRef.current.get(repoName)
      if (timer) { clearInterval(timer); pollTimersRef.current.delete(repoName) }
      delete ipcEventCountRef.current[repoName]
      dispatch({ type: 'STREAM_EVENT', repoName, event: { t: 'error', d: error, ts: new Date().toISOString() } })
      dispatch({ type: 'STREAM_DONE', repoName, result: null })
    })

    return () => {
      ipcSetupRef.current = false
      unsubEvent()
      unsubDone()
      unsubError()
    }
  }, [])

  // Clean up poll timers when streamLive changes
  useEffect(() => {
    pollTimersRef.current.forEach((timer, name) => {
      if (!state.streamLive[name]) {
        logger.log('poll cleanup for', name, '(no longer live)')
        clearInterval(timer)
        pollTimersRef.current.delete(name)
      }
    })
  }, [state.streamLive])

  // Cleanup all poll timers on unmount
  useEffect(() => {
    return () => {
      pollTimersRef.current.forEach((timer) => { clearInterval(timer) })
      pollTimersRef.current.clear()
    }
  }, [])

  const startResolve = useCallback(async (
    repoName: string,
    opts?: { agent?: string; noConfirm?: boolean }
  ): Promise<void> => {
    logger.log('startResolve', repoName, opts)
    ipcEventCountRef.current[repoName] = 0
    dispatch({ type: 'STREAM_START', repoName })
    try {
      engineApi.resolveStreamStart(repoName, opts)
      logger.log('resolveStreamStart sent')
    } catch (err) {
      logger.error('resolveStreamStart failed', repoName, err)
      dispatch({ type: 'STREAM_EVENT', repoName, event: { t: 'error', d: `Failed to start resolve: ${(err as Error).message}`, ts: new Date().toISOString() } })
      dispatch({ type: 'STREAM_DONE', repoName, result: null })
    }
  }, [])

  const loadAgentLog = useCallback(async (repoName: string): Promise<void> => {
    logger.log('loadAgentLog', repoName)
    const existing = pollTimersRef.current.get(repoName)
    if (existing) {
      clearInterval(existing)
      pollTimersRef.current.delete(repoName)
    }
    try {
      const res = await engineApi.readAgentLog(repoName)
      logger.log('loadAgentLog result', repoName, res.events.length, 'events, isRunning:', res.isRunning)
      pollWatermarkRef.current[repoName] = res.events.length
      dispatch({ type: 'STREAM_LOAD', repoName, events: res.events, isRunning: res.isRunning })
      if (res.isRunning) {
        logger.log('starting poll for', repoName, 'count:', res.events.length)
        const timer = setInterval(async () => {
          try {
            const pollRes = await engineApi.readAgentLog(repoName)
            const prevCount = pollWatermarkRef.current[repoName] ?? 0
            logger.log('poll: read for', repoName, 'logEvents:', pollRes.events.length, 'prevDispatched:', prevCount, 'isRunning:', pollRes.isRunning)
            if (!pollRes.isRunning) {
              if (pollRes.events.length > prevCount) {
                const newEvents = pollRes.events.slice(prevCount)
                logger.log('poll: final new events for', repoName, 'count:', newEvents.length)
                for (const ev of newEvents) {
                  dispatch({ type: 'STREAM_EVENT', repoName, event: ev })
                }
              }
              dispatch({ type: 'STREAM_DONE', repoName, result: null })
              const t = pollTimersRef.current.get(repoName)
              if (t) { clearInterval(t); pollTimersRef.current.delete(repoName) }
              delete pollWatermarkRef.current[repoName]
              return
            }
            if (pollRes.events.length > prevCount) {
              const newEvents = pollRes.events.slice(prevCount)
              logger.log('poll: new events for', repoName, 'count:', newEvents.length, 'types:', newEvents.map(e => e.t).join(','))
              for (const ev of newEvents) {
                dispatch({ type: 'STREAM_EVENT', repoName, event: ev })
              }
              pollWatermarkRef.current[repoName] = pollRes.events.length
            }
          } catch (pollErr) {
            logger.error('poll failed for', repoName, pollErr)
          }
        }, 2000)
        pollTimersRef.current.set(repoName, timer)
      } else if (res.events.length > 0) {
        dispatch({ type: 'STREAM_DONE', repoName, result: null })
      }
    } catch (err) {
      logger.error('loadAgentLog failed', repoName, err)
      dispatch({ type: 'STREAM_LOAD', repoName, events: [], isRunning: false })
    }
  }, [])

  const clearResult = useCallback((repoName: string) => {
    const timer = pollTimersRef.current.get(repoName)
    if (timer) { clearInterval(timer); pollTimersRef.current.delete(repoName) }
    delete pollWatermarkRef.current[repoName]
    dispatch({ type: 'STREAM_CLEAR', repoName })
  }, [])

  const isStreamLive = useCallback((repoName: string): boolean => {
    return !!state.streamLive[repoName]
  }, [state.streamLive])

  const getStreamEvents = useCallback((repoName: string): AgentStreamEvent[] => {
    return state.streamEvents[repoName] ?? []
  }, [state.streamEvents])

  // Expose raw streamResults for HomePage side effects (auto-confirm, refresh triggers)
  const streamResults = useMemo(() => state.streamResults, [state.streamResults])

  return {
    resolveResults: state.resolveResults,
    isStreamLive,
    getStreamEvents,
    startResolve,
    loadAgentLog,
    clearResult,
    streamResults
  }
}
