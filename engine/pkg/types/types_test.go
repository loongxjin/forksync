package types

import (
	"encoding/json"
	"testing"
)

func TestRepoStatusConstants(t *testing.T) {
	tests := []struct {
		name   string
		status RepoStatus
		want   string
	}{
		{"synced", RepoStatusSynced, "synced"},
		{"syncing", RepoStatusSyncing, "syncing"},
		{"conflict", RepoStatusConflict, "conflict"},
		{"resolving", RepoStatusResolving, "resolving"},
		{"resolved", RepoStatusResolved, "resolved"},
		{"error", RepoStatusError, "error"},
		{"unconfigured", RepoStatusUnconfigured, "unconfigured"},
		{"up_to_date", RepoStatusUpToDate, "up_to_date"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.status) != tt.want {
				t.Errorf("RepoStatus %s = %q; want %q", tt.name, tt.status, tt.want)
			}
		})
	}
}

func TestConflictFileJSON(t *testing.T) {
	cf := ConflictFile{Path: "pkg/handler.go"}
	data, err := json.Marshal(cf)
	if err != nil {
		t.Fatalf("marshal ConflictFile: %v", err)
	}

	// Verify only "path" field appears (no oursContent, theirsContent, etc.)
	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if _, ok := parsed["oursContent"]; ok {
		t.Error("ConflictFile should not have oursContent field")
	}
	if _, ok := parsed["theirsContent"]; ok {
		t.Error("ConflictFile should not have theirsContent field")
	}
	if _, ok := parsed["mergedContent"]; ok {
		t.Error("ConflictFile should not have mergedContent field")
	}
	if _, ok := parsed["aiExplanation"]; ok {
		t.Error("ConflictFile should not have aiExplanation field")
	}
}

func TestAgentInfoJSON(t *testing.T) {
	info := AgentInfo{
		Name:      "claude",
		Binary:    "claude",
		Path:      "/usr/local/bin/claude",
		Installed: true,
		Version:   "1.0.3",
	}

	data, err := json.Marshal(info)
	if err != nil {
		t.Fatalf("marshal AgentInfo: %v", err)
	}

	var parsed AgentInfo
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal AgentInfo: %v", err)
	}

	if parsed.Name != "claude" {
		t.Errorf("Name = %q; want %q", parsed.Name, "claude")
	}
	if !parsed.Installed {
		t.Error("Installed should be true")
	}
}

func TestAgentResolveResultJSON(t *testing.T) {
	result := AgentResolveResult{
		Success:       true,
		ResolvedFiles: []string{"a.go", "b.go"},
		Diff:          "--- a.go\n+++ a.go\n@@ ...",
		Summary:       "resolved 2 files",
		SessionID:     "sess-123",
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal AgentResolveResult: %v", err)
	}

	var parsed AgentResolveResult
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(parsed.ResolvedFiles) != 2 {
		t.Errorf("ResolvedFiles length = %d; want 2", len(parsed.ResolvedFiles))
	}
	if parsed.SessionID != "sess-123" {
		t.Errorf("SessionID = %q; want %q", parsed.SessionID, "sess-123")
	}
}

func TestSyncResultAgentFields(t *testing.T) {
	sr := SyncResult{
		RepoID:         "repo-1",
		RepoName:       "my-repo",
		Status:         RepoStatusResolved,
		CommitsPulled:  5,
		AgentUsed:      "claude",
		ConflictsFound: 3,
		AutoResolved:   3,
		PendingConfirm: []string{"a.go"},
	}

	data, err := json.Marshal(sr)
	if err != nil {
		t.Fatalf("marshal SyncResult: %v", err)
	}

	var parsed SyncResult
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if parsed.AgentUsed != "claude" {
		t.Errorf("AgentUsed = %q; want %q", parsed.AgentUsed, "claude")
	}
	if parsed.Status != RepoStatusResolved {
		t.Errorf("Status = %q; want %q", parsed.Status, RepoStatusResolved)
	}
}

func TestStatusDataWithAgents(t *testing.T) {
	sd := StatusData{
		Repos: []Repo{
			{ID: "1", Name: "repo1"},
		},
		Agents: []AgentInfo{
			{Name: "claude", Installed: true},
			{Name: "opencode", Installed: false},
		},
		PreferredAgent: "claude",
	}

	data, err := json.Marshal(sd)
	if err != nil {
		t.Fatalf("marshal StatusData: %v", err)
	}

	var parsed StatusData
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(parsed.Agents) != 2 {
		t.Errorf("Agents length = %d; want 2", len(parsed.Agents))
	}
	if parsed.PreferredAgent != "claude" {
		t.Errorf("PreferredAgent = %q; want %q", parsed.PreferredAgent, "claude")
	}
}

func TestResolveDataWithAgentResult(t *testing.T) {
	rd := ResolveData{
		RepoID: "repo-1",
		Conflicts: []ConflictFile{
			{Path: "a.go"},
			{Path: "b.go"},
		},
		AgentResult: &AgentResolveResult{
			Success:       true,
			ResolvedFiles: []string{"a.go", "b.go"},
			Diff:          "diff content",
			Summary:       "resolved",
			SessionID:     "sess-1",
		},
	}

	data, err := json.Marshal(rd)
	if err != nil {
		t.Fatalf("marshal ResolveData: %v", err)
	}

	var parsed ResolveData
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if parsed.AgentResult == nil {
		t.Fatal("AgentResult should not be nil")
	}
	if len(parsed.AgentResult.ResolvedFiles) != 2 {
		t.Errorf("AgentResult.ResolvedFiles length = %d; want 2", len(parsed.AgentResult.ResolvedFiles))
	}
}
