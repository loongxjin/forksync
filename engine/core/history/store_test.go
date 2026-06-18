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

	_, err := store.Insert(r)
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
		_, err := store.Insert(Record{
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
		_, err := store.Insert(Record{
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

func TestRecord_EmptyConflictFiles(t *testing.T) {
	store := newTestStore(t)

	r := Record{
		RepoID:        "repo-1",
		RepoName:      "myrepo",
		Status:        "up_to_date",
		ConflictFiles: nil,
		CreatedAt:     time.Now(),
	}
	_, err := store.Insert(r)
	require.NoError(t, err)

	records, err := store.Recent(1)
	require.NoError(t, err)
	require.Len(t, records, 1)
	assert.Equal(t, []string(nil), records[0].ConflictFiles)
}

func TestRecord_ReturnsID(t *testing.T) {
	store := newTestStore(t)

	r := Record{
		RepoID:    "repo-1",
		RepoName:  "myrepo",
		Status:    "synced",
		CreatedAt: time.Now(),
	}
	id, err := store.Insert(r)
	require.NoError(t, err)
	assert.Greater(t, id, int64(0))

	// Second record should have a higher ID
	r2 := Record{
		RepoID:    "repo-2",
		RepoName:  "other",
		Status:    "synced",
		CreatedAt: time.Now(),
	}
	id2, err := store.Insert(r2)
	require.NoError(t, err)
	assert.Greater(t, id2, id)
}

func TestUpdateSummary(t *testing.T) {
	store := newTestStore(t)

	r := Record{
		RepoID:    "repo-1",
		RepoName:  "myrepo",
		Status:    "synced",
		CreatedAt: time.Now(),
	}
	id, err := store.Insert(r)
	require.NoError(t, err)

	// Update summary
	err = store.UpdateSummary(id, "This is a test summary", "done")
	require.NoError(t, err)

	// Verify via LatestByRepo (the production read path)
	record, err := store.LatestByRepo("repo-1")
	require.NoError(t, err)
	assert.Equal(t, "This is a test summary", record.Summary)
	assert.Equal(t, "done", record.SummaryStatus)
}

func TestLatestByRepo(t *testing.T) {
	store := newTestStore(t)

	_, err := store.Insert(Record{
		RepoID:    "repo-a",
		RepoName:  "repo-a",
		Status:    "synced",
		CreatedAt: time.Now().Add(-2 * time.Hour),
	})
	require.NoError(t, err)

	_, err = store.Insert(Record{
		RepoID:    "repo-a",
		RepoName:  "repo-a",
		Status:    "conflict",
		CreatedAt: time.Now(),
	})
	require.NoError(t, err)

	record, err := store.LatestByRepo("repo-a")
	require.NoError(t, err)
	assert.Equal(t, "conflict", record.Status)
}

func TestMigration_AddsSummaryColumns(t *testing.T) {
	store := newTestStore(t)

	// Record and query should work with new columns
	r := Record{
		RepoID:        "repo-1",
		RepoName:      "test",
		Status:        "synced",
		Summary:       "test summary",
		SummaryStatus: "done",
		CreatedAt:     time.Now(),
	}
	id, err := store.Insert(r)
	require.NoError(t, err)

	record, err := store.LatestByRepo("repo-1")
	require.NoError(t, err)
	assert.Equal(t, int64(id), record.ID)
	assert.Equal(t, "test summary", record.Summary)
	assert.Equal(t, "done", record.SummaryStatus)
}
