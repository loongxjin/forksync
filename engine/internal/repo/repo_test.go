package repo

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/loongxjin/forksync/engine/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestStore(t *testing.T) *JSONStore {
	t.Helper()
	dir := t.TempDir()
	s := NewJSONStore(dir)
	return s
}

func TestNewJSONStore(t *testing.T) {
	dir := t.TempDir()
	s := NewJSONStore(dir)
	require.NotNil(t, s)
	assert.Equal(t, filepath.Join(dir, "repos.json"), s.path)
	assert.Empty(t, s.repos)
	assert.Empty(t, s.nameIndex)

	// Verify it implements Store interface
	var _ Store = s
}

func TestLoad_NonExistentFile(t *testing.T) {
	s := newTestStore(t)
	err := s.Load()
	require.NoError(t, err)

	repos, err := s.List()
	require.NoError(t, err)
	assert.Empty(t, repos)
}

func TestLoad_InvalidJSON(t *testing.T) {
	s := newTestStore(t)
	// Write invalid JSON to the file
	err := os.WriteFile(s.path, []byte("not valid json"), 0644)
	require.NoError(t, err)

	err = s.Load()
	assert.Error(t, err)
}

func TestAdd_AutoGenerateUUID(t *testing.T) {
	s := newTestStore(t)
	repo := types.Repo{
		Name:     "test-repo",
		Origin:   "https://github.com/user/test-repo.git",
		Upstream: "https://github.com/original/test-repo.git",
		Branch:   "main",
	}
	err := s.Add(repo)
	require.NoError(t, err)

	// Verify ID was generated
	got, ok := s.GetByName("test-repo")
	require.True(t, ok)
	assert.NotEmpty(t, got.ID)
	assert.Equal(t, "test-repo", got.Name)
	assert.Equal(t, repo.Origin, got.Origin)
}

func TestAdd_WithExistingID(t *testing.T) {
	s := newTestStore(t)
	repo := types.Repo{
		ID:       "custom-id-123",
		Name:     "test-repo",
		Origin:   "https://github.com/user/test-repo.git",
		Upstream: "https://github.com/original/test-repo.git",
	}
	err := s.Add(repo)
	require.NoError(t, err)

	got, ok := s.Get("custom-id-123")
	require.True(t, ok)
	assert.Equal(t, "custom-id-123", got.ID)
}

func TestAdd_DuplicateName(t *testing.T) {
	s := newTestStore(t)
	repo := types.Repo{
		Name:   "dup-repo",
		Origin: "https://github.com/user/dup-repo.git",
	}
	err := s.Add(repo)
	require.NoError(t, err)

	// Add another repo with same name
	repo2 := types.Repo{
		Name:   "dup-repo",
		Origin: "https://github.com/other/dup-repo.git",
	}
	err = s.Add(repo2)
	assert.ErrorIs(t, err, ErrRepoExists)
}

