import { describe, it, expect, vi, beforeEach } from 'vitest'
import { EventEmitter } from 'events'
import { Readable } from 'stream'

vi.mock('electron', () => ({ app: { isPackaged: false } }))
vi.mock('../logger', () => ({
  default: { info: vi.fn(), warn: vi.fn(), error: vi.fn(), debug: vi.fn() }
}))

import { EngineServer } from '../server'

type FakeChild = EventEmitter & {
  pid: number
  stdout: Readable
  stderr: Readable
  kill: () => void
}

function makeFakeChild(): FakeChild {
  const child = new EventEmitter() as FakeChild
  child.pid = 1000
  child.stdout = new Readable({ read() {} })
  child.stderr = new Readable({ read() {} })
  child.kill = vi.fn()
  return child
}

const mockFetch = vi.fn()

beforeEach(() => {
  mockFetch.mockReset()
  mockFetch.mockResolvedValue({ ok: true })
  vi.stubGlobal('fetch', mockFetch)
  vi.useFakeTimers()
})

describe('EngineServer crash supervision', () => {
  it('broadcasts starting then ready on a successful boot', async () => {
    const server = new EngineServer()
    const fake = makeFakeChild()
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    server.spawn = () => fake as any

    const statuses: string[] = []
    server.onStatusChange((s) => statuses.push(s))

    const ready = server.getBaseUrl()
    fake.stdout.push('FORKSYNC_HTTP_ADDR=127.0.0.1:54321\n')
    fake.stdout.push('FORKSYNC_TOKEN=abc123deadbeef\n')
    await vi.runAllTimersAsync()
    await ready

    expect(statuses).toContain('starting')
    expect(statuses).toContain('ready')
    expect(await server.getBaseUrl()).toBe('http://127.0.0.1:54321')
    expect(server.getToken()).toBe('abc123deadbeef')
  })

  it('does NOT respawn when kill() was called (app quitting)', async () => {
    const server = new EngineServer()
    let spawnCount = 0
    const fake = makeFakeChild()
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    server.spawn = () => { spawnCount++; return fake as any }

    const ready = server.getBaseUrl()
    fake.stdout.push('FORKSYNC_HTTP_ADDR=127.0.0.1:54321\n')
    await vi.runAllTimersAsync()
    await ready

    server.kill()
    fake.emit('close', 1)
    await vi.advanceTimersByTimeAsync(10_000)

    expect(spawnCount).toBe(1) // initial only, no respawn
  })
})
