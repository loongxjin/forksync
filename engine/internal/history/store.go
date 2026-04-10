// Package history provides SQLite-backed sync history storage.
// It records every sync operation for audit and timeline display.
package history

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// Record represents a single sync history entry.
type Record struct {
	ID             int64     `json:"id"`
	RepoID         string    `json:"repoId"`
	RepoName       string    `json:"repoName"`
	Status         string    `json:"status"` // synced, conflict, error
	CommitsPulled  int       `json:"commitsPulled"`
	ConflictFiles  []string  `json:"conflictFiles"`
	AgentUsed      string    `json:"agentUsed"`
	ConflictsFound int       `json:"conflictsFound"`
	AutoResolved   int       `json:"autoResolved"`
	ErrorMessage   string    `json:"errorMessage"`
	CreatedAt      time.Time `json:"createdAt"`
}

// Store is the SQLite-backed sync history store.
type Store struct {
	mu sync.Mutex
	db *sql.DB
}

// NewStore creates (or opens) a SQLite history database at dbPath.
func NewStore(configDir string) (*Store, error) {
	dbDir := filepath.Join(configDir, "db")
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		return nil, fmt.Errorf("create db directory: %w", err)
	}

	dbPath := filepath.Join(dbDir, "forksync.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

const schema = `
CREATE TABLE IF NOT EXISTS sync_history (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	repo_id TEXT NOT NULL,
	repo_name TEXT NOT NULL,
	status TEXT NOT NULL,
	commits_pulled INTEGER DEFAULT 0,
	conflict_files TEXT DEFAULT '[]',
	agent_used TEXT DEFAULT '',
	conflicts_found INTEGER DEFAULT 0,
	auto_resolved INTEGER DEFAULT 0,
	error_message TEXT DEFAULT '',
	created_at DATETIME DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_sync_history_repo_id ON sync_history(repo_id);
CREATE INDEX IF NOT EXISTS idx_sync_history_created_at ON sync_history(created_at);
`

func (s *Store) migrate() error {
	_, err := s.db.Exec(schema)
	return err
}

// Record inserts a new sync history entry.
func (s *Store) Record(r Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	conflictFiles, err := json.Marshal(r.ConflictFiles)
	if err != nil {
		return fmt.Errorf("marshal conflict files: %w", err)
	}

	_, err = s.db.Exec(`
		INSERT INTO sync_history (repo_id, repo_name, status, commits_pulled, conflict_files, agent_used, conflicts_found, auto_resolved, error_message, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.RepoID, r.RepoName, r.Status, r.CommitsPulled, string(conflictFiles),
		r.AgentUsed, r.ConflictsFound, r.AutoResolved, r.ErrorMessage, r.CreatedAt,
	)
	return err
}

// Recent returns the most recent N sync history records, ordered by created_at DESC.
func (s *Store) Recent(n int) ([]Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rows, err := s.db.Query(`
		SELECT id, repo_id, repo_name, status, commits_pulled, conflict_files, agent_used, conflicts_found, auto_resolved, error_message, created_at
		FROM sync_history ORDER BY created_at DESC LIMIT ?`, n)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanRecords(rows)
}

// ByRepo returns recent sync history for a specific repo, limited to N records.
func (s *Store) ByRepo(repoID string, n int) ([]Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rows, err := s.db.Query(`
		SELECT id, repo_id, repo_name, status, commits_pulled, conflict_files, agent_used, conflicts_found, auto_resolved, error_message, created_at
		FROM sync_history WHERE repo_id = ? ORDER BY created_at DESC LIMIT ?`, repoID, n)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanRecords(rows)
}

// Summary returns aggregated stats: total syncs, conflicts, errors, and last sync time.
func (s *Store) Summary() (totalSyncs int, conflicts int, errors int, lastSync time.Time, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var lastSyncStr string
	err = s.db.QueryRow(`
		SELECT
			COUNT(*),
			COALESCE(SUM(CASE WHEN status = 'conflict' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN status = 'error' THEN 1 ELSE 0 END), 0),
			COALESCE(MAX(created_at), '')
		FROM sync_history`).Scan(&totalSyncs, &conflicts, &errors, &lastSyncStr)
	if err != nil {
		return
	}
	if lastSyncStr != "" {
		lastSync, _ = time.Parse(time.RFC3339, lastSyncStr)
	}
	return
}

// Close closes the underlying database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

func scanRecords(rows *sql.Rows) ([]Record, error) {
	var records []Record
	for rows.Next() {
		var r Record
		var conflictFilesJSON string
		var createdAtStr string

		if err := rows.Scan(&r.ID, &r.RepoID, &r.RepoName, &r.Status, &r.CommitsPulled,
			&conflictFilesJSON, &r.AgentUsed, &r.ConflictsFound, &r.AutoResolved,
			&r.ErrorMessage, &createdAtStr); err != nil {
			return nil, err
		}

		if err := json.Unmarshal([]byte(conflictFilesJSON), &r.ConflictFiles); err != nil {
			r.ConflictFiles = []string{}
		}

		parsed, err := time.Parse("2006-01-02T15:04:05Z07:00", createdAtStr)
		if err != nil {
			parsed, _ = time.Parse(time.RFC3339, createdAtStr)
		}
		r.CreatedAt = parsed

		records = append(records, r)
	}
	return records, rows.Err()
}
