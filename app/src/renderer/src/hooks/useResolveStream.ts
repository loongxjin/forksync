/**
 * useResolveStream — unified resolve state management hook
 *
 * The disk NDJSON log is the SINGLE source of truth for agent output.
 * The WebSocket stream is just a "push" notification — when it fires we
 * re-read the disk log. This guarantees no duplication and no data loss
 * whether the stream is live, reconnected, or replayed after restart.
 *
 * Both auto-resolve (sync) and manual-resolve paths write to the same disk
 * log (including a terminal `done` frame), so the frontend reading logic is
 * identical for both.
 */

import { useReducer, useCallback, useRef, useEffect, useMemo } from 'react'
import type { ResolveData, AgentStreamEvent, AgentResolveResult, ConflictFile } from '@shared/types/engine'
import { engineApi } from '@/lib/api'
import { useLogger } from '@/hooks/useLogger'

// ---------------------------------------------------------------------------
// Stream State & Reducer (exported for testing)
// ---------------------------------------------------------------------------

export interface StreamState {
  /** Agent events from the disk log (full replacement, never appended). */
  streamEvents: Record<string, AgentStreamEvent[]>
  /** Whether a repo's agent log is still being written to (isRunning). */
  streamLive: Record<string, boolean>
  /** Raw stream outcomes — used by HomePage for side effects (refresh, auto-confirm). */
  streamResults: Record<string, ResolveData | null>
  /** Resolve details (AI summary, conflicts, diff) — restored from done frame. */
  resolveResults: Record<string, ResolveData>
}

export type StreamAction =
  | { type: 'STREAM_LOAD'; repoName: string; events: AgentStreamEvent[]; isRunning: boolean }
  | { type: 'STREAM_DONE'; repoName: string; result?: ResolveData | null }
  | { type: 'STREAM_CLEAR'; repoName: string }

export const initialStreamState: StreamState = {
  streamEvents: {},
  streamLive: {},
  streamResults: {},
  resolveResults: {}
}

/**
 * Extract ResolveData from the last `done` event in a log. Returns null if
 * there is no done frame (agent still running, or log is from an old path
 * that didn't write done frames).
 */
export function extractResolveDataFromEvents(events: AgentStreamEvent[]): ResolveData | null {
  for (let i = events.length - 1; i >= 0; i--) {
    const ev = events[i]
    if (ev.t === 'done') {
      const resolvedFiles: string[] = (ev as any).resolvedFiles ?? []
      const conflicts: ConflictFile[] = resolvedFiles.map((f: string) => ({ path: f }))
      const agentResult: AgentResolveResult = {
        success: ev.success !== false,
        summary: (ev as any).summary ?? '',
        sessionId: (ev as any).session_id ?? '',
        agentName: (ev as any).agentName ?? '',
        resolvedFiles,
        diff: (ev as any).diff ?? ''
      }
      return {
        repoId: '',
        conflicts,
        agentResult
      }
    }
  }
  return null
}

export function streamReducer(state: StreamState, action: StreamAction): StreamState {
  switch (action.type) {
    case 'STREAM_LOAD': {
      // Full replacement — disk log is authoritative.
      const resolveData = extractResolveDataFromEvents(action.events)
      const nextResolve = { ...state.resolveResults }
      if (resolveData) {
        nextResolve[action.repoName] = resolveData
      }
      return {
        ...state,
        streamEvents: { ...state.streamEvents, [action.repoName]: action.events },
        streamLive: { ...state.streamLive, [action.repoName]: action.isRunning },
        resolveResults: nextResolve
      }
    }
    case 'STREAM_DONE': {
      const nextStream = { ...state.streamResults }
      const nextResolve = { ...state.resolveResults }
      const nextLive = { ...state.streamLive }
      delete nextLive[action.repoName]
      if (action.result !== undefined) {
        nextStream[action.repoName] = action.result
        if (action.result) {
          nextResolve[action.repoName] = action.result
        }
      }
      return {
        ...state,
        streamResults: nextStream,
        resolveResults: nextResolve,
        streamLive: nextLive
      }
    }
    case 'STREAM_CLEAR': {
      const nextEvents = { ...state.streamEvents }
      const nextLive = { ...state.streamLive }
      const nextStream = { ...state.streamResults }
      const nextResolve = { ...state.resolveResults }
      delete nextEvents[action.repoName]
      delete nextLive[action.repoName]
      delete nextStream[action.repoName]
      delete nextResolve[action.repoName]
      return {
        streamEvents: nextEvents,
        streamLive: nextLive,
        streamResults: nextStream,
        resolveResults: nextResolve
      }
    }
    default:
      return state
  }
}

