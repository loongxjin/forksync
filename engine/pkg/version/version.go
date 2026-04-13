package version

// Version is set at build time via -ldflags.
// Example: go build -ldflags "-X github.com/loongxjin/forksync/engine/pkg/version.Version=0.1.0"
var Version = "dev"

// Commit is set at build time via -ldflags.
var Commit = "unknown"

// BuildDate is set at build time via -ldflags.
var BuildDate = "unknown"
