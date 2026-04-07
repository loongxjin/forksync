package session

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/loongxjin/forksync/engine/internal/agent"
	"github.com/loongxjin/forksync/engine/pkg/types"
)

const defaultSessionTTL = 24 * time.Hour

// Manager manages agent sessions for all repositories.
// It provides the high-level API that the rest of the codebase uses.
type Manager struct {
	store    *SessionStore
	provider agent.AgentProvider
	mu       sync.Mutex
	active   map[string]*agent.Session // in-memory cache: repoID → session
	ttl      time.Duration
}

// NewManager creates a new session manager.
func NewManager(store *SessionStore, provider agent.AgentProvider) *Manager {
	return &Manager{
		store:    store,
		provider: provider,
		active:   make(map[string]*agent.Session),
		ttl:      defaultSessionTTL,
	}
}

// ProviderName returns the name of the underlying agent provider.
func (m *Manager) ProviderName() string {
	if m.provider == nil {
		return "unknown"
	}
	return m.provider.Name()
}

// SetTTL configures the session expiration duration.
func (m *Manager) SetTTL(d time.Duration) {
	m.ttl = d
}

// GetOrCreate returns an active session for the given repository.
// If an active session exists (in memory or on disk), it is reused.
// If the session is expired, a new one is created.
func (m *Manager) GetOrCreate(ctx context.Context, repoID, repoPath string) (*agent.Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 1. Check in-memory cache
	if sess, ok := m.active[repoID]; ok {
		return sess, nil
	}

	// 2. Check disk store
	rec, err := m.store.Load(repoID)
	if err == nil && rec.Status == "active" {
		// Check if expired
		if time.Since(rec.LastUsedAt) < m.ttl {
			sess := &agent.Session{
				ID:        rec.SessionID,
				Provider:  rec.AgentName,
				RepoPath:  rec.RepoPath,
				StartedAt: rec.CreatedAt,
			}
			m.active[repoID] = sess
			return sess, nil
		}
		// Expired — mark it
		_ = m.store.UpdateStatus(repoID, "expired")
	}

	// 3. Create new session
	return m.createSession(ctx, repoID, repoPath)
}

// ResolveConflicts is the main entry point for conflict resolution.
// It ensures a session exists, builds the prompt, and calls the agent.
func (m *Manager) ResolveConflicts(ctx context.Context, repoID string, conflictFiles []string, strategy string) (*agent.AgentResult, error) {
	// Get or create session
	rec, err := m.store.Load(repoID)
	if err != nil {
		return nil, fmt.Errorf("no session for repo %s: %w", repoID, err)
	}

	sess := &agent.Session{
		ID:       rec.SessionID,
		Provider: rec.AgentName,
		RepoPath: rec.RepoPath,
	}

	// Build prompt
	prompt := agent.BuildConflictPrompt(conflictFiles, strategy)

	// Call agent
	result, err := m.provider.ResolveConflicts(ctx, sess, prompt)
	if err != nil {
		_ = m.store.UpdateStatus(repoID, "failed")
		return result, err
	}

	// Update session's last used time
	_ = m.store.UpdateLastUsed(repoID)

	// Update session ID if agent returned a new one
	if result.SessionID != "" && result.SessionID != rec.SessionID {
		rec.SessionID = result.SessionID
		_ = m.store.Save(rec)
	}

	return result, nil
}

// CreateSessionForRepo creates a new agent session for a repository.
// This is the primary method used when no session exists yet.
func (m *Manager) CreateSessionForRepo(ctx context.Context, repoID, repoPath string) (*agent.Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.createSession(ctx, repoID, repoPath)
}

// createSession is the internal implementation (caller must hold m.mu).
func (m *Manager) createSession(ctx context.Context, repoID, repoPath string) (*agent.Session, error) {
	contextFiles := scanContextFiles(repoPath)

	sess, err := m.provider.StartSession(ctx, agent.SessionOptions{
		RepoPath:     repoPath,
		RepoName:     filepath.Base(repoPath),
		ContextFiles: contextFiles,
	})
	if err != nil {
		return nil, fmt.Errorf("start agent session for %s: %w", repoID, err)
	}

	// Persist session record
	rec := &SessionRecord{
		RepoID:     repoID,
		RepoPath:   repoPath,
		AgentName:  m.provider.Name(),
		SessionID:  sess.ID,
		CreatedAt:  time.Now(),
		LastUsedAt: time.Now(),
		Status:     "active",
	}
	if err := m.store.Save(rec); err != nil {
		return nil, fmt.Errorf("save session record: %w", err)
	}

	m.active[repoID] = sess
	return sess, nil
}

// CloseAll terminates all active sessions and clears the in-memory cache.
func (m *Manager) CloseAll() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var lastErr error
	for repoID, sess := range m.active {
		if err := m.provider.EndSession(context.Background(), sess.ID); err != nil {
			lastErr = err
		}
		delete(m.active, repoID)
	}
	return lastErr
}

// CleanupExpired removes expired and failed session records.
func (m *Manager) CleanupExpired() (int, error) {
	return m.store.CleanupExpired()
}

// ListSessions returns all persisted session records.
func (m *Manager) ListSessions() ([]*SessionRecord, error) {
	return m.store.ListAll()
}

// ListSessionsAsInfo converts session records to AgentSessionInfo for API output.
func (m *Manager) ListSessionsAsInfo() ([]types.AgentSessionInfo, error) {
	records, err := m.store.ListAll()
	if err != nil {
		return nil, err
	}

	var infos []types.AgentSessionInfo
	for _, rec := range records {
		infos = append(infos, types.AgentSessionInfo{
			ID:         rec.SessionID,
			RepoID:     rec.RepoID,
			AgentName:  rec.AgentName,
			Status:     rec.Status,
			CreatedAt:  rec.CreatedAt,
			LastUsedAt: rec.LastUsedAt,
		})
	}
	return infos, nil
}

// scanContextFiles looks for common project context files in a repo.
// Returns file names (relative paths) that exist.
func scanContextFiles(repoPath string) []string {
	candidates := []string{
		"README.md", "README", "README.txt",
		"CONTRIBUTING.md",
		"AGENTS.md",
		".editorconfig",
		"go.mod", "package.json", "Cargo.toml",
	}

	var found []string
	for _, f := range candidates {
		if _, err := os.Stat(filepath.Join(repoPath, f)); err == nil {
			found = append(found, f)
		}
	}
	return found
}

// readContextFileContent reads a context file and returns its content.
// Used by adapters that support context injection (e.g., Claude --print).
// Returns empty string if file doesn't exist or can't be read.
func readContextFileContent(repoPath, filename string) string {
	data, err := os.ReadFile(filepath.Join(repoPath, filename))
	if err != nil {
		return ""
	}
	// Limit to 4KB per file to avoid overwhelming the prompt
	const maxLen = 4096
	if len(data) > maxLen {
		return string(data[:maxLen]) + "\n... (truncated)"
	}
	return string(data)
}
