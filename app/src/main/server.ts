/**
 * EngineServer — manages the embedded Go HTTP server process.
 *
 * Electron spawns a single long-lived `forksync` binary (or `go run .` in dev)
 * which serves the engine over HTTP + WebSocket on 127.0.0.1:<random-port>.
 * The server announces its address by printing a single
 * `FORKSYNC_HTTP_ADDR=127.0.0.1:<port>` line to stdout; this class reads that
 * line, polls /healthz until ready, and exposes the base URL to EngineClient.
 *
 * On app quit the server process is SIGTERM'd.
 *
 * Crash supervision: if the child exits with a non-zero code after a successful
 * start, the server is respawned with exponential backoff (500ms → 1s → 2s →
 * 5s cap, up to MAX_RESTART_ATTEMPTS times). Status listeners are notified on
 * every reconnect so the renderer can show a "reconnecting" banner. If the
 * backoff is exhausted, the server is left down and listeners get 'down'.
 */

import { app } from 'electron'
import { join } from 'path'
import { spawn as cpSpawn, exec, type ChildProcess } from 'child_process'
import { createInterface } from 'readline'
import log from './logger'

const ADDR_PREFIX = 'FORKSYNC_HTTP_ADDR='
const TOKEN_PREFIX = 'FORKSYNC_TOKEN='
const STARTUP_TIMEOUT_MS = 30_000
const HEALTH_POLL_INTERVAL_MS = 100

// Crash-supervisor backoff schedule (ms). Each entry is the wait before the
// next respawn attempt; after the last entry, attempts stop.
const RESTART_BACKOFF_MS = [500, 1_000, 2_000, 5_000, 5_000]
const MAX_RESTART_ATTEMPTS = RESTART_BACKOFF_MS.length

/** Lifecycle status broadcast to status listeners (e.g. the renderer). */
export type EngineStatus = 'starting' | 'ready' | 'reconnecting' | 'down'

/**
 * Kill a child process AND its descendants. The Go server spawns agent
 * subprocesses (claude/opencode/codex), so a bare child.kill() would orphan
 * them. On Unix we kill the whole process group (the server is spawned with
 * detached:true); on Windows we taskkill the tree.
 */
function killProcessTree(child: ChildProcess): void {
  try {
    if (process.platform === 'win32') {
      if (child.pid) {
        // /T = kill child processes, /F = force
        exec(`taskkill /pid ${child.pid} /T /F`)
      }
    } else if (child.pid) {
      // Negative pid = signal the whole process group.
      process.kill(-child.pid, 'SIGTERM')
    }
  } catch {
    // Fallback to a direct kill if group/tree kill fails.
    child.kill()
  }
}

export class EngineServer {
  private child: ChildProcess | null = null
  private baseUrl = ''
  // Random bearer token announced by the engine via FORKSYNC_TOKEN=. Empty
  // until the engine prints it; getToken() returns '' (requests then fail
  // auth) if read before the announcement arrives.
  private token = ''
  private startPromise: Promise<string> | null = null

  // Crash supervision state.
  // `quitting` distinguishes an intentional kill (app quit) from a crash so the
  // close handler doesn't try to respawn during shutdown.
  private quitting = false
  // Tracks whether the most recent start() announced an address (i.e. reached
  // a healthy state at least once) — used to decide if a close is a crash.
  private startedHealthy = false
  private restartAttempts = 0
  private readonly statusListeners = new Set<(status: EngineStatus) => void>()

  /**
   * Spawn seam. Production uses child_process.spawn; tests replace this to
   * drive a fake child's events (stdout announcement, 'close' crash) without
   * touching the real process graph. Overriding here avoids relying on
   * vi.mock of built-in CJS modules, which vitest does not intercept reliably.
   */
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  spawn: (binary: string, args: string[], opts: any) => any = (binary, args, opts) =>
    cpSpawn(binary, args, opts)

  /** Subscribe to lifecycle status changes. Returns an unsubscribe function. */
  onStatusChange(cb: (status: EngineStatus) => void): () => void {
    this.statusListeners.add(cb)
    return () => {
      this.statusListeners.delete(cb)
    }
  }

