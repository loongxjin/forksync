import { describe, it, expect, vi } from 'vitest'
import { historyReducer, initialState } from '@/contexts/HistoryContext'
import type { HistoryAction } from '@/contexts/HistoryContext'
import type { SyncHistoryRecord } from '@shared/types/engine'

describe('historyReducer', () => {
  it('returns initial state for unknown action', () => {
    const state = historyReducer(initialState, { type: 'UNKNOWN' } as unknown as HistoryAction)
    expect(state).toBe(initialState)
  })

  describe('SET_LOADING', () => {
    it('sets loading', () => {
      const next = historyReducer(initialState, { type: 'SET_LOADING', loading: true })
      expect(next.loading).toBe(true)
    })
  })

  describe('SET_RECORDS', () => {
    it('sets records, loading=false, initialized=true, lastLoadAt to now', () => {
      vi.useFakeTimers()
      vi.setSystemTime(new Date('2026-05-20T10:00:00Z'))

      const records: SyncHistoryRecord[] = [
        { id: 1, repoName: 'repo1', status: 'synced', startedAt: '2026-01-01', finishedAt: '2026-01-01' }
      ] as SyncHistoryRecord[]
      const prev = { ...initialState, loading: true }
      const next = historyReducer(prev, { type: 'SET_RECORDS', records })

      expect(next.records).toBe(records)
      expect(next.loading).toBe(false)
      expect(next.initialized).toBe(true)
      expect(next.lastLoadAt).toBe(new Date('2026-05-20T10:00:00Z').getTime())

      vi.useRealTimers()
    })
  })

  describe('CLEAR', () => {
    it('clears records and resets initialized', () => {
      const prev = {
        ...initialState,
        records: [{ id: 1 }] as SyncHistoryRecord[],
        initialized: true
      }
      const next = historyReducer(prev, { type: 'CLEAR' })
      expect(next.records).toEqual([])
      expect(next.initialized).toBe(false)
    })
  })

  describe('SET_ERROR', () => {
    it('sets loading=false', () => {
      const next = historyReducer(
        { ...initialState, loading: true },
        { type: 'SET_ERROR' }
      )
      expect(next.loading).toBe(false)
    })
  })

  describe('UPDATE_RECORD', () => {
    it('merges updates for matching repoName', () => {
      const records: SyncHistoryRecord[] = [
        { id: 1, repoName: 'repo1', status: 'synced', startedAt: '2026-01-01' },
        { id: 2, repoName: 'repo2', status: 'synced', startedAt: '2026-01-01' }
      ] as SyncHistoryRecord[]
      const prev = { ...initialState, records }
      const next = historyReducer(prev, {
        type: 'UPDATE_RECORD',
        repoName: 'repo1',
        updates: { status: 'error' }
      })
      expect(next.records[0].status).toBe('error')
      expect(next.records[1].status).toBe('synced')
    })

    it('does not modify records when repoName not found', () => {
      const records: SyncHistoryRecord[] = [
        { id: 1, repoName: 'repo1', status: 'synced', startedAt: '2026-01-01' }
      ] as SyncHistoryRecord[]
      const prev = { ...initialState, records }
      const next = historyReducer(prev, {
        type: 'UPDATE_RECORD',
        repoName: 'nonexistent',
        updates: { status: 'error' }
      })
      expect(next.records[0].status).toBe('synced')
    })
  })
})