func TestAdd_PersistsToFile(t *testing.T) {
	dir := t.TempDir()
	s := NewJSONStore(dir)

	repo := types.Repo{
		Name:     "persisted-repo",
		Origin:   "https://github.com/user/persisted-repo.git",
		Upstream: "https://github.com/upstream/persisted-repo.git",
		Branch:   "main",
		AutoSync: true,
	}
	err := s.Add(repo)
	require.NoError(t, err)

	// Verify file was created
	_, err = os.Stat(filepath.Join(dir, "repos.json"))
	require.NoError(t, err)

	// Verify file content
	data, err := os.ReadFile(filepath.Join(dir, "repos.json"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "persisted-repo")
}

func TestGet(t *testing.T) {
	s := newTestStore(t)
	repo := types.Repo{
		ID:     "get-id",
		Name:   "get-repo",
		Origin: "https://github.com/user/get-repo.git",
	}
	err := s.Add(repo)
	require.NoError(t, err)

	got, ok := s.Get("get-id")
	assert.True(t, ok)
	assert.Equal(t, "get-id", got.ID)
	assert.Equal(t, "get-repo", got.Name)
}

func TestGet_NotFound(t *testing.T) {
	s := newTestStore(t)
	_, ok := s.Get("nonexistent")
	assert.False(t, ok)
}

func TestGetByName(t *testing.T) {
	s := newTestStore(t)
	repo := types.Repo{
		Name:   "by-name-repo",
		Origin: "https://github.com/user/by-name-repo.git",
	}
	err := s.Add(repo)
	require.NoError(t, err)

	got, ok := s.GetByName("by-name-repo")
	assert.True(t, ok)
	assert.Equal(t, "by-name-repo", got.Name)
}

func TestGetByName_NotFound(t *testing.T) {
	s := newTestStore(t)
	_, ok := s.GetByName("nonexistent")
	assert.False(t, ok)
}

func TestList(t *testing.T) {
	s := newTestStore(t)

	repos := []types.Repo{
		{Name: "repo-a", Origin: "https://github.com/user/a.git"},
		{Name: "repo-b", Origin: "https://github.com/user/b.git"},
		{Name: "repo-c", Origin: "https://github.com/user/c.git"},
	}
	for _, r := range repos {
		err := s.Add(r)
		require.NoError(t, err)
	}

	list, err := s.List()
	require.NoError(t, err)
	assert.Len(t, list, 3)

	// Verify repos are returned in insertion order (by CreatedAt)
	assert.Equal(t, "repo-a", list[0].Name)
	assert.Equal(t, "repo-b", list[1].Name)
	assert.Equal(t, "repo-c", list[2].Name)
}

func TestList_Empty(t *testing.T) {
	s := newTestStore(t)
	list, err := s.List()
	require.NoError(t, err)
	assert.Empty(t, list)
}

func TestUpdate(t *testing.T) {
	s := newTestStore(t)
	repo := types.Repo{
		ID:     "update-id",
		Name:   "update-repo",
		Origin: "https://github.com/user/update-repo.git",
		Branch: "main",
	}
	err := s.Add(repo)
	require.NoError(t, err)

	// Update the repo
	repo.Branch = "develop"
	repo.AutoSync = true
	err = s.Update(repo)
	require.NoError(t, err)

	got, ok := s.Get("update-id")
	require.True(t, ok)
	assert.Equal(t, "develop", got.Branch)
	assert.True(t, got.AutoSync)
}

func TestUpdate_NameChange(t *testing.T) {
	s := newTestStore(t)
	repo := types.Repo{
		ID:     "name-change-id",
		Name:   "old-name",
		Origin: "https://github.com/user/old-name.git",
	}
	err := s.Add(repo)
	require.NoError(t, err)

	// Verify old name is indexed
	_, ok := s.GetByName("old-name")
	require.True(t, ok)

	// Update name
	repo.Name = "new-name"
	err = s.Update(repo)
	require.NoError(t, err)

	// Old name should no longer be indexed
	_, ok = s.GetByName("old-name")
	assert.False(t, ok)

	// New name should be indexed
	got, ok := s.GetByName("new-name")
	assert.True(t, ok)
	assert.Equal(t, "new-name", got.Name)

	// ID should remain the same
	assert.Equal(t, "name-change-id", got.ID)
}

func TestUpdate_NotFound(t *testing.T) {
	s := newTestStore(t)
	repo := types.Repo{
		ID:   "nonexistent",
		Name: "ghost",
	}
	err := s.Update(repo)
	assert.ErrorIs(t, err, ErrRepoNotFound)
}

func TestUpdate_NameConflict(t *testing.T) {
	s := newTestStore(t)
	// Add two repos
	require.NoError(t, s.Add(types.Repo{Name: "repo-a", Origin: "https://github.com/user/a.git"}))
	require.NoError(t, s.Add(types.Repo{Name: "repo-b", Origin: "https://github.com/user/b.git"}))

	// Get repo-b to have its auto-generated ID
	repoB, ok := s.GetByName("repo-b")
	require.True(t, ok)

	// Try to rename repo-b to repo-a (should fail)
	repoB.Name = "repo-a"
	err := s.Update(repoB)
	assert.ErrorIs(t, err, ErrRepoExists)
}

func TestAdd_DuplicateID(t *testing.T) {
	s := newTestStore(t)
	repo1 := types.Repo{ID: "same-id", Name: "repo-1", Origin: "https://github.com/user/1.git"}
	require.NoError(t, s.Add(repo1))

	repo2 := types.Repo{ID: "same-id", Name: "repo-2", Origin: "https://github.com/user/2.git"}
	err := s.Add(repo2)
	assert.ErrorIs(t, err, ErrRepoExists)
}

func TestRemove(t *testing.T) {
	s := newTestStore(t)
	repo := types.Repo{
		ID:     "remove-id",
		Name:   "remove-repo",
		Origin: "https://github.com/user/remove-repo.git",
	}
	err := s.Add(repo)
	require.NoError(t, err)

	err = s.Remove("remove-id")
	require.NoError(t, err)

	// Should no longer be found by ID
	_, ok := s.Get("remove-id")
	assert.False(t, ok)

	// Should no longer be found by name
	_, ok = s.GetByName("remove-repo")
	assert.False(t, ok)

	// List should be empty
	list, err := s.List()
	require.NoError(t, err)
	assert.Empty(t, list)
}

func TestRemove_NotFound(t *testing.T) {
	s := newTestStore(t)
	err := s.Remove("nonexistent")
	assert.ErrorIs(t, err, ErrRepoNotFound)
}

func TestSaveAndReload(t *testing.T) {
	dir := t.TempDir()

	// Create and populate store
	s1 := NewJSONStore(dir)
	repos := []types.Repo{
		{Name: "round-trip-a", Origin: "https://github.com/user/a.git", Upstream: "https://github.com/upstream/a.git", Branch: "main"},
		{Name: "round-trip-b", Origin: "https://github.com/user/b.git", Branch: "develop"},
	}
	for _, r := range repos {
		err := s1.Add(r)
		require.NoError(t, err)
	}

	// Create a new store and load from the same file
	s2 := NewJSONStore(dir)
	err := s2.Load()
	require.NoError(t, err)

	// Verify all repos survived the round-trip
	list, err := s2.List()
	require.NoError(t, err)
	assert.Len(t, list, 2)

	a, ok := s2.GetByName("round-trip-a")
	require.True(t, ok)
	assert.Equal(t, "https://github.com/upstream/a.git", a.Upstream)
	assert.Equal(t, "main", a.Branch)

	b, ok := s2.GetByName("round-trip-b")
	require.True(t, ok)
	assert.Equal(t, "develop", b.Branch)

	// IDs should have been persisted
	assert.NotEmpty(t, a.ID)
	assert.NotEmpty(t, b.ID)
	assert.NotEqual(t, a.ID, b.ID)
}

func TestSaveMethod(t *testing.T) {
	dir := t.TempDir()
	s := NewJSONStore(dir)

	// Use Add to populate, then Save should round-trip correctly
	repo := types.Repo{
		Name:   "manual-repo",
		Origin: "https://github.com/user/manual.git",
	}
	require.NoError(t, s.Add(repo))

	// Save should write to file (already called by Add, but explicit Save should work too)
	err := s.Save()
	require.NoError(t, err)

	// Load into a new store and verify
	s2 := NewJSONStore(dir)
	require.NoError(t, s2.Load())

	got, ok := s2.GetByName("manual-repo")
	require.True(t, ok)
	assert.Equal(t, "https://github.com/user/manual.git", got.Origin)
}

func TestEmptyName(t *testing.T) {
	s := newTestStore(t)
	repo := types.Repo{
		Name: "",
	}
	err := s.Add(repo)
	assert.ErrorIs(t, err, ErrInvalidRepo)
}

func TestStoreInterface(t *testing.T) {
	s := newTestStore(t)
	// Verify JSONStore implements Store
	var store Store = s
	require.NotNil(t, store)

	// Exercise all interface methods
	repo := types.Repo{
		ID:     "iface-id",
		Name:   "iface-repo",
		Origin: "https://github.com/user/iface.git",
	}

	require.NoError(t, store.Add(repo))

	got, ok := store.Get("iface-id")
	assert.True(t, ok)
	assert.Equal(t, "iface-repo", got.Name)

	got, ok = store.GetByName("iface-repo")
	assert.True(t, ok)
	assert.Equal(t, "iface-id", got.ID)

	list, err := store.List()
	require.NoError(t, err)
	assert.Len(t, list, 1)

	repo.Branch = "main"
	require.NoError(t, store.Update(repo))

	require.NoError(t, store.Remove("iface-id"))
	_, ok = store.Get("iface-id")
	assert.False(t, ok)
}
