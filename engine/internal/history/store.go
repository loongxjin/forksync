// Package history provides SQLite-backed sync history storage.
// It records every sync operation for audit and timeline display.
package history

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
	Summary        string    `json:"summary"`
	SummaryStatus  string    `json:"summaryStatus"`
	OldHEAD        string    `json:"oldHEAD"`
	CreatedAt      time.Time `json:"createdAt"`
}

// Store is the SQLite-backed sync history store.
type Store struct {
	mu sync.RWMutex
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

// migration defines a single schema migration step.
type migration struct {
	Name string
	SQL  string
}

var migrations = []migration{
	{Name: "add_summary", SQL: "ALTER TABLE sync_history ADD COLUMN summary TEXT DEFAULT ''"},
	{Name: "add_summary_status", SQL: "ALTER TABLE sync_history ADD COLUMN summary_status TEXT DEFAULT ''"},
	{Name: "add_old_head", SQL: "ALTER TABLE sync_history ADD COLUMN old_head TEXT DEFAULT ''"},
}

func (s *Store) migrate() error {
	if _, err := s.db.Exec(schema); err != nil {
		return err
	}
	// Ensure migration tracking table exists
	if _, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (name TEXT PRIMARY KEY)`); err != nil {
		return fmt.Errorf("create migration table: %w", err)
	}
	// Run pending migrations in order.
	// If a migration fails with "duplicate column", treat it as already applied
	// and record it so the transition from the old additive scheme is seamless.
	for _, m := range migrations {
		var applied string
		if err := s.db.QueryRow(`SELECT name FROM schema_migrations WHERE name = ?`, m.Name).Scan(&applied); err == nil {
			continue // already applied
		}
		if _, err := s.db.Exec(m.SQL); err != nil {
			if strings.Contains(err.Error(), "duplicate column") {
				// Column already exists from the old migration scheme — record it and continue.
				if _, insertErr := s.db.Exec(`INSERT INTO schema_migrations (name) VALUES (?)`, m.Name); insertErr != nil {
					return fmt.Errorf("record migration %s: %w", m.Name, insertErr)
				}
				continue
			}
			return fmt.Errorf("migration %s failed: %w", m.Name, err)
		}
		if _, err := s.db.Exec(`INSERT INTO schema_migrations (name) VALUES (?)`, m.Name); err != nil {
			return fmt.Errorf("record migration %s: %w", m.Name, err)
		}
	}
	return nil
}

// Record inserts a new sync history entry and returns the inserted row ID.
func (s *Store) Record(r Record) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	conflictFiles, err := json.Marshal(r.ConflictFiles)
	if err != nil {
		return 0, fmt.Errorf("marshal conflict files: %w", err)
	}

	result, err := s.db.Exec(`
		INSERT INTO sync_history (repo_id, repo_name, status, commits_pulled, conflict_files, agent_used, conflicts_found, auto_resolved, error_message, summary, summary_status, old_head, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.RepoID, r.RepoName, r.Status, r.CommitsPulled, string(conflictFiles),
		r.AgentUsed, r.ConflictsFound, r.AutoResolved, r.ErrorMessage,
		r.Summary, r.SummaryStatus, r.OldHEAD, r.CreatedAt,
	)
	if err != nil {
		return 0, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("get last insert id: %w", err)
	}
	return id, nil
}

// Recent returns the most recent N sync history records, ordered by created_at DESC.
func (s *Store) Recent(n int) ([]Record, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`
		SELECT id, repo_id, repo_name, status, commits_pulled, conflict_files, agent_used, conflicts_found, auto_resolved, error_message, COALESCE(summary, ''), COALESCE(summary_status, ''), COALESCE(old_head, ''), created_at
		FROM sync_history ORDER BY created_at DESC LIMIT ?`, n)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanRecords(rows)
}

// ByRepo returns recent sync history for a specific repo, limited to N records.
func (s *Store) ByRepo(repoID string, n int) ([]Record, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`
		SELECT id, repo_id, repo_name, status, commits_pulled, conflict_files, agent_used, conflicts_found, auto_resolved, error_message, COALESCE(summary, ''), COALESCE(summary_status, ''), COALESCE(old_head, ''), created_at
		FROM sync_history WHERE repo_id = ? ORDER BY created_at DESC LIMIT ?`, repoID, n)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanRecords(rows)
}

// UpdateSummary updates the summary and summary_status for a history record.
func (s *Store) UpdateSummary(id int64, summary, status string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec(
		`UPDATE sync_history SET summary = ?, summary_status = ? WHERE id = ?`,
		summary, status, id,
	)
	return err
}

// UpdateStatus updates the sync status for a history record.
func (s *Store) UpdateStatus(id int64, status string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec(
		`UPDATE sync_history SET status = ? WHERE id = ?`,
		status, id,
	)
	return err
}

// GetByID returns a single history record by ID.
func (s *Store) GetByID(id int64) (*Record, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`
		SELECT id, repo_id, repo_name, status, commits_pulled, conflict_files, agent_used, conflicts_found, auto_resolved, error_message, COALESCE(summary, ''), COALESCE(summary_status, ''), COALESCE(old_head, ''), created_at
		FROM sync_history WHERE id = ?`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	records, err := scanRecords(rows)
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("record not found: %d", id)
	}
	return &records[0], nil
}

// LatestByRepo returns the most recent history record for a specific repo.
func (s *Store) LatestByRepo(repoID string) (*Record, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`
		SELECT id, repo_id, repo_name, status, commits_pulled, conflict_files, agent_used, conflicts_found, auto_resolved, error_message, COALESCE(summary, ''), COALESCE(summary_status, ''), COALESCE(old_head, ''), created_at
		FROM sync_history WHERE repo_id = ? ORDER BY created_at DESC LIMIT 1`, repoID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	records, err := scanRecords(rows)
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("no history for repo: %s", repoID)
	}
	return &records[0], nil
}

// Summary returns aggregated stats: total syncs, conflicts, errors, and last sync time.
func (s *Store) Summary() (totalSyncs int, conflicts int, errors int, lastSync time.Time, err error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

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

// ClearAll removes all sync history records and returns the number of deleted rows.
func (s *Store) ClearAll() (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	res, err := s.db.Exec(`DELETE FROM sync_history`)
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("get rows affected: %w", err)
	}
	return n, nil
}

// ClearByRepo removes sync history for a specific repo and returns the number of deleted rows.
func (s *Store) ClearByRepo(repoID string) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	res, err := s.db.Exec(`DELETE FROM sync_history WHERE repo_id = ?`, repoID)
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("get rows affected: %w", err)
	}
	return n, nil
}

// ClearBefore removes sync history older than the given time and returns the number of deleted rows.
func (s *Store) ClearBefore(before time.Time) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	res, err := s.db.Exec(`DELETE FROM sync_history WHERE created_at < ?`, before.Format(time.RFC3339))
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("get rows affected: %w", err)
	}
	return n, nil
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
			&r.ErrorMessage, &r.Summary, &r.SummaryStatus, &r.OldHEAD, &createdAtStr); err != nil {
			return nil, err
		}

		if err := json.Unmarshal([]byte(conflictFilesJSON), &r.ConflictFiles); err != nil {
			r.ConflictFiles = []string{}
		}

		parsed, err := time.Parse("2006-01-02T15:04:05Z07:00", createdAtStr)
		if err != nil {
			parsed, err = time.Parse(time.RFC3339, createdAtStr)
			if err != nil {
				parsed = time.Time{}
			}
		}
		r.CreatedAt = parsed

		records = append(records, r)
	}
	return records, rows.Err()
}
