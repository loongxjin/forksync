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
 */

import { app } from 'electron'
import { join } from 'path'
import { spawn, ChildProcess } from 'child_process'
import { createInterface } from 'readline'
import log from './logger'

const ADDR_PREFIX = 'FORKSYNC_HTTP_ADDR='
const STARTUP_TIMEOUT_MS = 30_000
const HEALTH_POLL_INTERVAL_MS = 100

function killProcessGroup(child: ChildProcess): void {
  try {
    if (process.platform === 'win32') {
      child.kill() // Windows: no process groups; rely on tree-kill via taskkill if needed
    } else if (child.pid) {
      process.kill(-child.pid, 'SIGTERM')
    }
  } catch {
    child.kill()
  }
}

export class EngineServer {
  private child: ChildProcess | null = null
  private baseUrl = ''
  private startPromise: Promise<string> | null = null

  /** Returns the HTTP base URL once the server is ready (e.g. http://127.0.0.1:54321). */
  async getBaseUrl(): Promise<string> {
    if (this.baseUrl) return this.baseUrl
    if (!this.startPromise) {
      this.startPromise = this.start()
    }
    this.baseUrl = await this.startPromise
    return this.baseUrl
  }

  /** Returns a ws:// URL for the given path, derived from the running server. */
  async getWsUrl(path: string): Promise<string> {
    const http = await this.getBaseUrl()
    return http.replace(/^http/, 'ws') + path
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
      const child = spawn(binary, args, {
        cwd,
        env,
        stdio: ['ignore', 'pipe', 'pipe'],
        detached: process.platform !== 'win32'
      })
      this.child = child

      const timeout = setTimeout(() => {
        killProcessGroup(child)
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

      // Read stdout line-by-line for the FORKSYNC_HTTP_ADDR announcement.
      if (child.stdout) {
        const rl = createInterface({ input: child.stdout })
        rl.on('line', (line) => {
          const idx = line.indexOf(ADDR_PREFIX)
          if (idx >= 0) {
            const addr = line.slice(idx + ADDR_PREFIX.length).trim()
            const url = `http://${addr}`
            log.info('[engine-server] announced', url)
            this.waitForHealth(url).then(() => finish(null, url), (e) => finish(e, ''))
          } else {
            log.debug('[engine-server] stdout:', line)
          }
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
        } else if (code !== 0) {
          log.warn('[engine-server] exited with code', code)
        }
      })
    })
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
    if (this.child) {
      log.info('[engine-server] killing server process')
      killProcessGroup(this.child)
      this.child = null
      this.baseUrl = ''
      this.startPromise = null
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
