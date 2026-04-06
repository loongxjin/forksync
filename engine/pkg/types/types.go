package types

import (
	"fmt"
	"time"
)

// ApiResponse 通用 JSON 响应格式
type ApiResponse[T any] struct {
	Success bool   `json:"success"`
	Data    T      `json:"data"`
	Error   string `json:"error"`
}

// RepoStatus 仓库状态枚举
type RepoStatus string

const (
	RepoStatusSynced       RepoStatus = "synced"
	RepoStatusSyncing      RepoStatus = "syncing"
	RepoStatusConflict     RepoStatus = "conflict"
	RepoStatusResolving    RepoStatus = "resolving"  // agent 正在解决冲突
	RepoStatusResolved     RepoStatus = "resolved"   // agent 解决完成，等待用户确认
	RepoStatusError        RepoStatus = "error"
	RepoStatusUnconfigured RepoStatus = "unconfigured"
	RepoStatusUpToDate     RepoStatus = "up_to_date"
)

// Time 可序列化的 time.Time
type Time struct {
	time.Time
}

func (t Time) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf("\"%s\"", t.Format(time.RFC3339))), nil
}

func (t *Time) UnmarshalJSON(data []byte) error {
	str := string(data)
	str = str[1 : len(str)-1]
	parsed, err := time.Parse(time.RFC3339, str)
	if err != nil {
		return err
	}
	t.Time = parsed
	return nil
}

// Repo 仓库配置模型
type Repo struct {
	ID               string     `json:"id"`
	Name             string     `json:"name"`
	Path             string     `json:"path"`
	Origin           string     `json:"origin"`
	Upstream         string     `json:"upstream"`
	Branch           string     `json:"branch"`
	AutoSync         bool       `json:"autoSync"`
	SyncInterval     string     `json:"syncInterval"`
	ConflictStrategy string     `json:"conflictStrategy"`
	LastSync         *Time      `json:"lastSync"`
	Status           RepoStatus `json:"status"`
	AheadBy          int        `json:"aheadBy"`
	BehindBy         int        `json:"behindBy"`
	ErrorMessage     string     `json:"errorMessage,omitempty"`
}

// ScannedRepo 扫描结果
type ScannedRepo struct {
	Path              string `json:"path"`
	Name              string `json:"name"`
	Origin            string `json:"origin"`
	IsFork            bool   `json:"isFork"`
	SuggestedUpstream string `json:"suggestedUpstream,omitempty"`
}

// SyncResult 同步结果
type SyncResult struct {
	RepoID         string     `json:"repoId"`
	RepoName       string     `json:"repoName"`
	Status         RepoStatus `json:"status"`
	CommitsPulled  int        `json:"commitsPulled"`
	ConflictFiles  []string   `json:"conflictFiles,omitempty"`
	ErrorMessage   string     `json:"errorMessage,omitempty"`
	AgentUsed      string     `json:"agentUsed,omitempty"`
	ConflictsFound int        `json:"conflictsFound,omitempty"`
	AutoResolved   int        `json:"autoResolved,omitempty"`
	PendingConfirm []string   `json:"pendingConfirm,omitempty"`
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

// AcceptData accept 响应
type AcceptData struct {
	RepoID   string `json:"repoId"`
	Resolved bool   `json:"resolved"`
}

// RejectData reject 响应
type RejectData struct {
	RepoID     string `json:"repoId"`
	RolledBack bool   `json:"rolledBack"`
}

// DoneData done 响应
type DoneData struct {
	RepoID             string   `json:"repoId"`
	AllResolved        bool     `json:"allResolved"`
	RemainingConflicts []string `json:"remainingConflicts,omitempty"`
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
