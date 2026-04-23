package conflict

import (
	"context"
	"testing"
)

func TestGetConflictFiles(t *testing.T) {
	ctx := context.Background()

	files, err := GetConflictFiles(ctx, "/tmp/nonexistent", []string{"a.go", "b.go"})
	if err != nil {
		t.Fatalf("GetConflictFiles: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 files; got %d", len(files))
	}
	if files[0].Path != "a.go" {
		t.Errorf("files[0].Path = %q; want %q", files[0].Path, "a.go")
	}
	if files[1].Path != "b.go" {
		t.Errorf("files[1].Path = %q; want %q", files[1].Path, "b.go")
	}
}

func TestGetConflictFiles_Empty(t *testing.T) {
	ctx := context.Background()

	files, err := GetConflictFiles(ctx, "/tmp", []string{})
	if err != nil {
		t.Fatalf("GetConflictFiles: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("expected 0 files; got %d", len(files))
	}
}

func TestHasConflictMarkers(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    bool
	}{
		{"conflict markers", "<<<<<<< HEAD\nfoo\n=======\nbar\n>>>>>>> upstream", true},
		{"only open marker", "<<<<<<< HEAD\nfoo", false},
		{"no markers", "hello world", false},
		{"empty", "", false},
		{"equals and close only", "=======\n>>>>>>> upstream", false},
		{"markdown false positive", "=======\n\n>>>>>>> quote", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := HasConflictMarkers(tt.content)
			if got != tt.want {
				t.Errorf("HasConflictMarkers() = %v; want %v", got, tt.want)
			}
		})
	}
}

func TestDetectConflicts_NoGit(t *testing.T) {
	// In a directory without git, should return nil (no error)
	ctx := context.Background()
	files := DetectConflicts(ctx, "/tmp/nonexistent-repo-xyz")
	if files != nil {
		t.Errorf("expected nil for nonexistent dir; got %v", files)
	}
}
