/**
 * EngineClient — Go engine binary communication layer
 *
 * Spawns the ForkSync Go binary (or `go run` in dev mode) and parses
 * JSON responses from CLI commands. All methods return ApiResponse<T>
 * matching the Go engine's JSON contract.
 */

import { app } from 'electron'
import { join } from 'path'
import { spawn, ChildProcess, exec, execFile } from 'child_process'
import { createInterface } from 'readline'
import { existsSync, readdirSync, readFileSync, mkdirSync, statSync } from 'fs'
import { homedir } from 'os'
import { promisify } from 'util'
import log from './logger'

/**
 * Kill a child process and its entire process group.
 * Uses `detached: true` on non-Windows to create a new process group,
 * then signals the entire group with `process.kill(-pid)`.
 * On Windows, uses `taskkill /T /F /PID` for tree kill.
 */
function killProcessGroup(child: ChildProcess): void {
  try {
    if (process.platform === 'win32') {
      // Windows: use taskkill to kill the process tree
      exec(`taskkill /pid ${child.pid} /T /F`)
    } else if (child.pid) {
      // Unix: kill the entire process group
      process.kill(-child.pid, 'SIGTERM')
    }
  } catch {
    // Fallback to regular kill if process group kill fails
    child.kill()
  }
}

const execAsync = promisify(exec)
const execFileAsync = promisify(execFile)
import type {
  ApiResponse,
  StatusData,
  SyncData,
  ScanData,
  AddData,
  RemoveData,
  ResolveData,
  AcceptData,
  RejectData,
  AgentListData,
  AgentSessionsData,
  AgentCleanupData,
  AgentResetData,
  HistoryData,
  EngineConfig,
  ConfigSetData,
  AgentStreamEvent,
  PostSyncCommand
} from '@shared/types/engine'

/** Default timeout for quick commands (status, config, history, etc.) */
const DEFAULT_TIMEOUT_MS = 30 * 1000

/** Timeout for long-running commands (sync, resolve with AI agents) */
const LONG_TIMEOUT_MS = 10 * 60 * 1000

export class EngineClient {
  private binaryPath: string
  private projectRoot: string
  private engineDir: string

  constructor() {
    // Production: bundled binary in resources
    // Development: use `go run`
    if (app.isPackaged) {
      const ext = process.platform === 'win32' ? '.exe' : ''
      this.binaryPath = join(process.resourcesPath, `forksync${ext}`)
      this.projectRoot = ''
      this.engineDir = ''
    } else {
      this.binaryPath = 'go'
      // Resolve project root (where engine/ directory lives)
      // __dirname = app/out/main → up 3 levels = forksync/
      this.projectRoot = join(__dirname, '..', '..', '..')
      // Engine module lives in engine/ subdirectory
      this.engineDir = join(this.projectRoot, 'engine')
    }
  }

  // -----------------------------------------------------------------------
  // Public API — one method per CLI command
  // -----------------------------------------------------------------------

  /** `forksync status --json [--exclude repo1,repo2]` */
  async status(exclude?: string[]): Promise<ApiResponse<StatusData>> {
    const args = ['status']
    if (exclude && exclude.length > 0) {
      args.push('--exclude', exclude.join(','))
    }
    return this.execCommand<StatusData>(args)
  }

  /** `forksync sync --all --json` */
  async syncAll(): Promise<ApiResponse<SyncData>> {
    return this.execCommand<SyncData>(['sync', '--all'], LONG_TIMEOUT_MS)
  }

  /** `forksync sync <name> --json` */
  async syncRepo(name: string): Promise<ApiResponse<SyncData>> {
    return this.execCommand<SyncData>(['sync', name], LONG_TIMEOUT_MS)
  }

  /** `forksync scan <dir> --json` */
  async scan(dir: string): Promise<ApiResponse<ScanData>> {
    return this.execCommand<ScanData>(['scan', dir])
  }

  /** `forksync add <path> [--upstream <url>] [--branch-mapping <json>] --json` */
  async add(repoPath: string, upstream?: string, branchMapping?: { localBranch: string; remoteBranch: string }): Promise<ApiResponse<AddData>> {
    const args = ['add', repoPath]
    if (upstream) {
      args.push('--upstream', upstream)
    }
    if (branchMapping && branchMapping.localBranch && branchMapping.remoteBranch) {
      args.push('--branch-mapping', JSON.stringify(branchMapping))
    }
    return this.execCommand<AddData>(args)
  }