  private setStatus(status: EngineStatus): void {
    for (const cb of this.statusListeners) {
      try {
        cb(status)
      } catch (err) {
        log.error('[engine-server] status listener threw:', (err as Error).message)
      }
    }
  }

  /** Returns the HTTP base URL once the server is ready (e.g. http://127.0.0.1:54321). */
  async getBaseUrl(): Promise<string> {
    if (this.baseUrl) return this.baseUrl
    if (!this.startPromise) {
      this.setStatus('starting')
      this.startPromise = this.start()
      // If startup fails, clear the cached rejected promise so the next call
      // retries instead of re-awaiting a permanently-rejected promise.
      this.startPromise.catch(() => {
        this.startPromise = null
      })
    }
    this.baseUrl = await this.startPromise
    this.startedHealthy = true
    this.restartAttempts = 0
    this.setStatus('ready')
    return this.baseUrl
  }

  /** Returns a ws:// URL for the given path, derived from the running server. */
  async getWsUrl(path: string): Promise<string> {
    const http = await this.getBaseUrl()
    return http.replace(/^http/, 'ws') + path
  }

  /**
   * Returns the random bearer token the engine announced, or '' if the
   * announcement has not yet been read (callers should treat '' as "no auth
   * available" — requests will be rejected by the engine's auth middleware).
   */
  getToken(): string {
    return this.token
  }

  private resolveBinary(): { binary: string; args: string[]; cwd?: string } {
    if (app.isPackaged) {
      const ext = process.platform === 'win32' ? '.exe' : ''
      return { binary: join(process.resourcesPath, `forksync${ext}`), args: [] }
    }
    // Dev: `go run .` from the engine/ directory.
    const projectRoot = join(__dirname, '..', '..', '..')
    return {
      binary: 'go',
      args: ['run', '.'],
      cwd: join(projectRoot, 'engine')
    }
  }

  private start(): Promise<string> {
    return new Promise((resolve, reject) => {
      const { binary, args, cwd } = this.resolveBinary()
      const env = { ...process.env, ...(app.isPackaged ? {} : { FORKSYNC_LOG_LEVEL: 'debug' }) }

      log.info('[engine-server] spawning', binary, args.join(' '), cwd ?? '')
      const child = this.spawn(binary, args, {
        cwd,
        env,
        stdio: ['ignore', 'pipe', 'pipe'],
        detached: process.platform !== 'win32'
      }) as ChildProcess
      this.child = child

      const timeout = setTimeout(() => {
        killProcessTree(child)
        reject(new Error(`engine server did not announce address within ${STARTUP_TIMEOUT_MS}ms`))
      }, STARTUP_TIMEOUT_MS)

      let resolved = false

      const finish = (err: Error | null, url: string): void => {
        if (resolved) return
        resolved = true
        clearTimeout(timeout)
        if (err) reject(err)
        else resolve(url)
      }

      // Read stdout line-by-line for the FORKSYNC_HTTP_ADDR + FORKSYNC_TOKEN
      // announcements. The address line triggers the health wait + resolve;
      // the token line is stashed for getToken() to hand to EngineClient.
      if (child.stdout) {
        const rl = createInterface({ input: child.stdout })
        rl.on('line', (line) => {
          const addrIdx = line.indexOf(ADDR_PREFIX)
          if (addrIdx >= 0) {
            const addr = line.slice(addrIdx + ADDR_PREFIX.length).trim()
            const url = `http://${addr}`
            log.info('[engine-server] announced', url)
            this.waitForHealth(url).then(() => finish(null, url), (e) => finish(e, ''))
            return
          }
          const tokIdx = line.indexOf(TOKEN_PREFIX)
          if (tokIdx >= 0) {
            this.token = line.slice(tokIdx + TOKEN_PREFIX.length).trim()
            log.debug('[engine-server] captured auth token')
            return
          }
          log.debug('[engine-server] stdout:', line)
        })
      }
      if (child.stderr) {
        const rl = createInterface({ input: child.stderr })
        rl.on('line', (line) => {
          log.warn('[engine-server] stderr:', line)
        })
      }

      child.on('error', (err) => {
        log.error('[engine-server] spawn error:', err.message)
        finish(err, '')
      })
      child.on('close', (code) => {
        if (!resolved) {
          finish(new Error(`engine server exited before announcing address (code ${code})`), '')
          // A pre-announce exit during the very first start is a startup
          // failure, not a mid-session crash — let the caller's promise
          // rejection surface it. No supervision retry here.
          return
        }
        // The process exited AFTER a healthy start: this is a crash.
        this.handleCrash(code)
      })
    })
  }

