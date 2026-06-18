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

func TestInit_CreatesDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "logs")
	err := Init(dir)
	require.NoError(t, err)
	defer Close()

	stat, statErr := os.Stat(dir)
	assert.NoError(t, statErr)
	assert.True(t, stat.IsDir())
}

func TestInfo_WritesToDailyFile(t *testing.T) {
	dir := t.TempDir()
	err := Init(dir)
	require.NoError(t, err)
	defer Close()

	Info("sync completed", "repo", "myrepo")

	// Check the file was created with today's date
	expectedFile := filepath.Join(dir, "sync-"+time.Now().Format("2006-01-02")+".log")
	data, readErr := os.ReadFile(expectedFile)
	require.NoError(t, readErr)

	content := string(data)
	assert.Contains(t, content, "INFO")
	assert.Contains(t, content, "sync completed")
	assert.Contains(t, content, "myrepo")
}

func TestError_WritesToDailyFile(t *testing.T) {
	dir := t.TempDir()
	err := Init(dir)
	require.NoError(t, err)
	defer Close()

	Error("fetch failed", "error", "connection refused")

	expectedFile := filepath.Join(dir, "sync-"+time.Now().Format("2006-01-02")+".log")
	data, readErr := os.ReadFile(expectedFile)
	require.NoError(t, readErr)

	content := string(data)
	assert.Contains(t, content, "ERROR")
	assert.Contains(t, content, "fetch failed")
	assert.Contains(t, content, "connection refused")
}

func TestAppend_MultipleWrites(t *testing.T) {
	dir := t.TempDir()
	err := Init(dir)
	require.NoError(t, err)
	defer Close()

	Info("line 1")
	Info("line 2")
	Error("line 3")

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
	err := Init(dir)
	require.NoError(t, err)

	Info("test")
	require.NoError(t, Close())

	// File should be released, can write again after close
	err2 := Init(dir)
	require.NoError(t, err2)
	Info("after close")
	require.NoError(t, Close())
}
