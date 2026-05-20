import { describe, it, expect, vi } from 'vitest'

// Mock electron
vi.mock('electron', () => ({
  app: {
    isPackaged: false,
    getPath: vi.fn(() => '/tmp')
  }
}))

vi.mock('../logger', () => ({
  default: { info: vi.fn(), warn: vi.fn(), error: vi.fn() }
}))

import {
  EngineClient,
  EngineTimeoutError,
  EngineProcessError,
  EngineParseError,
  EngineSpawnError
} from '../engine'

// ---------------------------------------------------------------------------
// Error type tests — these are simple classes, no mocking needed
// ---------------------------------------------------------------------------

describe('EngineClient error types', () => {
  it('EngineTimeoutError has correct name and message', () => {
    const err = new EngineTimeoutError('timed out after 30000ms')
    expect(err.name).toBe('EngineTimeoutError')
    expect(err.message).toBe('timed out after 30000ms')
    expect(err).toBeInstanceOf(Error)
  })

  it('EngineProcessError has exitCode and stderr', () => {
    const err = new EngineProcessError('Engine exited with code 1', 1, 'fatal: not a git repository')
    expect(err.name).toBe('EngineProcessError')
    expect(err.exitCode).toBe(1)
    expect(err.stderr).toBe('fatal: not a git repository')
    expect(err).toBeInstanceOf(Error)
  })

  it('EngineProcessError handles null exitCode', () => {
    const err = new EngineProcessError('killed', null, '')
    expect(err.exitCode).toBeNull()
  })

  it('EngineParseError has stdout and stderr', () => {
    const err = new EngineParseError('Failed to parse engine output: unexpected token', 'raw output', 'some stderr')
    expect(err.name).toBe('EngineParseError')
    expect(err.stdout).toBe('raw output')
    expect(err.stderr).toBe('some stderr')
  })

  it('EngineSpawnError has correct name and message', () => {
    const err = new EngineSpawnError('Failed to spawn engine: ENOENT')
    expect(err.name).toBe('EngineSpawnError')
    expect(err.message).toBe('Failed to spawn engine: ENOENT')
  })
})

// ---------------------------------------------------------------------------
// buildArgs tests — test the dev vs packaged path
// ---------------------------------------------------------------------------

describe('EngineClient.buildArgs', () => {
  it('adds --json and go run prefix in dev mode (isPackaged=false)', () => {
    const client = new EngineClient()
    const args = (client as any).buildArgs(['status'])
    expect(args).toEqual(['run', '.', 'status', '--json'])
  })

  it('builds correct args for sync command', () => {
    const client = new EngineClient()
    const args = (client as any).buildArgs(['sync'])
    expect(args).toEqual(['run', '.', 'sync', '--json'])
  })

  it('builds correct args for multi-word commands', () => {
    const client = new EngineClient()
    const args = (client as any).buildArgs(['resolve', 'my-repo', '--accept'])
    expect(args).toEqual(['run', '.', 'resolve', 'my-repo', '--accept', '--json'])
  })
})
