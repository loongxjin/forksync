import { describe, it, expect, vi, beforeEach } from 'vitest'

// Mock the EngineServer so EngineClient gets a deterministic base URL without
// spawning any process.
const mockFetch = vi.fn()
vi.mock('../server', () => ({
  getEngineServer: () => ({
    getBaseUrl: async () => 'http://127.0.0.1:9999',
    getWsUrl: async (path: string) => `ws://127.0.0.1:9999${path}`
  })
}))

// Stub global fetch — Node 18+ provides it; we override to assert call shape.
vi.stubGlobal('fetch', mockFetch)

vi.mock('../logger', () => ({
  default: { info: vi.fn(), warn: vi.fn(), error: vi.fn(), debug: vi.fn() }
}))

import { EngineClient, EngineRequestError, EngineParseError } from '../engine'

function jsonResponse(body: unknown, ok = true): Response {
  return {
    ok,
    status: 200,
    text: async () => JSON.stringify(body)
  } as Response
}

beforeEach(() => {
  mockFetch.mockReset()
})

// ---------------------------------------------------------------------------
// Error type tests
// ---------------------------------------------------------------------------

describe('EngineClient error types', () => {
  it('EngineRequestError carries code', () => {
    const err = new EngineRequestError('Engine request failed: ECONNREFUSED', 'Error')
    expect(err.name).toBe('EngineRequestError')
    expect(err.code).toBe('Error')
    expect(err).toBeInstanceOf(Error)
  })

  it('EngineParseError carries body', () => {
    const err = new EngineParseError('Failed to parse engine output: unexpected token', 'raw output')
    expect(err.name).toBe('EngineParseError')
    expect(err.body).toBe('raw output')
  })
})

// ---------------------------------------------------------------------------
// HTTP method tests — verify each EngineClient method maps to the right route
// ---------------------------------------------------------------------------

describe('EngineClient HTTP routes', () => {
  it('GET /status with exclude query', async () => {
    mockFetch.mockResolvedValueOnce(jsonResponse({ success: true, data: { repos: [], agents: [], preferredAgent: '' } }))
    const c = new EngineClient()
    await c.status(['a', 'b'])
    const [url, init] = mockFetch.mock.calls[0]
    expect(url).toBe('http://127.0.0.1:9999/status?exclude=a%2Cb')
    expect(init.method).toBe('GET')
  })

  it('POST /sync/all', async () => {
    mockFetch.mockResolvedValueOnce(jsonResponse({ success: true, data: { results: [] } }))
    const c = new EngineClient()
    await c.syncAll()
    const [url, init] = mockFetch.mock.calls[0]
    expect(url).toBe('http://127.0.0.1:9999/sync/all')
    expect(init.method).toBe('POST')
  })

  it('POST /repos with body', async () => {
    mockFetch.mockResolvedValueOnce(jsonResponse({ success: true, data: { repo: { name: 'x' } } }))
    const c = new EngineClient()
    await c.add('/p', 'https://up', { localBranch: 'main', remoteBranch: 'master' })
    const [url, init] = mockFetch.mock.calls[0]
    expect(url).toBe('http://127.0.0.1:9999/repos')
    expect(init.method).toBe('POST')
    expect(JSON.parse(init.body)).toEqual({
      path: '/p',
      upstream: 'https://up',
      branchMapping: { localBranch: 'main', remoteBranch: 'master' }
    })
  })

  it('DELETE /repos/{name}', async () => {
    mockFetch.mockResolvedValueOnce(jsonResponse({ success: true, data: { removed: 'x' } }))
    const c = new EngineClient()
    await c.remove('my repo')
    const [url, init] = mockFetch.mock.calls[0]
    expect(url).toBe('http://127.0.0.1:9999/repos/my%20repo')
    expect(init.method).toBe('DELETE')
  })

  it('POST resolve maps prepare flag to mode', async () => {
    mockFetch.mockResolvedValueOnce(jsonResponse({ success: true, data: { repoId: 'x', conflicts: [] } }))
    const c = new EngineClient()
    await c.resolve('r', { prepare: true })
    const [, init] = mockFetch.mock.calls[0]
    expect(JSON.parse(init.body).mode).toBe('prepare')
  })

  it('PUT /config with key/value body', async () => {
    mockFetch.mockResolvedValueOnce(jsonResponse({ success: true, data: { key: 'agent.preferred', value: 'claude' } }))
    const c = new EngineClient()
    await c.configSet('agent.preferred', 'claude')
    const [url, init] = mockFetch.mock.calls[0]
    expect(url).toBe('http://127.0.0.1:9999/config')
    expect(init.method).toBe('PUT')
    expect(JSON.parse(init.body)).toEqual({ key: 'agent.preferred', value: 'claude' })
  })

  it('GET /history builds query string', async () => {
    mockFetch.mockResolvedValueOnce(jsonResponse({ success: true, data: { records: [] } }))
    const c = new EngineClient()
    await c.history('myrepo', 5)
    const [url] = mockFetch.mock.calls[0]
    expect(url).toContain('/history?')
    expect(url).toContain('repo=myrepo')
    expect(url).toContain('limit=5')
  })

  it('surfaces a non-JSON body as EngineParseError', async () => {
    mockFetch.mockResolvedValueOnce({ ok: true, status: 200, text: async () => 'not-json' } as Response)
    const c = new EngineClient()
    await expect(c.status()).rejects.toBeInstanceOf(EngineParseError)
  })

  it('surfaces a fetch failure as EngineRequestError', async () => {
    mockFetch.mockRejectedValueOnce(Object.assign(new Error('boom'), { name: 'Error' }))
    const c = new EngineClient()
    await expect(c.status()).rejects.toBeInstanceOf(EngineRequestError)
  })
})
