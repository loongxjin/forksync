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
  AcceptData,
  RejectData,
  AgentListData,
  AgentSessionsData,
  AgentCleanupData,
  AgentResetData,
  HistoryData,
  EngineConfig,
  ConfigSetData
} from '../renderer/src/types/engine'

/** Default timeout for quick commands (status, config, history, etc.) */
const DEFAULT_TIMEOUT_MS = 30 * 1000

/** Timeout for long-running commands (sync, resolve with AI agents) */
const LONG_TIMEOUT_MS = 10 * 60 * 1000

/** Post-sync command — mirrors Go PostSyncCommand */
export interface PostSyncCommand {
  id: string
  name: string
  cmd: string
}

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
    return this.exec<StatusData>(args)
  }

  /** `forksync sync --all --json` */
  async syncAll(): Promise<ApiResponse<SyncData>> {
    return this.exec<SyncData>(['sync', '--all'], LONG_TIMEOUT_MS)
  }

  /** `forksync sync <name> --json` */
  async syncRepo(name: string): Promise<ApiResponse<SyncData>> {
    return this.exec<SyncData>(['sync', name], LONG_TIMEOUT_MS)
  }

  /** `forksync scan <dir> --json` */
  async scan(dir: string): Promise<ApiResponse<ScanData>> {
    return this.exec<ScanData>(['scan', dir])
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
    return this.exec<AddData>(args)
  }

  /** `forksync remove <name> --json` */
  async remove(name: string): Promise<ApiResponse<RemoveData>> {
    return this.exec<RemoveData>(['remove', name])
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
    return this.exec<ResolveData>(args, LONG_TIMEOUT_MS)
  }

  /** `forksync resolve <name> --accept --json` */
  async resolveAccept(name: string): Promise<ApiResponse<AcceptData>> {
    return this.exec<AcceptData>(['resolve', name, '--accept'], LONG_TIMEOUT_MS)
  }

  /** `forksync resolve <name> --reject --json` */
  async resolveReject(name: string): Promise<ApiResponse<RejectData>> {
    return this.exec<RejectData>(['resolve', name, '--reject'], LONG_TIMEOUT_MS)
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

  /** `forksync agent reset <name> --json` */
  async agentReset(name: string): Promise<ApiResponse<AgentResetData>> {
    return this.exec<AgentResetData>(['agent', 'reset', name])
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
    return this.exec<HistoryData>(args)
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
    return this.exec<{ message: string }>(args)
  }

  /** `forksync config get --json` */
  async configGet(): Promise<ApiResponse<EngineConfig>> {
    return this.exec<EngineConfig>(['config', 'get'])
  }

  /** `forksync config set <key> <value> --json` */
  async configSet(key: string, value: string): Promise<ApiResponse<ConfigSetData>> {
    return this.exec<ConfigSetData>(['config', 'set', key, value])
  }

  /** `forksync post-sync list <name> --json` */
  async postSyncList(repoName: string): Promise<ApiResponse<{ commands: PostSyncCommand[] }>> {
    return this.exec<{ commands: PostSyncCommand[] }>(['post-sync', 'list', repoName])
  }

  /** `forksync post-sync add <name> --name <name> --cmd <cmd> --json` */
  async postSyncAdd(repoName: string, cmdName: string, cmd: string): Promise<ApiResponse<{ commands: PostSyncCommand[] }>> {
    return this.exec<{ commands: PostSyncCommand[] }>(['post-sync', 'add', repoName, '--name', cmdName, '--cmd', cmd])
  }

  /** `forksync post-sync remove <name> --id <cmd-id> --json` */
  async postSyncRemove(repoName: string, cmdId: string): Promise<ApiResponse<{ commands: PostSyncCommand[] }>> {
    return this.exec<{ commands: PostSyncCommand[] }>(['post-sync', 'remove', repoName, '--id', cmdId])
  }

  /** `forksync summarize <repo-name> --json` */
  async summarize(repoName: string): Promise<ApiResponse<{ historyId: number; repoName: string; summary: string; summaryStatus: string }>> {
    return this.exec<{ historyId: number; repoName: string; summary: string; summaryStatus: string }>(['summarize', repoName], LONG_TIMEOUT_MS)
  }

  /** `forksync summarize <repo-name> --retry --json` */
  async summarizeRetry(repoName: string): Promise<ApiResponse<{ historyId: number; repoName: string; summary: string; summaryStatus: string }>> {
    return this.exec<{ historyId: number; repoName: string; summary: string; summaryStatus: string }>(['summarize', repoName, '--retry'], LONG_TIMEOUT_MS)
  }

  // -----------------------------------------------------------------------
  // Private — spawn + parse logic
  // -----------------------------------------------------------------------

  /**
   * Execute a CLI command and parse the JSON response.
   */
  private async exec<T>(args: string[], timeout: number = DEFAULT_TIMEOUT_MS): Promise<ApiResponse<T>> {
    return this.execRaw<T>(args, timeout)
  }

  /**
   * Low-level exec: spawn the Go binary, collect stdout, parse JSON.
   */
  private execRaw<T>(args: string[], timeout: number): Promise<ApiResponse<T>> {
    return new Promise((resolve, reject) => {
      const fullArgs = this.buildArgs(args)

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
        child.kill()
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
