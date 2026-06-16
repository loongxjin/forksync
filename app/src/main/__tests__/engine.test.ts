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

// ---------------------------------------------------------------------------
// Route coverage for the remaining EngineClient methods (appended)
// ---------------------------------------------------------------------------
function stubOk(data: unknown): void {
  mockFetch.mockResolvedValueOnce(jsonResponse({ success: true, data }))
}
function firstCall(): [string, RequestInit] {
  return mockFetch.mock.calls[0] as [string, RequestInit]
}

describe('EngineClient remaining routes', () => {
  beforeEach(() => mockFetch.mockReset())

  it('scan → POST /scan {dir}', async () => {
    stubOk({ repos: [] })
    await new EngineClient().scan('/some/dir')
    const [url, init] = firstCall()
    expect(url).toBe('http://127.0.0.1:9999/scan')
    expect(init.method).toBe('POST')
    expect(JSON.parse(init.body as string)).toEqual({ dir: '/some/dir' })
  })

  it('syncRepo → POST /sync/repos/{name}', async () => {
    stubOk({ results: [] })
    await new EngineClient().syncRepo('my repo')
    const [url, init] = firstCall()
    expect(url).toBe('http://127.0.0.1:9999/sync/repos/my%20repo')
    expect(init.method).toBe('POST')
  })

  it('resolvePrepare → mode=prepare', async () => {
    stubOk({ repoId: 'r', conflicts: [] })
    await new EngineClient().resolvePrepare('r')
    expect(JSON.parse(firstCall()[1].body as string).mode).toBe('prepare')
  })

  it('resolveAccept → mode=accept', async () => {
    stubOk({ repoId: 'r', resolved: true })
    await new EngineClient().resolveAccept('r')
    expect(JSON.parse(firstCall()[1].body as string).mode).toBe('accept')
  })

  it('resolveReject → mode=reject', async () => {
    stubOk({ repoId: 'r', rolledBack: true })
    await new EngineClient().resolveReject('r')
    expect(JSON.parse(firstCall()[1].body as string).mode).toBe('reject')
  })

  it('agentList → GET /agents', async () => {
    stubOk({ agents: [], preferred: '' })
    await new EngineClient().agentList()
    expect(firstCall()[0]).toBe('http://127.0.0.1:9999/agents')
    expect(firstCall()[1].method).toBe('GET')
  })

  it('agentSessions → GET /agents/sessions', async () => {
    stubOk({ sessions: [] })
    await new EngineClient().agentSessions()
    expect(firstCall()[0]).toBe('http://127.0.0.1:9999/agents/sessions')
  })

  it('agentCleanup → POST /agents/cleanup', async () => {
    stubOk({ removed: 0 })
    await new EngineClient().agentCleanup()
    expect(firstCall()[0]).toBe('http://127.0.0.1:9999/agents/cleanup')
    expect(firstCall()[1].method).toBe('POST')
  })

  it('agentReset → POST /agents/{name}/reset', async () => {
    stubOk({ repoId: 'r', cleared: true })
    await new EngineClient().agentReset('r')
    expect(firstCall()[0]).toBe('http://127.0.0.1:9999/agents/r/reset')
  })

  it('historyCleanup → POST /history/cleanup', async () => {
    stubOk({ message: 'ok' })
    await new EngineClient().historyCleanup({ keepDays: 7 })
    expect(JSON.parse(firstCall()[1].body as string)).toEqual({ repo: undefined, keepDays: 7 })
  })

  it('configGet → GET /config', async () => {
    stubOk({})
    await new EngineClient().configGet()
    expect(firstCall()[0]).toBe('http://127.0.0.1:9999/config')
    expect(firstCall()[1].method).toBe('GET')
  })

  it('postSyncList → GET /repos/{name}/post-sync', async () => {
    stubOk({ commands: [] })
    await new EngineClient().postSyncList('r')
    expect(firstCall()[0]).toBe('http://127.0.0.1:9999/repos/r/post-sync')
  })

  it('postSyncAdd → POST with name+cmd body', async () => {
    stubOk({ commands: [] })
    await new EngineClient().postSyncAdd('r', 'build', 'npm run build')
    expect(JSON.parse(firstCall()[1].body as string)).toEqual({ name: 'build', cmd: 'npm run build' })
  })

  it('postSyncRemove → DELETE with id body', async () => {
    stubOk({ commands: [] })
    await new EngineClient().postSyncRemove('r', 'cmd-1')
    const [url, init] = firstCall()
    expect(init.method).toBe('DELETE')
    expect(url).toBe('http://127.0.0.1:9999/repos/r/post-sync')
    expect(JSON.parse(init.body as string)).toEqual({ id: 'cmd-1' })
  })

  it('summarize → POST /repos/{name}/summarize {retry:false}', async () => {
    stubOk({ historyId: 1, repoName: 'r', summary: '', summaryStatus: 'done' })
    await new EngineClient().summarize('r')
    expect(firstCall()[0]).toBe('http://127.0.0.1:9999/repos/r/summarize')
    expect(JSON.parse(firstCall()[1].body as string).retry).toBe(false)
  })

  it('summarizeRetry → retry:true', async () => {
    stubOk({ historyId: 1, repoName: 'r', summary: '', summaryStatus: 'done' })
    await new EngineClient().summarizeRetry('r')
    expect(JSON.parse(firstCall()[1].body as string).retry).toBe(true)
  })

  it('readAgentLog → GET agent-log (bare object)', async () => {
    mockFetch.mockResolvedValueOnce({ ok: true, status: 200, json: async () => ({ events: [], isRunning: false }) } as Response)
    const res = await new EngineClient().readAgentLog('r')
    expect(firstCall()[0]).toBe('http://127.0.0.1:9999/repos/r/agent-log')
    expect(res).toEqual({ events: [], isRunning: false })
  })

  it('repoDiff → GET diff (bare object)', async () => {
    mockFetch.mockResolvedValueOnce({ ok: true, status: 200, json: async () => ({ success: true, diff: 'changes' }) } as Response)
    const res = await new EngineClient().repoDiff('r')
    expect(firstCall()[0]).toBe('http://127.0.0.1:9999/repos/r/diff')
    expect(res.success).toBe(true)
  })
})