  /** `forksync remove <name> --json` */
  async remove(name: string): Promise<ApiResponse<RemoveData>> {
    return this.execCommand<RemoveData>(['remove', name])
  }

  /** `forksync resolve <name> [--agent <name>] [--no-confirm] --json` */
  async resolve(
    name: string,
    opts?: { agent?: string; noConfirm?: boolean; prepare?: boolean; retry?: boolean; manual?: boolean }
  ): Promise<ApiResponse<ResolveData>> {
    const args = ['resolve', name]
    if (opts?.prepare) args.push('--prepare')
    if (opts?.agent) args.push('--agent', opts.agent)
    if (opts?.noConfirm) args.push('--no-confirm')
    if (opts?.retry) args.push('--retry')
    if (opts?.manual) args.push('--manual')
    return this.execCommand<ResolveData>(args, LONG_TIMEOUT_MS)
  }

  /** `forksync resolve <name> --prepare --json` (lightweight workflow state update) */
  async resolvePrepare(name: string): Promise<ApiResponse<ResolveData>> {
    return this.resolve(name, { prepare: true })
  }

  /**
   * Spawn `forksync resolve <name> --stream` and emit NDJSON lines as events.
   * Returns a controller with onEvent/onDone/onError/kill callbacks.
   */
  resolveStream(
    name: string,
    opts?: { agent?: string; noConfirm?: boolean }
  ): {
    onEvent: (cb: (ev: AgentStreamEvent) => void) => void
    onDone: (cb: (result: ApiResponse<ResolveData>) => void) => void
    onError: (cb: (err: string) => void) => void
    kill: () => void
  } {
    const args = ['resolve', name, '--stream']
    if (opts?.agent) {
      args.push('--agent', opts.agent)
    }
    if (opts?.noConfirm) {
      args.push('--no-confirm')
    }

    const fullArgs = this.buildArgs(args)
    const child: ChildProcess = spawn(this.binaryPath, fullArgs, {
      cwd: app.isPackaged ? undefined : this.engineDir,
      env: { ...process.env, ...(app.isPackaged ? {} : { FORKSYNC_LOG_LEVEL: 'debug' }) },
      stdio: ['ignore', 'pipe', 'pipe'],
      detached: process.platform !== 'win32' // create new process group on Unix
    })
    log.info('[engine:resolveStream] spawned', name, 'pid:', child.pid)

    // Debug log for tracing stream issues (goes to electron-log file)
    const debugLog = (msg: string): void => {
      log.debug(`[engine:resolveStream] ${msg}`)
    }
    debugLog(`START resolveStream name=${name} pid=${child.pid} args=${JSON.stringify(fullArgs)}`)

    const eventCbs: Array<(ev: AgentStreamEvent) => void> = []
    const doneCbs: Array<(result: ApiResponse<ResolveData>) => void> = []
    const errorCbs: Array<(err: string) => void> = []

    let killed = false
    let notified = false // whether done/error was already notified

    const notifyEvent = (ev: AgentStreamEvent): void => {
      for (const cb of eventCbs) cb(ev)
    }
    const notifyDone = (result: ApiResponse<ResolveData>): void => {
      notified = true
      log.info('[engine:resolveStream] done', name, 'success:', result.success)
      for (const cb of doneCbs) cb(result)
    }
    const notifyError = (err: string): void => {
      notified = true
      log.error('[engine:resolveStream] error', name, err)
      for (const cb of errorCbs) cb(err)
    }

    // Ensure log dir exists
    try { mkdirSync(join(homedir(), '.forksync', 'logs'), { recursive: true }) } catch {}

    // Read stdout line-by-line
    if (child.stdout) {
      const rl = createInterface({ input: child.stdout })
      rl.on('line', (line) => {
        if (!line.trim()) return
        debugLog(`STDOUT line len=${line.length} preview=${line.substring(0, 200)}`)
        try {
          const parsed = JSON.parse(line)
          // Stream events have 't' field
          if (parsed.t != null) {
            // 'done' event is a stream completion signal — treat as final result
            if (parsed.t === 'done') {
              debugLog(`STDOUT done event success=${parsed.success}`)
              notifyDone({
                success: parsed.success ?? true,
                data: {
                  repoId: '',
                  conflicts: [],
                  agentResult: {
                    success: parsed.success ?? true,
                    summary: parsed.summary ?? '',
                    sessionId: parsed.session_id ?? '',
                    agentName: '',
                    resolvedFiles: [],
                    diff: ''
                  }
                },
                error: ''
              } as ApiResponse<ResolveData>)
            } else if (parsed.t === 'error') {
              debugLog(`STDOUT error event`)
              notifyError(parsed.d ?? 'Agent resolve error')
            } else {
              debugLog(`STDOUT event type=${parsed.t}`)
              notifyEvent(parsed as AgentStreamEvent)
            }
          } else if (parsed.success != null) {
            // Final ApiResponse (without 't' field)
            debugLog(`STDOUT ApiResponse success=${parsed.success}`)
            notifyDone(parsed as ApiResponse<ResolveData>)
          } else {
            // Unknown JSON — treat as raw stdout
            debugLog(`STDOUT unknown json`)
            notifyEvent({ t: 'stdout', d: line, ts: new Date().toISOString() })
          }
        } catch {
          // Not valid JSON — raw stdout
          debugLog(`STDOUT raw (not JSON) preview=${line.substring(0, 200)}`)
          notifyEvent({ t: 'stdout', d: line, ts: new Date().toISOString() })
        }
      })
    }

    // Read stderr line-by-line
    if (child.stderr) {
      const rl = createInterface({ input: child.stderr })
      rl.on('line', (line) => {
        if (!line.trim()) return
        debugLog(`STDERR preview=${line.substring(0, 200)}`)
        notifyEvent({ t: 'stderr', d: line, ts: new Date().toISOString() })
      })
    }

    child.on('error', (err) => {
      debugLog(`SPAWN ERROR: ${err.message}`)
      if (!killed) notifyError(`Failed to spawn engine: ${err.message}`)
    })

    child.on('close', (code) => {
      debugLog(`CLOSE code=${code} killed=${killed} notified=${notified}`)
      if (killed) return
      if (!notified) {
        // Safety net: process exited but notifyDone/notifyError was never called.
        // Can happen if the final JSON output couldn't be parsed.
        if (code !== 0) {
          notifyError(`Engine exited with code ${code}`)
        } else {
          debugLog(`CLOSE: code 0 but not notified, treating as done`)
          notifyDone({ success: true, data: null as unknown as ResolveData, error: '' })
        }
      }
    })

    return {
      onEvent: (cb) => { eventCbs.push(cb) },
      onDone: (cb) => { doneCbs.push(cb) },
      onError: (cb) => { errorCbs.push(cb) },
      kill: () => {
        killed = true
        killProcessGroup(child)
      }
    }
  }

