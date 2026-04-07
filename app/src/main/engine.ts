/**
 * EngineClient — Go engine binary communication layer
 *
 * Spawns the ForkSync Go binary (or `go run` in dev mode) and parses
 * JSON responses from CLI commands. All methods return ApiResponse<T>
 * matching the Go engine's JSON contract.
 */

import { app } from 'electron'
import { join } from 'path'
import { spawn, ChildProcess } from 'child_process'
import type {
  ApiResponse,
  StatusData,
  SyncData,
  ScanData,
  AddData,
  RemoveData,
  ResolveData,
  DoneData,
  RejectData,
  AgentListData,
  AgentSessionsData,
  AgentCleanupData
} from '../renderer/src/types/engine'

/** Default timeout for engine commands (10 minutes — agent resolve can be slow) */
const DEFAULT_TIMEOUT_MS = 10 * 60 * 1000

export class EngineClient {
  private binaryPath: string
  private projectRoot: string
  private engineDir: string

  constructor() {
    // Production: bundled binary in resources
    // Development: use `go run`
    if (app.isPackaged) {
      this.binaryPath = join(process.resourcesPath, 'forksync')
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

  /** `forksync status --json` */
  async status(): Promise<ApiResponse<StatusData>> {
    return this.exec<StatusData>(['status'])
  }

  /** `forksync sync --all --json` */
  async syncAll(): Promise<ApiResponse<SyncData>> {
    return this.exec<SyncData>(['sync', '--all'])
  }

  /** `forksync sync <name> --json` */
  async syncRepo(name: string): Promise<ApiResponse<SyncData>> {
    return this.exec<SyncData>(['sync', name])
  }

  /** `forksync scan <dir> --json` */
  async scan(dir: string): Promise<ApiResponse<ScanData>> {
    return this.exec<ScanData>(['scan', dir])
  }

  /** `forksync add <path> [--upstream <url>] --json` */
  async add(repoPath: string, upstream?: string): Promise<ApiResponse<AddData>> {
    const args = ['add', repoPath]
    if (upstream) {
      args.push('--upstream', upstream)
    }
    return this.exec<AddData>(args)
  }

  /**
   * `forksync remove <name> --json`
   *
   * Note: Go engine has a double-wrapping bug — outputJSON wraps an already-
   * wrapped ApiResponse. We unwrap the outer layer here.
   */
  async remove(name: string): Promise<ApiResponse<RemoveData>> {
    const raw = await this.execRaw(['remove', name])
    // Handle double-wrapping: outer.success is the real indicator
    if (raw.success && typeof raw.data === 'object' && raw.data !== null) {
      const inner = raw.data as Record<string, unknown>
      // Check if inner is also an ApiResponse (has success field)
      if ('success' in inner && 'data' in inner) {
        return inner as unknown as ApiResponse<RemoveData>
      }
    }
    return raw as ApiResponse<RemoveData>
  }

  /** `forksync resolve <name> [--agent <name>] [--no-confirm] --json` */
  async resolve(
    name: string,
    opts?: { agent?: string; noConfirm?: boolean }
  ): Promise<ApiResponse<ResolveData>> {
    const args = ['resolve', name]
    if (opts?.agent) {
      args.push('--agent', opts.agent)
    }
    if (opts?.noConfirm) {
      args.push('--no-confirm')
    }
    return this.exec<ResolveData>(args)
  }

  /** `forksync resolve <name> --done --json` */
  async resolveDone(name: string): Promise<ApiResponse<DoneData>> {
    return this.exec<DoneData>(['resolve', name, '--done'])
  }

  /** `forksync resolve <name> --reject --json` */
  async resolveReject(name: string): Promise<ApiResponse<RejectData>> {
    return this.exec<RejectData>(['resolve', name, '--reject'])
  }

  /** `forksync agent list --json` */
  async agentList(): Promise<ApiResponse<AgentListData>> {
    return this.exec<AgentListData>(['agent', 'list'])
  }

  /** `forksync agent sessions --json` */
  async agentSessions(): Promise<ApiResponse<AgentSessionsData>> {
    return this.exec<AgentSessionsData>(['agent', 'sessions'])
  }

  /** `forksync agent cleanup --json` */
  async agentCleanup(): Promise<ApiResponse<AgentCleanupData>> {
    return this.exec<AgentCleanupData>(['agent', 'cleanup'])
  }

  // -----------------------------------------------------------------------
  // Private — spawn + parse logic
  // -----------------------------------------------------------------------

  /**
   * Execute a CLI command and parse the JSON response.
   */
  private async exec<T>(args: string[]): Promise<ApiResponse<T>> {
    return this.execRaw<T>(args)
  }

  /**
   * Low-level exec: spawn the Go binary, collect stdout, parse JSON.
   */
  private execRaw<T>(args: string[]): Promise<ApiResponse<T>> {
    return new Promise((resolve, reject) => {
      const fullArgs = this.buildArgs(args)
      const timeout = DEFAULT_TIMEOUT_MS

      const child: ChildProcess = spawn(this.binaryPath, fullArgs, {
        cwd: app.isPackaged ? undefined : this.engineDir,
        env: { ...process.env },
        stdio: ['ignore', 'pipe', 'pipe']
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
        child.kill('SIGTERM')
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
