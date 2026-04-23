package types

import (
	"fmt"
	"strings"
	"time"
)

// ApiResponse 通用 JSON 响应格式
type ApiResponse[T any] struct {
	Success bool   `json:"success"`
	Data    T      `json:"data"`
	Error   string `json:"error,omitempty"`
}

// RepoStatus 仓库状态枚举
type RepoStatus string

const (
	RepoStatusUpToDate     RepoStatus = "up_to_date"
	RepoStatusSyncNeeded   RepoStatus = "sync_needed" // 上游有更新，需要同步
	RepoStatusSyncing      RepoStatus = "syncing"
	RepoStatusConflict     RepoStatus = "conflict"
	RepoStatusResolving    RepoStatus = "resolving" // agent 正在解决冲突
	RepoStatusResolved     RepoStatus = "resolved"  // agent 解决完成，等待用户确认
	RepoStatusError        RepoStatus = "error"
	RepoStatusUnconfigured RepoStatus = "unconfigured"
)

// SessionStatus Agent 会话状态枚举
type SessionStatus string

const (
	SessionStatusActive  SessionStatus = "active"
	SessionStatusExpired SessionStatus = "expired"
	SessionStatusFailed  SessionStatus = "failed"
)

// SummaryStatus 同步摘要生成状态枚举
type SummaryStatus string

const (
	SummaryStatusPending    SummaryStatus = "pending"
	SummaryStatusGenerating SummaryStatus = "generating"
	SummaryStatusDone       SummaryStatus = "done"
	SummaryStatusFailed     SummaryStatus = "failed"
)

// ConflictStrategy constants
const (
	StrategyAgentResolve = "agent_resolve"
)

// ResolveStrategy constants
const (
	ResolveStrategyPreserveOurs   = "preserve_ours"
	ResolveStrategyPreserveTheirs = "preserve_theirs"
	ResolveStrategyBalanced       = "balanced"
)

// Time 可序列化的 time.Time
type Time struct {
	time.Time
}

func (t Time) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf("\"%s\"", t.Format(time.RFC3339))), nil
}

func (t *Time) UnmarshalJSON(data []byte) error {
	str := strings.Trim(string(data), `"`)
	if str == "" || str == "null" {
		return nil
	}
	parsed, err := time.Parse(time.RFC3339, str)
	if err != nil {
		return err
	}
	t.Time = parsed
	return nil
}

// BranchMapping defines a mapping between local and remote branch names
type BranchMapping struct {
	LocalBranch  string `json:"localBranch"`
	RemoteBranch string `json:"remoteBranch"`
}

// PostSyncCommand represents a shell command to run after successful sync.
type PostSyncCommand struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Cmd  string `json:"cmd"`
}

// Repo represents a managed repository
type Repo struct {
	ID               string            `json:"id"`
	Name             string            `json:"name"`
	Path             string            `json:"path"`
	Origin           string            `json:"origin"`
	Upstream         string            `json:"upstream"`
	Branch           string            `json:"branch"`
	BranchMapping    *BranchMapping    `json:"branchMapping,omitempty"`
	AutoSync         bool              `json:"autoSync"`
	SyncInterval     string            `json:"syncInterval"`
	ConflictStrategy string            `json:"conflictStrategy"`
	PostSyncCommands []PostSyncCommand `json:"postSyncCommands,omitempty"`
	CreatedAt        time.Time         `json:"createdAt"`
	LastSync         *Time             `json:"lastSync"`
	Status           RepoStatus        `json:"status"`
	AheadBy          int               `json:"aheadBy"`
	BehindBy         int               `json:"behindBy"`
	ErrorMessage     string            `json:"errorMessage,omitempty"`
}

// GetRemoteBranchForLocal returns the remote branch name for a given local branch.
// If a custom mapping exists and matches, it returns the mapped remote branch.
// Otherwise, it returns the local branch name (same-name mapping).
func (r *Repo) GetRemoteBranchForLocal(localBranch string) string {
	if r.BranchMapping != nil && r.BranchMapping.LocalBranch == localBranch {
		return r.BranchMapping.RemoteBranch
	}
	return localBranch
}

// RemoteName returns the name of the upstream remote ("upstream" if configured, "origin" otherwise).
func (r *Repo) RemoteName() string {
	if r.Upstream == "" {
		return "origin"
	}
	return "upstream"
}

// ScannedRepo 扫描结果
type ScannedRepo struct {
	Path              string   `json:"path"`
	Name              string   `json:"name"`
	Origin            string   `json:"origin"`
	IsFork            bool     `json:"isFork"`
	SuggestedUpstream string   `json:"suggestedUpstream,omitempty"`
	LocalBranches     []string `json:"localBranches,omitempty"`
	RemoteBranches    []string `json:"remoteBranches,omitempty"`
}

