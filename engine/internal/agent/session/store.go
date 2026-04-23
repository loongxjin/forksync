package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/loongxjin/forksync/engine/pkg/types"
)

// SessionRecord represents a persisted agent session for a repository.
type SessionRecord struct {
	RepoID     string    `json:"repoId"`
	RepoPath   string    `json:"repoPath"`
	AgentName  string    `json:"agentName"`
	SessionID  string    `json:"sessionId"`
	CreatedAt  time.Time `json:"createdAt"`
	LastUsedAt time.Time `json:"lastUsedAt"`
	Status     string    `json:"status"` // types.SessionStatus values
}

// SessionStore handles persistence of session records to disk.
// Each repository has its own JSON file at <dir>/<repoID>.json.
type SessionStore struct {
	dir string // ~/.forksync/sessions/
	mu  sync.Mutex
}

// NewSessionStore creates a new SessionStore rooted at the given directory.
func NewSessionStore(dir string) *SessionStore {
	return &SessionStore{dir: dir}
}

// Init ensures the session directory exists.
func (s *SessionStore) Init() error {
	return os.MkdirAll(s.dir, 0755)
}

// Save persists a session record to disk.
func (s *SessionStore) Save(rec *SessionRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.Init(); err != nil {
		return fmt.Errorf("init session dir: %w", err)
	}

	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal session record: %w", err)
	}

	path := s.filePath(rec.RepoID)
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("write session file: %w", err)
	}

	return nil
}

// Load reads a session record from disk.
func (s *SessionStore) Load(repoID string) (*SessionRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadLocked(repoID)
}

// loadLocked is the internal implementation of Load (caller must hold s.mu).
func (s *SessionStore) loadLocked(repoID string) (*SessionRecord, error) {
	path := s.filePath(repoID)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("session not found for repo %s", repoID)
		}
		return nil, fmt.Errorf("read session file: %w", err)
	}

	var rec SessionRecord
	if err := json.Unmarshal(data, &rec); err != nil {
		return nil, fmt.Errorf("parse session file: %w", err)
	}

	return &rec, nil
}

// Delete removes a session record from disk.
func (s *SessionStore) Delete(repoID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	path := s.filePath(repoID)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete session file: %w", err)
	}
	return nil
}

// UpdateStatus changes the status field of a session record.
func (s *SessionStore) UpdateStatus(repoID, status string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, err := s.loadLocked(repoID)
	if err != nil {
		return err
	}
	rec.Status = status
	return s.saveLocked(rec)
}

// UpdateLastUsed updates the LastUsedAt timestamp.
func (s *SessionStore) UpdateLastUsed(repoID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, err := s.loadLocked(repoID)
	if err != nil {
		return err
	}
	rec.LastUsedAt = time.Now()
	return s.saveLocked(rec)
}

// saveLocked is the internal implementation of Save (caller must hold s.mu).
func (s *SessionStore) saveLocked(rec *SessionRecord) error {
	if err := s.Init(); err != nil {
		return fmt.Errorf("init session dir: %w", err)
	}
	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal session record: %w", err)
	}
	path := s.filePath(rec.RepoID)
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("write session file: %w", err)
	}
	return nil
}

// ListAll returns all session records in the store.
func (s *SessionStore) ListAll() ([]*SessionRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.Init(); err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, fmt.Errorf("read session dir: %w", err)
	}

	records := make([]*SessionRecord, 0)
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		data, err := os.ReadFile(filepath.Join(s.dir, entry.Name()))
		if err != nil {
			continue
		}

		var rec SessionRecord
		if err := json.Unmarshal(data, &rec); err != nil {
			continue
		}
		records = append(records, &rec)
	}

	return records, nil
}

// CleanupExpired removes all session files with status "expired" or "failed".
func (s *SessionStore) CleanupExpired() (int, error) {
	records, err := s.ListAll()
	if err != nil {
		return 0, err
	}

	cleaned := 0
	for _, rec := range records {
		if rec.Status == string(types.SessionStatusExpired) || rec.Status == string(types.SessionStatusFailed) {
			if err := s.Delete(rec.RepoID); err == nil {
				cleaned++
			}
		}
	}

	return cleaned, nil
}

func (s *SessionStore) filePath(repoID string) string {
	return filepath.Join(s.dir, repoID+".json")
}
