package ai

import (
	"context"
)

// ConflictRequest is the input for AI conflict resolution.
type ConflictRequest struct {
	FilePath        string
	ConflictContent string
	UserDiff        string
	Language        string
}

// ConflictResolution is the output of AI conflict resolution.
type ConflictResolution struct {
	MergedContent string
	Explanation   string
	NeedsReview   bool
}

// Provider defines the interface for AI providers.
type Provider interface {
	ResolveConflicts(ctx context.Context, req ConflictRequest) (*ConflictResolution, error)
}
