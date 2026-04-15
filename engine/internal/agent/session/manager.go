package session

import (
	"context"
	"fmt"
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
// If resuming an existing session fails (e.g. the agent CLI lost it),
// it transparently creates a new session and retries.
func (m *Manager) ResolveConflicts(ctx context.Context, repoID, repoPath string, conflictFiles []string, strategy string) (*agent.AgentResult, error) {
	// Ensure session exists — reuse active or create new
	sess, err := m.GetOrCreate(ctx, repoID, repoPath)
	if err != nil {
		return nil, fmt.Errorf("ensure session for repo %s: %w", repoID, err)
	}

	// Build prompt — for new sessions merge system prompt + conflict task
	// into one prompt so the agent doesn't start working before receiving
	// the actual task.
	var prompt string
	if sess.IsNew {
		prompt = agent.BuildInitialConflictPrompt(conflictFiles, strategy)
	} else {
		prompt = agent.BuildConflictPrompt(conflictFiles, strategy)
	}

	// Call agent
	result, err := m.provider.ResolveConflicts(ctx, sess, prompt)
	if err != nil {
		// Resume failed — the session is stale on the agent side.
		// Discard it and create a fresh one, then retry.
		m.invalidateSession(repoID)
		sess, retryErr := m.createSessionLocked(ctx, repoID, repoPath)
		if retryErr != nil {
			_ = m.store.UpdateStatus(repoID, "failed")
			return nil, fmt.Errorf("resume failed (%v); recreate session also failed: %w", err, retryErr)
		}
		// Retry with merged prompt (this is a new session too)
		prompt = agent.BuildInitialConflictPrompt(conflictFiles, strategy)
		result, err = m.provider.ResolveConflicts(ctx, sess, prompt)
		if err != nil {
			_ = m.store.UpdateStatus(repoID, "failed")
			return result, err
		}
	}

	// Mark session as no longer new after first real interaction
	if sess.IsNew {
		sess.IsNew = false
		m.mu.Lock()
		m.active[repoID] = sess
		m.mu.Unlock()
	}

	// Update session's last used time
	_ = m.store.UpdateLastUsed(repoID)

	// Update session ID if agent returned a new one
	if result.SessionID != "" && result.SessionID != sess.ID {
		if rec, loadErr := m.store.Load(repoID); loadErr == nil {
			rec.SessionID = result.SessionID
			_ = m.store.Save(rec)
		}
		sess.ID = result.SessionID
		m.mu.Lock()
		m.active[repoID] = sess
		m.mu.Unlock()
	}

	return result, nil
}

// invalidateSession removes a session from in-memory cache and marks it
// as failed on disk, so subsequent GetOrCreate will create a new one.
func (m *Manager) invalidateSession(repoID string) {
	m.mu.Lock()
	delete(m.active, repoID)
	m.mu.Unlock()
	_ = m.store.UpdateStatus(repoID, "failed")
}

// createSessionLocked creates a new session after acquiring the mutex.
// Safe to call when the caller does not already hold m.mu.
func (m *Manager) createSessionLocked(ctx context.Context, repoID, repoPath string) (*agent.Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.createSession(ctx, repoID, repoPath)
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
	sess, err := m.provider.StartSession(ctx, agent.SessionOptions{
		RepoPath: repoPath,
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
func (m *Manager) CloseAll(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var lastErr error
	for repoID, sess := range m.active {
		if err := m.provider.EndSession(ctx, sess.ID); err != nil {
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

	infos := make([]types.AgentSessionInfo, 0, len(records))
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
