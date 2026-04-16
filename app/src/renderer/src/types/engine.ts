/**
 * ForkSync Engine — TypeScript Type Definitions
 *
 * 1:1 mapping of Go engine JSON contract (engine/pkg/types/types.go).
 * All types correspond to Go structs used in CLI command JSON output.
 */

// ---------------------------------------------------------------------------
// Generic Response Wrapper
// ---------------------------------------------------------------------------

/** Go ApiResponse[T] — all CLI commands (except `serve`) wrap output in this */
export interface ApiResponse<T> {
  success: boolean
  data: T
  error: string
}

// ---------------------------------------------------------------------------
// Enums & Shared Types
// ---------------------------------------------------------------------------

/** Repo status enum — 8 values matching Go RepoStatus constants */
export type RepoStatus =
  | 'synced'
  | 'syncing'
  | 'conflict'
  | 'resolving'
  | 'resolved'
  | 'error'
  | 'unconfigured'
  | 'up_to_date'

/** Agent session status */
export type AgentSessionStatus = 'active' | 'expired' | 'failed'

// ---------------------------------------------------------------------------
// Core Domain Models
// ---------------------------------------------------------------------------

/** Branch mapping configuration */
export interface BranchMapping {
  localBranch: string
  remoteBranch: string
}

/** Post-sync command — mirrors Go PostSyncCommand */
export interface PostSyncCommand {
  id: string
  name: string
  cmd: string
}

/** Post-sync command execution result */
export interface PostSyncResult {
  name: string
  cmd: string
  success: boolean
  output?: string
  error?: string
}

/** Go Repo — managed repository */
export interface Repo {
  id: string
  name: string
  path: string
  origin: string
  upstream: string
  branch: string
  branchMapping?: BranchMapping
  autoSync: boolean
  syncInterval: string
  conflictStrategy: string
  postSyncCommands?: PostSyncCommand[]
  createdAt: string
  lastSync: string | null
  status: RepoStatus
  aheadBy: number
  behindBy: number
  errorMessage?: string
}

/** Go ScannedRepo — scan discovery result */
export interface ScannedRepo {
  path: string
  name: string
  origin: string
  isFork: boolean
  suggestedUpstream?: string
  localBranches?: string[]
  remoteBranches?: string[]
}

/** Go SyncResult — per-repo sync outcome */
export interface SyncResult {
  repoId: string
  repoName: string
  status: RepoStatus
  commitsPulled: number
  conflictFiles?: string[]
  errorMessage?: string
  agentUsed?: string
  conflictsFound?: number
  autoResolved?: number
  pendingConfirm?: string[]
  postSyncResults?: PostSyncResult[]
}

/** Go ConflictFile — simplified conflict info (agent reads file contents) */
export interface ConflictFile {
  path: string
}

// ---------------------------------------------------------------------------
// Agent Types
// ---------------------------------------------------------------------------

/** Go AgentInfo — installed CLI agent information */
export interface AgentInfo {
  name: string
  binary: string
  path: string
  installed: boolean
  version?: string
}

/** Go AgentSessionInfo — agent session metadata */
export interface AgentSessionInfo {
  id: string
  repoId: string
  agentName: string
  status: string
  createdAt: string
  lastUsedAt: string
}

/** Go AgentResolveResult — agent conflict resolution output */
export interface AgentResolveResult {
  success: boolean
  resolvedFiles: string[]
  diff: string
  summary: string
  sessionId: string
  agentName: string
}

/** Go AgentResolveRequest — resolve command options */
export interface AgentResolveRequest {
  repoId: string
  files: string[]
  strategy: string
  autoConfirm: boolean
}

// ---------------------------------------------------------------------------
// Command Response Data Types
// ---------------------------------------------------------------------------

/** `forksync status` → ApiResponse<StatusData> */
export interface StatusData {
  repos: Repo[]
  agents: AgentInfo[]
  preferredAgent: string
}

/** `forksync scan <dir>` → ApiResponse<ScanData> */
export interface ScanData {
  repos: ScannedRepo[]
}

