/**
 * ForkSync Engine — TypeScript Type Definitions
 *
 * 1:1 mapping of Go engine JSON contract (engine/pkg/types/types.go).
 * All types mirror Go structs (pkg/types) served by the embedded HTTP server.
 */

// ---------------------------------------------------------------------------
// Generic Response Wrapper
// ---------------------------------------------------------------------------

/** ApiResponse[T] — most endpoints wrap their payload in this envelope */
export interface ApiResponse<T> {
  success: boolean
  data: T
  error: string
}

// ---------------------------------------------------------------------------
// Enums & Shared Types
// ---------------------------------------------------------------------------

/** Repo status enum — 9 values matching Go RepoStatus constants */
export type RepoStatus =
  | 'up_to_date'
  | 'sync_needed'
  | 'syncing'
  | 'conflict'
  | 'resolving'
  | 'resolved'
  | 'waiting'
  | 'error'
  | 'unconfigured'

/** Agent session status */
export type AgentSessionStatus = 'active' | 'failed'

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
  postSyncCommands?: PostSyncCommand[]
  workflow?: SyncWorkflow
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
  agentResult?: AgentResolveResult
  postSyncResults?: PostSyncResult[]
  commitError?: string
  workflow?: SyncWorkflow
}

// ---------------------------------------------------------------------------
// Workflow Types
// ---------------------------------------------------------------------------

export type WorkflowStep =
  | 'fetch'
  | 'merge'
  | 'check_conflicts'
  | 'resolve_strategy'
  | 'agent_resolve'
  | 'accept_changes'
  | 'commit'

export type WorkflowStepStatus =
  | 'pending'
  | 'running'
  | 'success'
  | 'failed'
  | 'skipped'
  | 'waiting'

export interface WorkflowStepRecord {
  step: WorkflowStep
  status: WorkflowStepStatus
  startedAt?: string
  endedAt?: string
  message?: string
  error?: string
  /** Identifies the agent-resolve log session for this step (agent_resolve only). */
  resolveSessionId?: string
}

export type WorkflowRunStatus = 'running' | 'waiting' | 'success' | 'failed'

export interface SyncWorkflow {
  runId: string
  steps: WorkflowStepRecord[]
  status: WorkflowRunStatus
  startedAt: string
  finishedAt?: string
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
  repoName: string
  agentName: string
  status: AgentSessionStatus
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

/** Agent stream event emitted during conflict resolution */
export interface AgentStreamEvent {
  t: 'start' | 'stdout' | 'stderr' | 'tool' | 'done' | 'error' | 'state_persisted'
  d?: string
  agent?: string
  files?: string[]
  ts: string
  success?: boolean
  summary?: string
  session_id?: string
  resolvedFiles?: string[]
  diff?: string
  agentName?: string
  name?: string
  path?: string
}

// ---------------------------------------------------------------------------
// Response Data Types (mirrors Go pkg/types, served by the HTTP server)
// ---------------------------------------------------------------------------

/** `GET /status` → ApiResponse<StatusData> */
export interface StatusData {
  repos: Repo[]
  agents: AgentInfo[]
  preferredAgent: string
}

/** `POST /scan` → ApiResponse<ScanData> */
export interface ScanData {
  repos: ScannedRepo[]
}

/** `POST /sync/all` / `POST /sync/repos/{name}` → ApiResponse<SyncData> */
export interface SyncData {
  results: SyncResult[]
}

/** `POST /repos` → ApiResponse<AddData> */
export interface AddData {
  repo: Repo
}

/** `POST /repos/{name}/resolve` (agent mode) → ApiResponse<ResolveData> */
export interface ResolveData {
  repoId: string
  conflicts: ConflictFile[]
  agentResult?: AgentResolveResult
  commitError?: string
}

/** `POST /repos/{name}/resolve` (accept mode) → ApiResponse<AcceptData> */
export interface AcceptData {
  repoId: string
  resolved: boolean
  remainingConflicts?: string[]
}

/** `POST /repos/{name}/resolve` (reject mode) → ApiResponse<RejectData> */
export interface RejectData {
  repoId: string
  rolledBack: boolean
}

/** `GET /agents` → ApiResponse<AgentListData> */
export interface AgentListData {
  agents: AgentInfo[]
  preferred: string
}

/** `GET /agents/sessions` → ApiResponse<AgentSessionsData> */
export interface AgentSessionsData {
  sessions: AgentSessionInfo[]
}

/** `POST /agents/{name}/reset` → ApiResponse<AgentResetData> */
export interface AgentResetData {
  repoId: string
  cleared: boolean
}

/** `GET /history` → ApiResponse<HistoryData> */
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
  status: RepoStatus
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
 * `POST /agents/cleanup` — returns {removed: number}.
 * Output shape: ApiResponse<{ removed: number }>
 */
export interface AgentCleanupData {
  removed: number
}

/**
 * `DELETE /repos/{name}` — returns the removed repo name.
 */
export interface RemoveData {
  removed: string
}

// ---------------------------------------------------------------------------
// App-level Types (not from Go engine)
// ---------------------------------------------------------------------------

/** `GET /config` → ApiResponse<EngineConfig> */
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

/** `PUT /config` → ApiResponse<ConfigSetData> */
export interface ConfigSetData {
  key: string
  value: unknown
}