  /**
   * Read the latest agent log file for a repo and return parsed events.
   */
  async readAgentLog(repoName: string): Promise<{
    events: AgentStreamEvent[]
    isRunning: boolean
  }> {
    log.debug('[engine:readAgentLog]', repoName)
    const configDir = join(homedir(), '.forksync')
    const logDir = join(configDir, 'agent-logs', repoName)

    if (!existsSync(logDir)) {
      log.debug('[engine:readAgentLog] logDir not found', logDir)
      return { events: [], isRunning: false }
    }

    const files = readdirSync(logDir)
      .filter((f) => f.endsWith('.ndjson'))
      .sort()
      .reverse()

    if (files.length === 0) {
      log.debug('[engine:readAgentLog] no log files')
      return { events: [], isRunning: false }
    }

    const latest = join(logDir, files[0])
    const stat = statSync(latest)
    log.debug('[engine:readAgentLog] reading', latest, 'size:', stat.size, 'mtime:', stat.mtime)
    const content = readFileSync(latest, 'utf-8')
    const lines = content.split('\n').filter(l => l.trim())
    const events: AgentStreamEvent[] = []
    const skippedLines: string[] = []

    for (const line of lines) {
      try {
        const parsed = JSON.parse(line)
        if (parsed.t != null) {
          events.push(parsed as AgentStreamEvent)
        } else {
          skippedLines.push(`no-t-field:${line.substring(0, 80)}`)
        }
      } catch (e) {
        skippedLines.push(`parse-error:${line.substring(0, 80)}`)
      }
    }

    // Event type distribution for debugging
    const typeDist: Record<string, number> = {}
    for (const ev of events) {
      typeDist[ev.t] = (typeDist[ev.t] || 0) + 1
    }

    // isRunning is true if the last event is not 'done' or 'error'
    const last = events[events.length - 1]
    const isRunning = last != null && last.t !== 'done' && last.t !== 'error'
    log.debug('[engine:readAgentLog] parsed', events.length, 'events (total lines:', lines.length, '), isRunning:', isRunning, 'types:', JSON.stringify(typeDist))
    if (skippedLines.length > 0) {
      log.warn('[engine:readAgentLog] skipped', skippedLines.length, 'lines:', skippedLines.slice(0, 5))
    }

    return { events, isRunning }
  }

