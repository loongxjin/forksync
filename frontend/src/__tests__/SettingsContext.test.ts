import { describe, it, expect } from 'vitest'
import { settingsReducer, initialState } from '@/contexts/SettingsContext'
import type { SettingsAction } from '@/contexts/SettingsContext'

describe('settingsReducer', () => {
  it('returns initial state for unknown action', () => {
    const state = settingsReducer(initialState, { type: 'UNKNOWN' } as unknown as SettingsAction)
    expect(state).toBe(initialState)
  })

  describe('SET_THEME', () => {
    it('sets theme', () => {
      const next = settingsReducer(initialState, { type: 'SET_THEME', theme: 'light' })
      expect(next.theme).toBe('light')
    })
  })

  describe('SET_IDE_CONFIG', () => {
    it('sets ideConfig and ideLoading=false', () => {
      const config = { defaultIDE: 'vscode', detectedIDEs: [], customIDEs: [] }
      const prev = { ...initialState, ideLoading: true }
      const next = settingsReducer(prev, { type: 'SET_IDE_CONFIG', config } as any)
      expect(next.ideConfig).toBe(config)
      expect(next.ideLoading).toBe(false)
    })
  })

  describe('SET_IDE_LOADING', () => {
    it('sets ideLoading', () => {
      const next = settingsReducer(initialState, { type: 'SET_IDE_LOADING', loading: true })
      expect(next.ideLoading).toBe(true)
    })
  })

  describe('SET_IDE_ERROR', () => {
    it('sets ideError and ideLoading=false', () => {
      const prev = { ...initialState, ideLoading: true }
      const next = settingsReducer(prev, { type: 'SET_IDE_ERROR', error: 'IDE detection failed' })
      expect(next.ideError).toBe('IDE detection failed')
      expect(next.ideLoading).toBe(false)
    })
  })

  describe('SET_ENGINE_CONFIG', () => {
    it('sets engineConfig, configLoading=false, configError=null', () => {
      const config = { sync: {}, agent: {} } as any
      const prev = { ...initialState, configLoading: true, configError: 'old error' }
      const next = settingsReducer(prev, { type: 'SET_ENGINE_CONFIG', config })
      expect(next.engineConfig).toBe(config)
      expect(next.configLoading).toBe(false)
      expect(next.configError).toBeNull()
    })
  })

  describe('SET_CONFIG_LOADING', () => {
    it('sets configLoading', () => {
      const next = settingsReducer(initialState, { type: 'SET_CONFIG_LOADING', loading: true })
      expect(next.configLoading).toBe(true)
    })
  })

  describe('SET_CONFIG_ERROR', () => {
    it('sets configError and configLoading=false', () => {
      const prev = { ...initialState, configLoading: true }
      const next = settingsReducer(prev, { type: 'SET_CONFIG_ERROR', error: 'failed' })
      expect(next.configError).toBe('failed')
      expect(next.configLoading).toBe(false)
    })
  })
})