/** `forksync sync` → ApiResponse<SyncData> */
export interface SyncData {
  results: SyncResult[]
}

/** `forksync add <path>` → ApiResponse<AddData> */
export interface AddData {
  repo: Repo
}

/** `forksync resolve <name>` → ApiResponse<ResolveData> */
export interface ResolveData {
  repoId: string
  conflicts: ConflictFile[]
  agentResult?: AgentResolveResult
}

/** `forksync resolve <name> --accept/--no-confirm` → ApiResponse<AcceptData> */
export interface AcceptData {
  repoId: string
  resolved: boolean
  remainingConflicts?: string[]
}

/** `forksync resolve <name> --reject` → ApiResponse<RejectData> */
export interface RejectData {
  repoId: string
  rolledBack: boolean
}

/** `forksync agent list` → ApiResponse<AgentListData> */
export interface AgentListData {
  agents: AgentInfo[]
  preferred: string
}

/** `forksync agent sessions` → ApiResponse<AgentSessionsData> */
export interface AgentSessionsData {
  sessions: AgentSessionInfo[]
}

/** `forksync history` → ApiResponse<HistoryData> */
export interface HistoryData {
  records: SyncHistoryRecord[]
}

/** Post-sync commands response data */
export interface PostSyncCommandsData {
  commands: PostSyncCommand[]
}

/** Sync history record from SQLite */
export interface SyncHistoryRecord {
  id: number
  repoId: string
  repoName: string
  status: string
  commitsPulled: number
  conflictFiles: string[]
  agentUsed: string
  conflictsFound: number
  autoResolved: number
  errorMessage: string
  summary: string
  summaryStatus: string
  createdAt: string
}

// ---------------------------------------------------------------------------
// Special / Non-standard Responses
// ---------------------------------------------------------------------------

/**
 * `forksync agent cleanup` — Go uses map[string]interface{} instead of typed struct.
 * Output shape: ApiResponse<{ removed: number }>
 */
export interface AgentCleanupData {
  removed: number
}

/**
 * `forksync remove <name>` — returns the removed repo name.
 */export interface RemoveData {
  removed: string
}

/**
 * `forksync serve` — bypasses ApiResponse wrapper entirely.
 * Outputs bare JSON (no success/data/error wrapper).
 */
export interface ServeStatus {
  running: boolean
  interval: string
  message: string
}

// ---------------------------------------------------------------------------
// App-level Types (not from Go engine)
// ---------------------------------------------------------------------------

/** Go engine Config — from `forksync config get --json` */
export interface EngineConfig {
  Sync: {
    DefaultInterval: string
    SyncOnStartup: boolean
    AutoLaunch: boolean
    AutoSummary: boolean
    SummaryAgent: string
    SummaryLanguage: string
  }
  Agent: {
    Preferred: string
    Priority: string[]
    Timeout: string
    ConflictStrategy: string
    ConfirmBeforeCommit: boolean
    SessionTTL: string
  }
  GitHub: {
    Token: string
  }
  Notification: {
    Enabled: boolean
  }
  Proxy: {
    Enabled: boolean
    URL: string
  }
}

/** `forksync config set` response */
export interface ConfigSetData {
  key: string
  value: unknown
}

/** IPC channel names for Electron main↔renderer communication */
export type EngineChannel =
  | 'engine:status'
  | 'engine:syncAll'
  | 'engine:syncRepo'
  | 'engine:scan'
  | 'engine:add'
  | 'engine:remove'
  | 'engine:resolve'
  | 'engine:resolveAccept'
  | 'engine:resolveReject'
  | 'engine:agentList'
  | 'engine:agentSessions'
  | 'engine:agentCleanup'
  | 'engine:history'

/** EngineClient method parameter types */
export interface ResolveOptions {
  agent?: string
  noConfirm?: boolean
}

export interface ScanOptions {
  dir: string
}

export interface AddOptions {
  path: string
  upstream?: string
  branchMapping?: BranchMapping
}
