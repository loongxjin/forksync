package history

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	store, err := NewStore(dir)
	require.NoError(t, err)
	t.Cleanup(func() { store.Close() })
	return store
}

func TestNewStore_CreatesDB(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	require.NoError(t, err)
	defer store.Close()

	dbPath := filepath.Join(dir, "db", "forksync.db")
	_, statErr := os.Stat(dbPath)
	assert.NoError(t, statErr, "database file should exist")
}

func TestRecord_AndRecent(t *testing.T) {
	store := newTestStore(t)

	now := time.Now().Truncate(time.Second)
	r := Record{
		RepoID:         "repo-1",
		RepoName:       "myrepo",
		Status:         "synced",
		CommitsPulled:  5,
		ConflictFiles:  []string{"a.go", "b.go"},
		AgentUsed:      "claude",
		ConflictsFound: 2,
		AutoResolved:   2,
		ErrorMessage:   "",
		CreatedAt:      now,
	}

	err := store.Record(r)
	require.NoError(t, err)

	records, err := store.Recent(10)
	require.NoError(t, err)
	require.Len(t, records, 1)

	got := records[0]
	assert.Equal(t, "repo-1", got.RepoID)
	assert.Equal(t, "myrepo", got.RepoName)
	assert.Equal(t, "synced", got.Status)
	assert.Equal(t, 5, got.CommitsPulled)
	assert.Equal(t, []string{"a.go", "b.go"}, got.ConflictFiles)
	assert.Equal(t, "claude", got.AgentUsed)
	assert.Equal(t, 2, got.ConflictsFound)
	assert.Equal(t, 2, got.AutoResolved)
}

func TestRecent_Limit(t *testing.T) {
	store := newTestStore(t)

	for i := 0; i < 5; i++ {
		err := store.Record(Record{
			RepoID:    "repo-1",
			RepoName:  "myrepo",
			Status:    "synced",
			CreatedAt: time.Now().Add(time.Duration(i) * time.Second),
		})
		require.NoError(t, err)
	}

	records, err := store.Recent(3)
	require.NoError(t, err)
	assert.Len(t, records, 3)
	// Most recent first
	assert.Equal(t, int64(5), records[0].ID)
}

func TestByRepo(t *testing.T) {
	store := newTestStore(t)

	for _, repoID := range []string{"repo-a", "repo-b", "repo-a"} {
		err := store.Record(Record{
			RepoID:    repoID,
			RepoName:  repoID,
			Status:    "synced",
			CreatedAt: time.Now(),
		})
		require.NoError(t, err)
	}

	records, err := store.ByRepo("repo-a", 10)
	require.NoError(t, err)
	assert.Len(t, records, 2)

	records, err = store.ByRepo("repo-b", 10)
	require.NoError(t, err)
	assert.Len(t, records, 1)
}

func TestSummary(t *testing.T) {
	store := newTestStore(t)

	records := []Record{
		{RepoID: "r1", RepoName: "r1", Status: "synced", CreatedAt: time.Now()},
		{RepoID: "r2", RepoName: "r2", Status: "conflict", ConflictFiles: []string{"f1"}, CreatedAt: time.Now()},
		{RepoID: "r3", RepoName: "r3", Status: "error", ErrorMessage: "fetch failed", CreatedAt: time.Now()},
		{RepoID: "r4", RepoName: "r4", Status: "synced", CreatedAt: time.Now()},
	}
	for _, r := range records {
		require.NoError(t, store.Record(r))
	}

	totalSyncs, conflicts, errors, _, err := store.Summary()
	require.NoError(t, err)
	assert.Equal(t, 4, totalSyncs)
	assert.Equal(t, 1, conflicts)
	assert.Equal(t, 1, errors)
}

func TestRecord_EmptyConflictFiles(t *testing.T) {
	store := newTestStore(t)

	r := Record{
		RepoID:        "repo-1",
		RepoName:      "myrepo",
		Status:        "up_to_date",
		ConflictFiles: nil,
		CreatedAt:     time.Now(),
	}
	require.NoError(t, store.Record(r))

	records, err := store.Recent(1)
	require.NoError(t, err)
	require.Len(t, records, 1)
	assert.Equal(t, []string(nil), records[0].ConflictFiles)
}
