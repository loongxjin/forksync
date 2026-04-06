package conflict

import (
	"context"
	"strings"

	"github.com/loongxjin/forksync/engine/internal/git"
	"github.com/loongxjin/forksync/engine/pkg/types"
)

// Detector handles conflict detection and file content extraction.
type Detector struct {
	gitOps *git.Operations
}

// NewDetector creates a new conflict Detector.
func NewDetector() *Detector {
	return &Detector{gitOps: git.NewOperations()}
}

// GetConflictFiles extracts conflict file information from a repo.
func (d *Detector) GetConflictFiles(ctx context.Context, repoPath string, conflictPaths []string) ([]types.ConflictFile, error) {
	var files []types.ConflictFile
	for _, path := range conflictPaths {
		cf := types.ConflictFile{Path: path}

		// Get the conflicted content (with markers)
		content, err := d.gitOps.GetConflictedContent(ctx, repoPath, path)
		if err != nil {
			cf.OursContent = "" // fallback
		} else {
			// Parse ours/theirs from conflict markers
			ours, theirs := parseConflictSections(content)
			cf.OursContent = ours
			cf.TheirsContent = theirs
		}

		files = append(files, cf)
	}
	return files, nil
}

// parseConflictSections extracts ours and theirs sections from conflict markers.
func parseConflictSections(content string) (ours, theirs string) {
	var oursParts, theirsParts []string
	inOurs := false
	inTheirs := false

	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(line, "<<<<<<<") {
			inOurs = true
			continue
		}
		if strings.HasPrefix(line, "=======") {
			inOurs = false
			inTheirs = true
			continue
		}
		if strings.HasPrefix(line, ">>>>>>>") {
			inTheirs = false
			continue
		}
		if inOurs {
			oursParts = append(oursParts, line)
		}
		if inTheirs {
			theirsParts = append(theirsParts, line)
		}
	}

	return strings.Join(oursParts, "\n"), strings.Join(theirsParts, "\n")
}

// HasConflictMarkers checks if content contains unresolved conflict markers.
func HasConflictMarkers(content string) bool {
	return strings.Contains(content, "<<<<<<<") ||
		(strings.Contains(content, "=======") && strings.Contains(content, ">>>>>>>"))
}