  /**
   * Called when the child exits with a non-zero code after having been healthy.
   * Resets cached state and, unless the app is quitting, schedules a respawn
   * with exponential backoff. In-flight getBaseUrl() callers will await the new
   * startPromise created by scheduleRestart.
   */
  private handleCrash(code: number | null): void {
    log.warn('[engine-server] exited with code', code, 'after healthy start')
    this.child = null
    this.baseUrl = ''
    this.startPromise = null
    this.startedHealthy = false
    if (this.quitting) {
      log.info('[engine-server] app is quitting — not respawning')
      return
    }
    this.scheduleRestart()
  }

  /**
   * Respawn the engine with exponential backoff. Each attempt creates a fresh
   * startPromise so concurrent getBaseUrl() callers await the same respawn.
   * After MAX_RESTART_ATTEMPTS failures, the server is left down and listeners
   * are notified.
   */
  private scheduleRestart(): void {
    if (this.restartAttempts >= MAX_RESTART_ATTEMPTS) {
      log.error(
        `[engine-server] gave up after ${MAX_RESTART_ATTEMPTS} restart attempts; engine is down`
      )
      this.setStatus('down')
      return
    }
    const delay = RESTART_BACKOFF_MS[this.restartAttempts]
    this.restartAttempts++
    this.setStatus('reconnecting')
    log.info(
      `[engine-server] respawning in ${delay}ms (attempt ${this.restartAttempts}/${MAX_RESTART_ATTEMPTS})`
    )
    setTimeout(() => {
      // Re-check quitting in case the app exited during the backoff window.
      if (this.quitting) return
      this.startPromise = this.start()
      this.startPromise
        .then((url) => {
          this.baseUrl = url
          this.startedHealthy = true
          this.restartAttempts = 0
          this.setStatus('ready')
          log.info('[engine-server] reconnected at', url)
        })
        .catch((err) => {
          log.error('[engine-server] restart attempt failed:', (err as Error).message)
          // start() rejects on pre-announce exit; chain into the next backoff.
          this.startPromise = null
          this.scheduleRestart()
        })
    }, delay)
  }

  private async waitForHealth(baseUrl: string): Promise<void> {
    const deadline = Date.now() + STARTUP_TIMEOUT_MS
    while (Date.now() < deadline) {
      try {
        const res = await fetch(`${baseUrl}/healthz`, { signal: AbortSignal.timeout(2000) })
        if (res.ok) {
          log.info('[engine-server] healthy at', baseUrl)
          return
        }
      } catch {
        // not ready yet
      }
      await new Promise((r) => setTimeout(r, HEALTH_POLL_INTERVAL_MS))
    }
    throw new Error('engine server did not become healthy in time')
  }

  /** Terminates the server process. Safe to call from app quit handlers. */
  kill(): void {
    // Mark quitting so the crash supervisor doesn't try to respawn during the
    // shutdown sequence. Also suppresses any in-flight backoff timer.
    this.quitting = true
    if (this.child) {
      log.info('[engine-server] killing server process')
      killProcessTree(this.child)
      this.child = null
      this.baseUrl = ''
      this.token = ''
      this.startPromise = null
      this.startedHealthy = false
    }
  }
}

let server: EngineServer | null = null

export function getEngineServer(): EngineServer {
  if (!server) {
    server = new EngineServer()
  }
  return server
}
