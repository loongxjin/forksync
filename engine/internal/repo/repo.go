package repo

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/loongxjin/forksync/engine/pkg/types"
)

var (
	ErrRepoExists   = errors.New("repository already exists")
	ErrRepoNotFound = errors.New("repository not found")
	ErrInvalidRepo  = errors.New("invalid repository: name is required")
)

type Store interface {
	List() ([]types.Repo, error)
	Get(id string) (types.Repo, bool)
	GetByName(name string) (types.Repo, bool)
	Add(repo types.Repo) error
	Update(repo types.Repo) error
	Remove(id string) error
}

type JSONStore struct {
	mu        sync.RWMutex
	path      string
	repos     map[string]types.Repo
	nameIndex map[string]string
}

func NewJSONStore(configDir string) *JSONStore {
	return &JSONStore{
		path:      filepath.Join(configDir, "repos.json"),
		repos:     make(map[string]types.Repo),
		nameIndex: make(map[string]string),
	}
}

func (s *JSONStore) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var repos []types.Repo
	if err := json.Unmarshal(data, &repos); err != nil {
		return err
	}

	// Migrate legacy status values
	migrated := false
	for i, r := range repos {
		if r.Status == "synced" {
			repos[i].Status = types.RepoStatusUpToDate
			migrated = true
		}
	}

	s.repos = make(map[string]types.Repo)
	s.nameIndex = make(map[string]string)
	for _, r := range repos {
		s.repos[r.ID] = r
		s.nameIndex[r.Name] = r.ID
	}

	// Persist migration if any repos were updated
	if migrated {
		_ = s.saveUnsafe()
	}

	return nil
}

// Save persists the current state to disk. Uses write lock for safe concurrent file writes.
func (s *JSONStore) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.saveUnsafe()
}

func (s *JSONStore) List() ([]types.Repo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	repos := make([]types.Repo, 0, len(s.repos))
	for _, r := range s.repos {
		repos = append(repos, r)
	}
	sort.Slice(repos, func(i, j int) bool {
		if !repos[i].CreatedAt.Equal(repos[j].CreatedAt) {
			return repos[i].CreatedAt.Before(repos[j].CreatedAt)
		}
		return repos[i].ID < repos[j].ID
	})
	return repos, nil
}

func (s *JSONStore) Get(id string) (types.Repo, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	r, ok := s.repos[id]
	return r, ok
}

func (s *JSONStore) GetByName(name string) (types.Repo, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	id, ok := s.nameIndex[name]
	if !ok {
		return types.Repo{}, false
	}
	r, ok := s.repos[id]
	return r, ok
}

func (s *JSONStore) Add(repo types.Repo) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if repo.Name == "" {
		return ErrInvalidRepo
	}

	if repo.ID == "" {
		repo.ID = uuid.New().String()
	}

	if repo.CreatedAt.IsZero() {
		repo.CreatedAt = time.Now()
	}

	if _, exists := s.nameIndex[repo.Name]; exists {
		return ErrRepoExists
	}

	if _, exists := s.repos[repo.ID]; exists {
		return ErrRepoExists
	}

	s.repos[repo.ID] = repo
	s.nameIndex[repo.Name] = repo.ID
	return s.saveUnsafe()
}

func (s *JSONStore) Update(repo types.Repo) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if repo.Name == "" {
		return ErrInvalidRepo
	}

	old, ok := s.repos[repo.ID]
	if !ok {
		return ErrRepoNotFound
	}

	if old.Name != repo.Name {
		// Check new name doesn't collide with another repo
		if existingID, exists := s.nameIndex[repo.Name]; exists && existingID != repo.ID {
			return ErrRepoExists
		}
		delete(s.nameIndex, old.Name)
		s.nameIndex[repo.Name] = repo.ID
	}

	s.repos[repo.ID] = repo
	return s.saveUnsafe()
}

func (s *JSONStore) Remove(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	r, ok := s.repos[id]
	if !ok {
		return ErrRepoNotFound
	}

	delete(s.repos, id)
	delete(s.nameIndex, r.Name)
	return s.saveUnsafe()
}

// saveUnsafe persists repos to disk atomically via temp file + rename.
// Must be called with write lock held.
func (s *JSONStore) saveUnsafe() error {
	repos := make([]types.Repo, 0, len(s.repos))
	for _, r := range s.repos {
		repos = append(repos, r)
	}
	sort.Slice(repos, func(i, j int) bool {
		if !repos[i].CreatedAt.Equal(repos[j].CreatedAt) {
			return repos[i].CreatedAt.Before(repos[j].CreatedAt)
		}
		return repos[i].ID < repos[j].ID
	})
	data, err := json.MarshalIndent(repos, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	// Windows requires removing the target before rename
	if runtime.GOOS == "windows" {
		_ = os.Remove(s.path)
	}
	return os.Rename(tmp, s.path)
}