// SyncResult 同步结果
type SyncResult struct {
	RepoID          string              `json:"repoId"`
	RepoName        string              `json:"repoName"`
	Status          RepoStatus          `json:"status"`
	CommitsPulled   int                 `json:"commitsPulled"`
	ConflictFiles   []string            `json:"conflictFiles,omitempty"`
	ErrorMessage    string              `json:"errorMessage,omitempty"`
	AgentUsed       string              `json:"agentUsed,omitempty"`
	ConflictsFound  int                 `json:"conflictsFound,omitempty"`
	AutoResolved    int                 `json:"autoResolved,omitempty"`
	PendingConfirm  []string            `json:"pendingConfirm,omitempty"`
	PostSyncResults []PostSyncResult    `json:"postSyncResults,omitempty"`
	AgentResult     *AgentResolveResult `json:"agentResult,omitempty"`
}

// PostSyncResult represents the result of running a single post-sync command.
type PostSyncResult struct {
	Name    string `json:"name"`
	Cmd     string `json:"cmd"`
	Success bool   `json:"success"`
	Output  string `json:"output,omitempty"`
	Error   string `json:"error,omitempty"`
}

// PostSyncCommandsData is the response for post-sync commands management.
type PostSyncCommandsData struct {
	Commands []PostSyncCommand `json:"commands"`
}

// ConflictFile 冲突文件（简化版，不再包含文件内容，由 agent 自行读取）
type ConflictFile struct {
	Path string `json:"path"`
}

// --- Agent 相关类型 ---

// AgentInfo Agent CLI 信息
type AgentInfo struct {
	Name      string `json:"name"`
	Binary    string `json:"binary"`
	Path      string `json:"path"`
	Installed bool   `json:"installed"`
	Version   string `json:"version,omitempty"`
}

// AgentSessionInfo Agent 会话信息
type AgentSessionInfo struct {
	ID         string    `json:"id"`
	RepoID     string    `json:"repoId"`
	AgentName  string    `json:"agentName"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"createdAt"`
	LastUsedAt time.Time `json:"lastUsedAt"`
}

// AgentResolveResult Agent 解决结果
type AgentResolveResult struct {
	Success       bool     `json:"success"`
	ResolvedFiles []string `json:"resolvedFiles"`
	Diff          string   `json:"diff"`
	Summary       string   `json:"summary"`
	SessionID     string   `json:"sessionId"`
	AgentName     string   `json:"agentName"`
}

// AgentResolveRequest Agent 解决请求
type AgentResolveRequest struct {
	RepoID      string   `json:"repoId"`
	Files       []string `json:"files"`
	Strategy    string   `json:"strategy"`
	AutoConfirm bool     `json:"autoConfirm"`
}

// --- 命令响应类型 ---

// StatusData status 响应
type StatusData struct {
	Repos          []Repo      `json:"repos"`
	Agents         []AgentInfo `json:"agents"`
	PreferredAgent string      `json:"preferredAgent"`
}

// ScanData scan 响应
type ScanData struct {
	Repos []ScannedRepo `json:"repos"`
}

// SyncData sync 响应
type SyncData struct {
	Results []SyncResult `json:"results"`
}

// AddData add 响应
type AddData struct {
	Repo Repo `json:"repo"`
}

// ResolveData resolve 响应
type ResolveData struct {
	RepoID      string              `json:"repoId"`
	Conflicts   []ConflictFile      `json:"conflicts"`
	AgentResult *AgentResolveResult `json:"agentResult,omitempty"`
}

// AcceptData accept 响应 (--accept 或 --no-confirm 後)
type AcceptData struct {
	RepoID             string   `json:"repoId"`
	Resolved           bool     `json:"resolved"`
	RemainingConflicts []string `json:"remainingConflicts,omitempty"`
}

// RejectData reject 响应
type RejectData struct {
	RepoID     string `json:"repoId"`
	RolledBack bool   `json:"rolledBack"`
}

// AgentListData agent list 响应
type AgentListData struct {
	Agents    []AgentInfo `json:"agents"`
	Preferred string      `json:"preferred"`
}

// AgentSessionsData agent sessions 响应
type AgentSessionsData struct {
	Sessions []AgentSessionInfo `json:"sessions"`
}

// HistoryData history 响应
type HistoryData struct {
	Records []SyncHistoryRecord `json:"records"`
}

// SyncHistoryRecord 同步历史记录（从 SQLite 读取）
type SyncHistoryRecord struct {
	ID             int64    `json:"id"`
	RepoID         string   `json:"repoId"`
	RepoName       string   `json:"repoName"`
	Status         string   `json:"status"`
	CommitsPulled  int      `json:"commitsPulled"`
	ConflictFiles  []string `json:"conflictFiles"`
	AgentUsed      string   `json:"agentUsed"`
	ConflictsFound int      `json:"conflictsFound"`
	AutoResolved   int      `json:"autoResolved"`
	ErrorMessage   string   `json:"errorMessage"`
	Summary        string   `json:"summary"`
	SummaryStatus  string   `json:"summaryStatus"`
	CreatedAt      string   `json:"createdAt"`
}
