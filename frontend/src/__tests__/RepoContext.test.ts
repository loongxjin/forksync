import { describe, it, expect } from 'vitest'
import { repoReducer, initialState } from '@/contexts/RepoContext'
import type { RepoAction } from '@/contexts/RepoContext'
import type { Repo, ScannedRepo, SyncResult } from '@shared/types/engine'

describe('repoReducer', () => {
  it('returns initial state for unknown action', () => {
    const state = repoReducer(initialState, { type: 'UNKNOWN' } as unknown as RepoAction)
    expect(state).toBe(initialState)
  })

  describe('SET_LOADING', () => {
    it('sets loading and clears error', () => {
      const prev = { ...initialState, error: 'old error' }
      const next = repoReducer(prev, { type: 'SET_LOADING', loading: true })
      expect(next.loading).toBe(true)
      expect(next.error).toBeNull()
    })
  })

  describe('SET_INITIALIZED', () => {
    it('sets initialized to true', () => {
      const next = repoReducer(initialState, { type: 'SET_INITIALIZED' })
      expect(next.initialized).toBe(true)
    })
  })

  describe('SET_REPOS', () => {
    it('sets repos, loading=false, initialized=true', () => {
      const repos = [{ id: '1', name: 'my-repo' }] as Repo[]
      const next = repoReducer(
        { ...initialState, loading: true },
        { type: 'SET_REPOS', repos }
      )
      expect(next.repos).toBe(repos)
      expect(next.loading).toBe(false)
      expect(next.initialized).toBe(true)
    })
  })

  describe('SET_REPOS_SILENT', () => {
    it('sets repos without changing loading/initialized', () => {
      const repos = [{ id: '1', name: 'my-repo' }] as Repo[]
      const next = repoReducer(initialState, { type: 'SET_REPOS_SILENT', repos })
      expect(next.repos).toBe(repos)
      expect(next.loading).toBe(false)
      expect(next.initialized).toBe(false)
    })
  })

  describe('SET_SCANNED', () => {
    it('sets scanned repos and loading=false', () => {
      const repos: ScannedRepo[] = [{ path: '/tmp/repo', name: 'repo', upstream: '', branches: [] }]
      const next = repoReducer(
        { ...initialState, loading: true },
        { type: 'SET_SCANNED', repos }
      )
      expect(next.scannedRepos).toBe(repos)
      expect(next.loading).toBe(false)
    })
  })

  describe('SET_SYNC_RESULTS', () => {
    it('sets sync results and loading=false', () => {
      const results: SyncResult[] = [{ name: 'repo1', status: 'synced' }]
      const next = repoReducer(
        { ...initialState, loading: true },
        { type: 'SET_SYNC_RESULTS', results }
      )
      expect(next.syncResults).toBe(results)
      expect(next.loading).toBe(false)
    })
  })

  describe('UPDATE_REPO', () => {
    it('updates matching repo by id', () => {
      const repos: Repo[] = [
        { id: '1', name: 'repo1', status: 'synced' } as Repo,
        { id: '2', name: 'repo2', status: 'error' } as Repo
      ]
      const prev = { ...initialState, repos }
      const updated = { ...repos[0], status: 'conflict' } as Repo
      const next = repoReducer(prev, { type: 'UPDATE_REPO', repo: updated })
      expect(next.repos[0].status).toBe('conflict')
      expect(next.repos[1].status).toBe('error')
    })
  })

  describe('SET_REPO_STATUS', () => {
    it('updates status for matching repo', () => {
      const repos: Repo[] = [
        { id: '1', name: 'repo1', status: 'synced' } as Repo,
        { id: '2', name: 'repo2', status: 'synced' } as Repo
      ]
      const prev = { ...initialState, repos }
      const next = repoReducer(prev, { type: 'SET_REPO_STATUS', repoId: '2', status: 'conflict' })
      expect(next.repos[0].status).toBe('synced')
      expect(next.repos[1].status).toBe('conflict')
    })
  })

  describe('REMOVE_REPO', () => {
    it('removes repo by id', () => {
      const repos: Repo[] = [
        { id: '1', name: 'repo1' } as Repo,
        { id: '2', name: 'repo2' } as Repo
      ]
      const prev = { ...initialState, repos }
      const next = repoReducer(prev, { type: 'REMOVE_REPO', repoId: '1' })
      expect(next.repos).toHaveLength(1)
      expect(next.repos[0].id).toBe('2')
    })
  })

  describe('SET_ERROR', () => {
    it('sets error and loading=false', () => {
      const next = repoReducer(
        { ...initialState, loading: true },
        { type: 'SET_ERROR', error: 'something broke' }
      )
      expect(next.error).toBe('something broke')
      expect(next.loading).toBe(false)
    })
  })
})
