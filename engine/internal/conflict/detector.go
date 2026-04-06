package conflict

import (
	"context"
	"os/exec"
	"strings"

	"github.com/loongxjin/forksync/engine/pkg/types"
)

// Detector handles conflict detection.
type Detector struct{}

// NewDetector creates a new conflict Detector.
func NewDetector() *Detector {
	return &Detector{}
}

// GetConflictFiles returns a list of conflict file paths from a repo.
// Unlike the previous version, this only returns file paths — the agent
// reads file contents directly from disk.
func (d *Detector) GetConflictFiles(ctx context.Context, repoPath string, conflictPaths []string) ([]types.ConflictFile, error) {
	files := make([]types.ConflictFile, 0, len(conflictPaths))
	for _, path := range conflictPaths {
		files = append(files, types.ConflictFile{Path: path})
	}
	return files, nil
}

// HasConflictMarkers checks if content contains unresolved conflict markers.
func HasConflictMarkers(content string) bool {
	return strings.Contains(content, "=======") && strings.Contains(content, ">>>>>>>")
}

// DetectConflicts runs git diff to find files with unresolved conflicts.
func DetectConflicts(ctx context.Context, repoPath string) []string {
	cmd := exec.CommandContext(ctx, "git", "diff", "--name-only", "--diff-filter=U")
	cmd.Dir = repoPath
	output, err := cmd.Output()
	if err != nil {
		return nil
	}
	var files []string
	for _, f := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if f != "" {
			files = append(files, f)
		}
	}
	return files
}