  /**
   * Get git diff for a repo (working tree vs HEAD).
   * Reads repos.json to find the repo path, then runs `git diff HEAD`.
   */
  async repoDiff(repoName: string): Promise<{ success: boolean; diff?: string; error?: string }> {
    try {
      const configDir = join(homedir(), '.forksync')
      const reposPath = join(configDir, 'repos.json')
      if (!existsSync(reposPath)) {
        return { success: false, error: 'repos.json not found' }
      }
      const repos = JSON.parse(readFileSync(reposPath, 'utf-8')) as Array<{ name: string; path: string }>
      const repo = repos.find((r) => r.name === repoName)
      if (!repo) {
        return { success: false, error: `repo "${repoName}" not found` }
      }
      const { stdout, stderr } = await execFileAsync('git', ['-C', repo.path, 'diff', 'HEAD'])
      if (stderr) {
        log.warn('[engine:repoDiff] stderr:', stderr)
      }
      return { success: true, diff: stdout }
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err)
      log.error('[engine:repoDiff] error:', message)
      return { success: false, error: message }
    }
  }

  /** `forksync resolve <name> --accept --json` */
  async resolveAccept(name: string): Promise<ApiResponse<AcceptData>> {
    return this.execCommand<AcceptData>(['resolve', name, '--accept'], LONG_TIMEOUT_MS)
  }

  /** `forksync resolve <name> --reject --json` */
  async resolveReject(name: string): Promise<ApiResponse<RejectData>> {
    return this.execCommand<RejectData>(['resolve', name, '--reject'], LONG_TIMEOUT_MS)
  }

  /** `forksync agent list --json` */
  async agentList(): Promise<ApiResponse<AgentListData>> {
    return this.execCommand<AgentListData>(['agent', 'list'])
  }

  /** `forksync agent sessions --json` */
  async agentSessions(): Promise<ApiResponse<AgentSessionsData>> {
    return this.execCommand<AgentSessionsData>(['agent', 'sessions'])
  }

  /** `forksync agent cleanup --json` */
  async agentCleanup(): Promise<ApiResponse<AgentCleanupData>> {
    return this.execCommand<AgentCleanupData>(['agent', 'cleanup'])
  }

  /** `forksync agent reset <name> --json` */
  async agentReset(name: string): Promise<ApiResponse<AgentResetData>> {
    return this.execCommand<AgentResetData>(['agent', 'reset', name])
  }

  /** `forksync history [--limit N] [repo-name] --json` */
  async history(repoName?: string, limit?: number): Promise<ApiResponse<HistoryData>> {
    const args = ['history']
    if (repoName) {
      args.push(repoName)
    }
    if (limit) {
      args.push('--limit', String(limit))
    }
    return this.execCommand<HistoryData>(args)
  }

  /** `forksync history --cleanup [--keep-days N] [repo-name] --json` */
  async historyCleanup(opts?: { repoName?: string; keepDays?: number }): Promise<ApiResponse<{ message: string }>> {
    const args = ['history', '--cleanup']
    if (opts?.keepDays && opts.keepDays > 0) {
      args.push('--keep-days', String(opts.keepDays))
    }
    if (opts?.repoName) {
      args.push(opts.repoName)
    }
    return this.execCommand<{ message: string }>(args)
  }

  /** `forksync config get --json` */
  async configGet(): Promise<ApiResponse<EngineConfig>> {
    return this.execCommand<EngineConfig>(['config', 'get'])
  }

  /** `forksync config set <key> <value> --json` */
  async configSet(key: string, value: string): Promise<ApiResponse<ConfigSetData>> {
    return this.execCommand<ConfigSetData>(['config', 'set', key, value])
  }

  /** `forksync post-sync list <name> --json` */
  async postSyncList(repoName: string): Promise<ApiResponse<{ commands: PostSyncCommand[] }>> {
    return this.execCommand<{ commands: PostSyncCommand[] }>(['post-sync', 'list', repoName])
  }

  /** `forksync post-sync add <name> --name <name> --cmd <cmd> --json` */
  async postSyncAdd(repoName: string, cmdName: string, cmd: string): Promise<ApiResponse<{ commands: PostSyncCommand[] }>> {
    return this.execCommand<{ commands: PostSyncCommand[] }>(['post-sync', 'add', repoName, '--name', cmdName, '--cmd', cmd])
  }

  /** `forksync post-sync remove <name> --id <cmd-id> --json` */
  async postSyncRemove(repoName: string, cmdId: string): Promise<ApiResponse<{ commands: PostSyncCommand[] }>> {
    return this.execCommand<{ commands: PostSyncCommand[] }>(['post-sync', 'remove', repoName, '--id', cmdId])
  }

  /** `forksync summarize <repo-name> --json` */
  async summarize(repoName: string): Promise<ApiResponse<{ historyId: number; repoName: string; summary: string; summaryStatus: string }>> {
    return this.execCommand<{ historyId: number; repoName: string; summary: string; summaryStatus: string }>(['summarize', repoName], LONG_TIMEOUT_MS)
  }

  /** `forksync summarize <repo-name> --retry --json` */
  async summarizeRetry(repoName: string): Promise<ApiResponse<{ historyId: number; repoName: string; summary: string; summaryStatus: string }>> {
    return this.execCommand<{ historyId: number; repoName: string; summary: string; summaryStatus: string }>(['summarize', repoName, '--retry'], LONG_TIMEOUT_MS)
  }


  // -----------------------------------------------------------------------
  // Private — unified command execution
  // -----------------------------------------------------------------------

  /**
   * Execute a CLI command and parse the JSON response.
   * All command methods delegate to this single entry point, centralizing
   * spawn logic, timeout handling, and error parsing.
   */
  private execCommand<T>(args: string[], timeout: number = DEFAULT_TIMEOUT_MS): Promise<ApiResponse<T>> {
    return new Promise((resolve, reject) => {
      const fullArgs = this.buildArgs(args)

      const child: ChildProcess = spawn(this.binaryPath, fullArgs, {
        cwd: app.isPackaged ? undefined : this.engineDir,
        env: { ...process.env, ...(app.isPackaged ? {} : { FORKSYNC_LOG_LEVEL: 'debug' }) },
        stdio: ['ignore', 'pipe', 'pipe'],
        detached: process.platform !== 'win32'
      })

      let stdout = ''
      let stderr = ''

      child.stdout?.on('data', (chunk: Buffer) => {
        stdout += chunk.toString()
      })

      child.stderr?.on('data', (chunk: Buffer) => {
        stderr += chunk.toString()
      })

      // Timeout handler
      const timer = setTimeout(() => {
        killProcessGroup(child)
        reject(new EngineTimeoutError(`Engine command timed out after ${timeout}ms`))
      }, timeout)

      child.on('close', (code) => {
        clearTimeout(timer)

        if (code !== 0 && !stdout) {
          // Non-zero exit with no stdout — process-level error
          reject(
            new EngineProcessError(
              `Engine exited with code ${code}`,
              code,
              stderr
            )
          )
          return
        }

        // Try to parse JSON from stdout
        try {
          const parsed = JSON.parse(stdout.trim()) as ApiResponse<T>
          resolve(parsed)
        } catch (err) {
          reject(
            new EngineParseError(
              `Failed to parse engine output: ${(err as Error).message}`,
              stdout,
              stderr
            )
          )
        }
      })

      child.on('error', (err) => {
        clearTimeout(timer)
        reject(new EngineSpawnError(`Failed to spawn engine: ${err.message}`))
      })
    })
  }

  /**
   * Build full CLI arguments — adds `--json` flag and `go run` prefix in dev.
   */
  private buildArgs(engineArgs: string[]): string[] {
    if (app.isPackaged) {
      return [...engineArgs, '--json']
    }
    return ['run', '.', ...engineArgs, '--json']
  }
}

// ---------------------------------------------------------------------------
// Custom Error Types
// ---------------------------------------------------------------------------

export class EngineTimeoutError extends Error {
  constructor(message: string) {
    super(message)
    this.name = 'EngineTimeoutError'
  }
}

export class EngineProcessError extends Error {
  readonly exitCode: number | null
  readonly stderr: string

  constructor(message: string, exitCode: number | null, stderr: string) {
    super(message)
    this.name = 'EngineProcessError'
    this.exitCode = exitCode
    this.stderr = stderr
  }
}

export class EngineParseError extends Error {
  readonly stdout: string
  readonly stderr: string

  constructor(message: string, stdout: string, stderr: string) {
    super(message)
    this.name = 'EngineParseError'
    this.stdout = stdout
    this.stderr = stderr
  }
}

export class EngineSpawnError extends Error {
  constructor(message: string) {
    super(message)
    this.name = 'EngineSpawnError'
  }
}
