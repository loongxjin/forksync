package logger

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew_CreatesDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "logs")
	_, err := New(dir)
	require.NoError(t, err)

	stat, statErr := os.Stat(dir)
	assert.NoError(t, statErr)
	assert.True(t, stat.IsDir())
}

func TestInfo_WritesToDailyFile(t *testing.T) {
	dir := t.TempDir()
	log, err := New(dir)
	require.NoError(t, err)
	defer log.Close()

	log.Info("sync completed for %s", "myrepo")

	// Check the file was created with today's date
	expectedFile := filepath.Join(dir, "sync-"+time.Now().Format("2006-01-02")+".log")
	data, readErr := os.ReadFile(expectedFile)
	require.NoError(t, readErr)

	content := string(data)
	assert.Contains(t, content, "INFO")
	assert.Contains(t, content, "sync completed for myrepo")
}

func TestError_WritesToDailyFile(t *testing.T) {
	dir := t.TempDir()
	log, err := New(dir)
	require.NoError(t, err)
	defer log.Close()

	log.Error("fetch failed: %v", "connection refused")

	expectedFile := filepath.Join(dir, "sync-"+time.Now().Format("2006-01-02")+".log")
	data, readErr := os.ReadFile(expectedFile)
	require.NoError(t, readErr)

	content := string(data)
	assert.Contains(t, content, "ERROR")
	assert.Contains(t, content, "fetch failed: connection refused")
}

func TestAppend_MultipleWrites(t *testing.T) {
	dir := t.TempDir()
	log, err := New(dir)
	require.NoError(t, err)
	defer log.Close()

	log.Info("line 1")
	log.Info("line 2")
	log.Error("line 3")

	expectedFile := filepath.Join(dir, "sync-"+time.Now().Format("2006-01-02")+".log")
	data, readErr := os.ReadFile(expectedFile)
	require.NoError(t, readErr)

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	assert.Len(t, lines, 3)
	assert.Contains(t, lines[0], "INFO")
	assert.Contains(t, lines[1], "INFO")
	assert.Contains(t, lines[2], "ERROR")
}

func TestClose_ReleasesFile(t *testing.T) {
	dir := t.TempDir()
	log, err := New(dir)
	require.NoError(t, err)

	log.Info("test")
	require.NoError(t, log.Close())

	// File should be released, can write again after close
	log2, err2 := New(dir)
	require.NoError(t, err2)
	log2.Info("after close")
	require.NoError(t, log2.Close())
}
