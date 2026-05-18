package workflow

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/loongxjin/forksync/engine/internal/logger"
	"github.com/loongxjin/forksync/engine/pkg/types"
)

const postSyncCommandTimeout = 60 * time.Second

// RunPostSyncCommands executes the repo's post-sync commands in order.
// It stops on the first failure. The sync status remains "up_to_date" regardless.
// Migrated from internal/sync/syncer.go to avoid circular dependency.
func RunPostSyncCommands(ctx context.Context, r types.Repo) []types.PostSyncResult {
	if len(r.PostSyncCommands) == 0 {
		return nil
	}

	var results []types.PostSyncResult
	for _, cmd := range r.PostSyncCommands {
		logger.Info("sync: executing post-sync command", "repo", r.Name, "command", cmd.Name, "cmd", cmd.Cmd)
		cmdCtx, cancel := context.WithTimeout(ctx, postSyncCommandTimeout)
		sh, flag := shell()
		c := exec.CommandContext(cmdCtx, sh, flag, cmd.Cmd)
		c.Dir = r.Path

		var stdout, stderr bytes.Buffer
		c.Stdout = &stdout
		c.Stderr = &stderr

		err := c.Run()
		cancel()

		res := types.PostSyncResult{
			Name: cmd.Name,
			Cmd:  cmd.Cmd,
		}

		if err != nil {
			res.Success = false
			res.Error = strings.TrimSpace(stderr.String())
			if res.Error == "" {
				res.Error = err.Error()
			}
			logger.Error("sync: post-sync command failed", "repo", r.Name, "command", cmd.Name, "error", res.Error)
			results = append(results, res)
			break // stop on first failure
		}

		res.Success = true
		res.Output = strings.TrimSpace(stdout.String())
		logger.Info("sync: post-sync command succeeded", "repo", r.Name, "command", cmd.Name)
		results = append(results, res)
	}

	return results
}

// PostSyncError returns a summary error message if any post-sync command failed.
func PostSyncError(results []types.PostSyncResult) string {
	for _, r := range results {
		if !r.Success {
			return fmt.Sprintf("post-sync command \"%s\" failed: %s", r.Name, r.Error)
		}
	}
	return ""
}

// shell returns the system shell for executing commands.
func shell() (string, string) {
	if runtime.GOOS == "windows" {
		return "cmd", "/c"
	}
	return "sh", "-c"
}
