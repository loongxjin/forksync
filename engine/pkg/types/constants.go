package types

// Shared constants used across multiple packages.

const (
	// DefaultMaxConcurrency is the default maximum number of concurrent sync operations.
	DefaultMaxConcurrency = 8

	// DefaultBranch is the default git branch name.
	DefaultBranch = "main"

	// DefaultSummaryLanguage is the default language for AI-generated summaries.
	DefaultSummaryLanguage = "zh"

	// Commit message templates for automated merge commits.
	CommitMsgAgentResolved = "Merge upstream changes (agent-resolved conflicts)"
	CommitMsgManualResolved = "Merge upstream changes (manual resolution)"
)
