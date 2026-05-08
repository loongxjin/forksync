package sync

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"syscall"

	"github.com/loongxjin/forksync/engine/internal/logger"
)

// locksDir returns the directory for sync lock files.
// Defaults to ~/.forksync/locks/
func locksDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".forksync", "locks")
}

// LockFilePath returns the path to the sync lock file for a repo.
func LockFilePath(repoID string) string {
	return filepath.Join(locksDir(), repoID+".pid")
}

// CreateSyncLock creates a sync lock file containing the current process PID.
// The locks directory is created automatically if it does not exist.
func CreateSyncLock(repoID string) error {
	dir := locksDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create locks dir: %w", err)
	}
	path := LockFilePath(repoID)
	pid := os.Getpid()
	if err := os.WriteFile(path, []byte(strconv.Itoa(pid)), 0644); err != nil {
		return fmt.Errorf("write lock file: %w", err)
	}
	logger.Debug("sync: created lock file", "repo", repoID, "pid", pid)
	return nil
}

// RemoveSyncLock removes the sync lock file for a repo.
func RemoveSyncLock(repoID string) {
	path := LockFilePath(repoID)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		logger.Warn("sync: failed to remove lock file", "repo", repoID, "error", err)
	}
}

// IsSyncProcessAlive checks whether the process recorded in the sync lock file
// is still running. Returns (false, nil) if there is no lock file or the
// recorded process no longer exists.
func IsSyncProcessAlive(repoID string) (bool, error) {
	path := LockFilePath(repoID)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil // no lock file → not running
		}
		return false, fmt.Errorf("read lock file: %w", err)
	}

	pid, err := strconv.Atoi(string(data))
	if err != nil {
		// Corrupted lock file → treat as not running, clean up
		logger.Warn("sync: corrupted lock file, removing", "repo", repoID, "content", string(data))
		RemoveSyncLock(repoID)
		return false, nil
	}

	// Send signal 0 to check if the process exists
	process, err := os.FindProcess(pid)
	if err != nil {
		return false, nil
	}
	if err := process.Signal(syscall.Signal(0)); err != nil {
		// Process does not exist or we lack permission → it's dead
		// Clean up stale lock file
		logger.Debug("sync: stale lock file detected, removing", "repo", repoID, "pid", pid)
		RemoveSyncLock(repoID)
		return false, nil
	}

	return true, nil
}
