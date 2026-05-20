import { describe, it, expect, vi } from 'vitest'

// Mock electron before importing the module under test
vi.mock('electron', () => ({
  ipcMain: { handle: vi.fn(), on: vi.fn() },
  app: { isPackaged: true, getPath: vi.fn(() => '/tmp') }
}))

vi.mock('../logger', () => ({
  default: { info: vi.fn(), warn: vi.fn(), error: vi.fn() }
}))

vi.mock('../engine', () => ({
  EngineClient: vi.fn()
}))

vi.mock('../notify', () => ({
  notifySyncResults: vi.fn(),
  updateNotificationConfig: vi.fn()
}))

import { assertString, assertOptionalString, assertSafePath } from '../ipc-engine'

describe('assertString', () => {
  it('returns valid non-empty string', () => {
    expect(assertString('hello', 'test')).toBe('hello')
  })

  it('rejects undefined', () => {
    expect(() => assertString(undefined, 'field')).toThrow('Invalid field: expected non-empty string')
  })

  it('rejects null', () => {
    expect(() => assertString(null, 'field')).toThrow('Invalid field: expected non-empty string')
  })

  it('rejects empty string', () => {
    expect(() => assertString('', 'field')).toThrow('Invalid field: expected non-empty string')
  })

  it('rejects number', () => {
    expect(() => assertString(123, 'field')).toThrow('Invalid field: expected non-empty string')
  })

  it('rejects string exceeding 4096 chars', () => {
    const long = 'a'.repeat(4097)
    expect(() => assertString(long, 'field')).toThrow('Invalid field: exceeds maximum length')
  })

  it('accepts string at exactly 4096 chars', () => {
    const exact = 'a'.repeat(4096)
    expect(assertString(exact, 'field')).toBe(exact)
  })
})

describe('assertOptionalString', () => {
  it('returns undefined for undefined', () => {
    expect(assertOptionalString(undefined, 'test')).toBeUndefined()
  })

  it('returns undefined for null', () => {
    expect(assertOptionalString(null, 'test')).toBeUndefined()
  })

  it('returns valid string', () => {
    expect(assertOptionalString('hello', 'test')).toBe('hello')
  })

  it('rejects number', () => {
    expect(() => assertOptionalString(42, 'field')).toThrow('Invalid field: expected string')
  })

  it('accepts empty string (optional, not validated for emptiness)', () => {
    expect(assertOptionalString('', 'test')).toBe('')
  })

  it('rejects string exceeding 4096 chars', () => {
    const long = 'b'.repeat(4097)
    expect(() => assertOptionalString(long, 'field')).toThrow('Invalid field: exceeds maximum length')
  })
})

describe('assertSafePath', () => {
  it('resolves a normal path', () => {
    const result = assertSafePath('/tmp/test', 'path')
    expect(result).toContain('tmp')
    expect(result).toContain('test')
  })

  it('rejects path traversal with ..', () => {
    expect(() => assertSafePath('/tmp/../etc/passwd', 'path')).toThrow('path traversal detected')
  })

  it('rejects non-string input', () => {
    expect(() => assertSafePath(123, 'path')).toThrow('Invalid path: expected non-empty string')
  })

  it('rejects empty string', () => {
    expect(() => assertSafePath('', 'path')).toThrow('Invalid path: expected non-empty string')
  })

  it('rejects string exceeding 4096 chars', () => {
    const long = '/'.repeat(4097)
    expect(() => assertSafePath(long, 'path')).toThrow('Invalid path: exceeds maximum length')
  })
})
