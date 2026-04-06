package session

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewSessionStore(t *testing.T) {
	tmpDir := t.TempDir()
	s := NewSessionStore(tmpDir)
	if s == nil {
		t.Fatal("NewSessionStore should return non-nil")
	}
	if s.dir != tmpDir {
		t.Errorf("dir = %q; want %q", s.dir, tmpDir)
	}
}

func TestSessionStore_SaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	s := NewSessionStore(tmpDir)

	rec := &SessionRecord{
		RepoID:     "repo-123",
		RepoPath:   "/path/to/repo",
		AgentName:  "claude",
		SessionID:  "sess-abc",
		CreatedAt:  time.Now().Truncate(time.Second),
		LastUsedAt: time.Now().Truncate(time.Second),
		Status:     "active",
	}

	// Save
	if err := s.Save(rec); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Verify file exists
	expectedPath := filepath.Join(tmpDir, "repo-123.json")
	if _, err := os.Stat(expectedPath); os.IsNotExist(err) {
		t.Fatal("session file should exist after Save")
	}

	// Load
	loaded, err := s.Load("repo-123")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if loaded.RepoID != rec.RepoID {
		t.Errorf("RepoID = %q; want %q", loaded.RepoID, rec.RepoID)
	}
	if loaded.SessionID != rec.SessionID {
		t.Errorf("SessionID = %q; want %q", loaded.SessionID, rec.SessionID)
	}
	if loaded.AgentName != rec.AgentName {
		t.Errorf("AgentName = %q; want %q", loaded.AgentName, rec.AgentName)
	}
	if loaded.Status != "active" {
		t.Errorf("Status = %q; want %q", loaded.Status, "active")
	}
}

func TestSessionStore_LoadNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	s := NewSessionStore(tmpDir)

	_, err := s.Load("nonexistent")
	if err == nil {
		t.Error("Load should return error for nonexistent session")
	}
}

func TestSessionStore_Delete(t *testing.T) {
	tmpDir := t.TempDir()
	s := NewSessionStore(tmpDir)

	rec := &SessionRecord{
		RepoID:    "repo-del",
		RepoPath:  "/path",
		AgentName: "claude",
		SessionID: "sess-del",
		Status:    "active",
	}
	_ = s.Save(rec)

	if err := s.Delete("repo-del"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err := s.Load("repo-del")
	if err == nil {
		t.Error("Load should fail after Delete")
	}
}

func TestSessionStore_ListAll(t *testing.T) {
	tmpDir := t.TempDir()
	s := NewSessionStore(tmpDir)

	records := []*SessionRecord{
		{RepoID: "repo-1", AgentName: "claude", SessionID: "sess-1", Status: "active"},
		{RepoID: "repo-2", AgentName: "opencode", SessionID: "sess-2", Status: "active"},
		{RepoID: "repo-3", AgentName: "droid", SessionID: "sess-3", Status: "expired"},
	}

	for _, r := range records {
		_ = s.Save(r)
	}

	all, err := s.ListAll()
	if err != nil {
		t.Fatalf("ListAll: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("ListAll returned %d records; want 3", len(all))
	}
}

func TestSessionStore_UpdateStatus(t *testing.T) {
	tmpDir := t.TempDir()
	s := NewSessionStore(tmpDir)

	rec := &SessionRecord{
		RepoID:    "repo-upd",
		AgentName: "claude",
		SessionID: "sess-upd",
		Status:    "active",
	}
	_ = s.Save(rec)

	if err := s.UpdateStatus("repo-upd", "expired"); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}

	loaded, _ := s.Load("repo-upd")
	if loaded.Status != "expired" {
		t.Errorf("Status = %q; want %q", loaded.Status, "expired")
	}
}

func TestSessionRecordJSONRoundTrip(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	rec := &SessionRecord{
		RepoID:     "repo-json",
		RepoPath:   "/path/to/repo",
		AgentName:  "claude",
		SessionID:  "sess-json",
		CreatedAt:  now,
		LastUsedAt: now,
		Status:     "active",
	}

	tmpDir := t.TempDir()
	s := NewSessionStore(tmpDir)
	_ = s.Save(rec)

	loaded, err := s.Load("repo-json")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if !loaded.CreatedAt.Equal(now) {
		t.Errorf("CreatedAt = %v; want %v", loaded.CreatedAt, now)
	}
	if !loaded.LastUsedAt.Equal(now) {
		t.Errorf("LastUsedAt = %v; want %v", loaded.LastUsedAt, now)
	}
}