// ---------------------------------------------------------------------------
// Hook Interface
// ---------------------------------------------------------------------------

export interface ResolveStreamHook {
  /** Resolve details restored from disk log done frames + stream done. */
  resolveResults: Record<string, ResolveData>
  /** Whether a repo's agent log is still being written to. */
  isStreamLive: (repoName: string) => boolean
  /** Get stream events for a repo. */
  getStreamEvents: (repoName: string) => AgentStreamEvent[]
  /** Trigger manual resolve: prepare + WS start + disk poll. */
  startResolve: (repoName: string, opts?: { agent?: string; noConfirm?: boolean }) => Promise<void>
  /** Load existing agent log from disk with optional polling. */
  loadAgentLog: (repoName: string) => Promise<void>
  /** Clear all stream state for a repo. */
  clearResult: (repoName: string) => void
  /** Raw stream results (for HomePage side effects). */
  streamResults: Record<string, ResolveData | null>
}

// ---------------------------------------------------------------------------
// Hook Implementation
// ---------------------------------------------------------------------------

export function useResolveStream(): ResolveStreamHook {
  const logger = useLogger('ResolveStream')
  const [state, dispatch] = useReducer(streamReducer, initialStreamState)

  const ipcSetupRef = useRef(false)
  const pollTimersRef = useRef<Map<string, ReturnType<typeof setInterval>>>(new Map())
  /** Repos currently being polled — prevents duplicate poll timers. */
  const pollingRef = useRef<Set<string>>(new Set())

  // ---------------------------------------------------------------------------
  // Core: read disk log → dispatch STREAM_LOAD
  // ---------------------------------------------------------------------------
  const readDiskLog = useCallback(async (repoName: string): Promise<boolean> => {
    try {
      const res = await engineApi.readAgentLog(repoName)
      logger.log('readDiskLog', repoName, res.events.length, 'events, isRunning:', res.isRunning)
      dispatch({ type: 'STREAM_LOAD', repoName, events: res.events, isRunning: res.isRunning })

      // Only fire STREAM_DONE when the log has events AND the agent is no
      // longer running. An empty log with isRunning=false means the log file
      // hasn't been created yet (race at stream start) — don't terminate.
      if (!res.isRunning && res.events.length > 0) {
        const resolveData = extractResolveDataFromEvents(res.events)
        dispatch({ type: 'STREAM_DONE', repoName, result: resolveData })
        // Stop polling — agent finished.
        const timer = pollTimersRef.current.get(repoName)
        if (timer) { clearInterval(timer); pollTimersRef.current.delete(repoName) }
        pollingRef.current.delete(repoName)
      }
      return res.isRunning
    } catch (err) {
      logger.error('readDiskLog failed', repoName, err)
      return false
    }
  }, [logger])

  // ---------------------------------------------------------------------------
  // Start / stop disk polling for a repo
  // ---------------------------------------------------------------------------
  const startPolling = useCallback((repoName: string): void => {
    if (pollingRef.current.has(repoName)) return
    pollingRef.current.add(repoName)
    logger.log('startPolling', repoName)

    const timer = setInterval(() => {
      readDiskLog(repoName).catch((err) => logger.error('poll error', repoName, err))
    }, 2000)
    pollTimersRef.current.set(repoName, timer)
  }, [logger, readDiskLog])

  const stopPolling = useCallback((repoName: string): void => {
    pollingRef.current.delete(repoName)
    const timer = pollTimersRef.current.get(repoName)
    if (timer) { clearInterval(timer); pollTimersRef.current.delete(repoName) }
  }, [])

  // ---------------------------------------------------------------------------
  // IPC listeners — WS tick triggers disk re-read; WS done delivers payload
  // ---------------------------------------------------------------------------
  useEffect(() => {
    if (ipcSetupRef.current) return
    ipcSetupRef.current = true

    const unsubTick = engineApi.onResolveStreamTick((repoName) => {
      logger.log('tick received', repoName)
      // Immediate disk re-read — no need to wait for the 2s poll interval.
      readDiskLog(repoName).catch((err) => logger.error('tick read error', repoName, err))
    })

    const unsubDone = engineApi.onResolveStreamDone((repoName, apiRes) => {
      logger.log('stream done received', repoName, apiRes.success)
      // Final disk read to capture the done frame, then stop polling.
      readDiskLog(repoName).catch(() => {})
      stopPolling(repoName)
      dispatch({ type: 'STREAM_DONE', repoName, result: apiRes.success ? apiRes.data : null })
    })

    const unsubError = engineApi.onResolveStreamError((repoName, error) => {
      logger.error('stream error received', repoName, error)
      readDiskLog(repoName).catch(() => {})
      stopPolling(repoName)
      dispatch({ type: 'STREAM_DONE', repoName, result: null })
    })

    return () => {
      ipcSetupRef.current = false
      unsubTick()
      unsubDone()
      unsubError()
    }
  }, [logger, readDiskLog, stopPolling])

  // Cleanup all poll timers on unmount
  useEffect(() => {
    return () => {
      pollTimersRef.current.forEach((timer) => { clearInterval(timer) })
      pollTimersRef.current.clear()
      pollingRef.current.clear()
    }
  }, [])

  // ---------------------------------------------------------------------------
  // Public operations
  // ---------------------------------------------------------------------------

  const startResolve = useCallback(async (
    repoName: string,
    opts?: { agent?: string; noConfirm?: boolean }
  ): Promise<void> => {
    logger.log('startResolve', repoName, opts)
    dispatch({ type: 'STREAM_CLEAR', repoName })
    try {
      engineApi.resolveStreamStart(repoName, opts)
      logger.log('resolveStreamStart sent — WS will tick, disk poll starting')
      // Start disk polling immediately. The WS tick will trigger extra reads
      // for real-time responsiveness, but the poll ensures we catch events
      // even if a tick is missed.
      startPolling(repoName)
      // Also do an immediate read in case the agent writes to disk fast.
      readDiskLog(repoName).catch(() => {})
    } catch (err) {
      logger.error('resolveStreamStart failed', repoName, err)
      dispatch({ type: 'STREAM_DONE', repoName, result: null })
    }
  }, [logger, startPolling, readDiskLog])

  const loadAgentLog = useCallback(async (repoName: string): Promise<void> => {
    logger.log('loadAgentLog', repoName)
    const isRunning = await readDiskLog(repoName)
    // If the agent is still running, start polling.
    if (isRunning) {
      startPolling(repoName)
    }
  }, [logger, readDiskLog, startPolling])

  const clearResult = useCallback((repoName: string) => {
    stopPolling(repoName)
    dispatch({ type: 'STREAM_CLEAR', repoName })
  }, [stopPolling])

  const isStreamLive = useCallback((repoName: string): boolean => {
    return !!state.streamLive[repoName]
  }, [state.streamLive])

  const getStreamEvents = useCallback((repoName: string): AgentStreamEvent[] => {
    return state.streamEvents[repoName] ?? []
  }, [state.streamEvents])

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
