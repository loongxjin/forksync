package conflict

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// ---------------------------------------------------------------------------
// parseConflictSections
// ---------------------------------------------------------------------------

func TestParseConflictSections(t *testing.T) {
	tests := []struct {
		name           string
		content        string
		wantOurs       string
		wantTheirs     string
	}{
		{
			name: "standard single conflict",
			content: `line before
<<<<<<< HEAD
our changes
our second line
=======
their changes
>>>>>>> branch-a
line after`,
			wantOurs:   "our changes\nour second line",
			wantTheirs: "their changes",
		},
		{
			name: "multiple conflict sections",
			content: `first section
<<<<<<< HEAD
ours1
=======
theirs1
>>>>>>> branch-a
middle content
<<<<<<< HEAD
ours2
=======
theirs2
>>>>>>> branch-b
last content`,
			wantOurs:   "ours1\nours2",
			wantTheirs: "theirs1\ntheirs2",
		},
		{
			name: "no conflict markers",
			content: `just regular content
nothing to see here
no markers at all`,
			wantOurs:   "",
			wantTheirs: "",
		},
		{
			name:       "empty content",
			content:    "",
			wantOurs:   "",
			wantTheirs: "",
		},
		{
			name: "empty ours and theirs sections",
			content: `before
<<<<<<< HEAD
=======
>>>>>>> branch-a
after`,
			wantOurs:   "",
			wantTheirs: "",
		},
		{
			name: "only theirs has content",
			content: `<<<<<<< HEAD
=======
their content here
>>>>>>> branch-a`,
			wantOurs:   "",
			wantTheirs: "their content here",
		},
		{
			name: "only ours has content",
			content: `<<<<<<< HEAD
our content here
=======
>>>>>>> branch-a`,
			wantOurs:   "our content here",
			wantTheirs: "",
		},
		{
			name: "markers with branch labels",
			content: `<<<<<<< HEAD feature-branch
our line
======= merge-base
their line
>>>>>>> refs/heads/develop`,
			wantOurs:   "our line",
			wantTheirs: "their line",
		},
		{
			name: "multiline content in each section",
			content: `<<<<<<< HEAD
line1
line2
line3
=======
lineA
lineB
>>>>>>> other`,
			wantOurs:   "line1\nline2\nline3",
			wantTheirs: "lineA\nlineB",
		},
		{
			name: "content with equals signs but not a separator",
			content: `<<<<<<< HEAD
value === something
=======
other === value
>>>>>>> branch`,
			wantOurs:   "value === something",
			wantTheirs: "other === value",
		},
		{
			name:        "single line without newline",
			content:     "<<<<<<< HEAD\nonly ours\n=======\nonly theirs\n>>>>>>> branch",
			wantOurs:   "only ours",
			wantTheirs: "only theirs",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotOurs, gotTheirs := parseConflictSections(tt.content)
			if gotOurs != tt.wantOurs {
				t.Errorf("parseConflictSections() ours = %q, want %q", gotOurs, tt.wantOurs)
			}
			if gotTheirs != tt.wantTheirs {
				t.Errorf("parseConflictSections() theirs = %q, want %q", gotTheirs, tt.wantTheirs)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// HasConflictMarkers
// ---------------------------------------------------------------------------

func TestHasConflictMarkers(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    bool
	}{
		{
			name: "full conflict markers present",
			content: `some code
<<<<<<< HEAD
ours
=======
theirs
>>>>>>> branch
more code`,
			want: true,
		},
		{
			name:    "only start marker",
			content: "<<<<<<< HEAD\nsome content",
			want:    true,
		},
		{
			name:    "only separator and end markers",
			content: "some content\n=======\n>>>>>>> branch",
			want:    true,
		},
		{
			name:    "no markers at all",
			content: "just regular content\nno conflict markers",
			want:    false,
		},
		{
			name:    "empty content",
			content: "",
			want:    false,
		},
		{
			name:    "only separator without end marker",
			content: "some content\n=======\nmore content",
			want:    false,
		},
		{
			name:    "only end marker without separator",
			content: "some content\n>>>>>>> branch",
			want:    false,
		},
		{
			name:    "separator and end marker present",
			content: "some content\n=======\n>>>>>>> branch",
			want:    true,
		},
		{
			name:    "start marker only is enough",
			content: "<<<<<<< HEAD",
			want:    true,
		},
		{
			name:    "similar but not exact markers - seven less than",
			content: "<<<<<< some content",
			want:    false,
		},
		{
			name:    "similar but not exact markers - six equals",
			content: "====== content",
			want:    false,
		},
		{
			name:    "similar but not exact markers - six greater than",
			content: ">>>>>> content",
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := HasConflictMarkers(tt.content)
			if got != tt.want {
				t.Errorf("HasConflictMarkers() = %v, want %v", got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// GetConflictFiles (integration test with temp directories)
// ---------------------------------------------------------------------------

func TestGetConflictFiles(t *testing.T) {
	t.Run("reads conflicted file content from disk", func(t *testing.T) {
		// Set up a temp directory with a conflicted file.
		tmpDir := t.TempDir()

		conflictContent := `<<<<<<< HEAD
our version of the file
=======
their version of the file
>>>>>>> upstream/main`
		conflictPath := filepath.Join(tmpDir, "main.go")
		if err := os.WriteFile(conflictPath, []byte(conflictContent), 0644); err != nil {
			t.Fatalf("failed to write conflict file: %v", err)
		}

		d := NewDetector()
		files, err := d.GetConflictFiles(context.Background(), tmpDir, []string{"main.go"})
		if err != nil {
			t.Fatalf("GetConflictFiles() error = %v", err)
		}
		if len(files) != 1 {
			t.Fatalf("GetConflictFiles() returned %d files, want 1", len(files))
		}

		cf := files[0]
		if cf.Path != "main.go" {
			t.Errorf("Path = %q, want %q", cf.Path, "main.go")
		}
		if cf.OursContent != "our version of the file" {
			t.Errorf("OursContent = %q, want %q", cf.OursContent, "our version of the file")
		}
		if cf.TheirsContent != "their version of the file" {
			t.Errorf("TheirsContent = %q, want %q", cf.TheirsContent, "their version of the file")
		}
	})

	t.Run("multiple conflicted files", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create a subdirectory for one of the files.
		subDir := filepath.Join(tmpDir, "pkg")
		if err := os.MkdirAll(subDir, 0755); err != nil {
			t.Fatalf("failed to create subdirectory: %v", err)
		}

		content1 := `<<<<<<< HEAD
ours-a
=======
theirs-a
>>>>>>> branch`
		content2 := `<<<<<<< HEAD
ours-b
=======
theirs-b
>>>>>>> branch`
		if err := os.WriteFile(filepath.Join(tmpDir, "file1.txt"), []byte(content1), 0644); err != nil {
			t.Fatalf("failed to write file1: %v", err)
		}
		if err := os.WriteFile(filepath.Join(subDir, "file2.txt"), []byte(content2), 0644); err != nil {
			t.Fatalf("failed to write file2: %v", err)
		}

		d := NewDetector()
		files, err := d.GetConflictFiles(context.Background(), tmpDir, []string{"file1.txt", "pkg/file2.txt"})
		if err != nil {
			t.Fatalf("GetConflictFiles() error = %v", err)
		}
		if len(files) != 2 {
			t.Fatalf("GetConflictFiles() returned %d files, want 2", len(files))
		}

		if files[0].OursContent != "ours-a" {
			t.Errorf("files[0] OursContent = %q, want %q", files[0].OursContent, "ours-a")
		}
		if files[0].TheirsContent != "theirs-a" {
			t.Errorf("files[0] TheirsContent = %q, want %q", files[0].TheirsContent, "theirs-a")
		}
		if files[1].OursContent != "ours-b" {
			t.Errorf("files[1] OursContent = %q, want %q", files[1].OursContent, "ours-b")
		}
		if files[1].TheirsContent != "theirs-b" {
			t.Errorf("files[1] TheirsContent = %q, want %q", files[1].TheirsContent, "theirs-b")
		}
	})

	t.Run("empty conflict paths returns nil slice", func(t *testing.T) {
		tmpDir := t.TempDir()
		d := NewDetector()
		files, err := d.GetConflictFiles(context.Background(), tmpDir, []string{})
		if err != nil {
			t.Fatalf("GetConflictFiles() error = %v", err)
		}
		if len(files) != 0 {
			t.Errorf("GetConflictFiles() returned %d files, want 0", len(files))
		}
	})

	t.Run("nil conflict paths returns nil slice", func(t *testing.T) {
		tmpDir := t.TempDir()
		d := NewDetector()
		files, err := d.GetConflictFiles(context.Background(), tmpDir, nil)
		if err != nil {
			t.Fatalf("GetConflictFiles() error = %v", err)
		}
		if len(files) != 0 {
			t.Errorf("GetConflictFiles() returned %d files, want 0", len(files))
		}
	})

	t.Run("missing file falls back to empty content", func(t *testing.T) {
		tmpDir := t.TempDir()
		d := NewDetector()
		files, err := d.GetConflictFiles(context.Background(), tmpDir, []string{"nonexistent.txt"})
		if err != nil {
			t.Fatalf("GetConflictFiles() error = %v", err)
		}
		if len(files) != 1 {
			t.Fatalf("GetConflictFiles() returned %d files, want 1", len(files))
		}
		if files[0].OursContent != "" {
			t.Errorf("OursContent = %q, want empty string for missing file", files[0].OursContent)
		}
		if files[0].TheirsContent != "" {
			t.Errorf("TheirsContent = %q, want empty string for missing file", files[0].TheirsContent)
		}
	})

	t.Run("file without conflict markers produces empty ours and theirs", func(t *testing.T) {
		tmpDir := t.TempDir()
		plainContent := "just a regular file\nno conflict markers here"
		if err := os.WriteFile(filepath.Join(tmpDir, "plain.txt"), []byte(plainContent), 0644); err != nil {
			t.Fatalf("failed to write plain file: %v", err)
		}

		d := NewDetector()
		files, err := d.GetConflictFiles(context.Background(), tmpDir, []string{"plain.txt"})
		if err != nil {
			t.Fatalf("GetConflictFiles() error = %v", err)
		}
		if len(files) != 1 {
			t.Fatalf("GetConflictFiles() returned %d files, want 1", len(files))
		}
		if files[0].OursContent != "" {
			t.Errorf("OursContent = %q, want empty for non-conflict file", files[0].OursContent)
		}
		if files[0].TheirsContent != "" {
			t.Errorf("TheirsContent = %q, want empty for non-conflict file", files[0].TheirsContent)
		}
	})
}

// ---------------------------------------------------------------------------
// NewDetector
// ---------------------------------------------------------------------------

func TestNewDetector(t *testing.T) {
	d := NewDetector()
	if d == nil {
		t.Fatal("NewDetector() returned nil, want non-nil Detector")
	}
	if d.gitOps == nil {
		t.Error("NewDetector() gitOps is nil, want non-nil")
	}
}
