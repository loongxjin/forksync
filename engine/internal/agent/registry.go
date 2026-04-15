package agent

import (
	"fmt"

	"github.com/loongxjin/forksync/engine/pkg/types"
)

// Registry manages agent providers and handles auto-discovery.
type Registry struct {
	providers []AgentProvider
	preferred string
}

// NewRegistry creates a new agent registry with all supported adapters.
// preferred is the user's preferred agent name (optional, can be empty).
func NewRegistry(preferred string) *Registry {
	return &Registry{
		providers: []AgentProvider{
			NewClaudeAdapter(),
			NewOpenCodeAdapter(),
			NewDroidAdapter(),
			NewCodexAdapter(),
		},
		preferred: preferred,
	}
}

// Discover scans for installed agent CLIs and returns info about each.
func (r *Registry) Discover() []types.AgentInfo {
	found := make([]types.AgentInfo, 0)
	for _, p := range r.providers {
		if p.IsAvailable() {
			found = append(found, types.AgentInfo{
				Name:      p.Name(),
				Installed: true,
			})
		}
	}
	return found
}

// GetPreferred returns the user's preferred agent if available,
// otherwise the first installed agent.
func (r *Registry) GetPreferred() (AgentProvider, error) {
	// Try user's preferred agent first
	if r.preferred != "" {
		for _, p := range r.providers {
			if p.Name() == r.preferred && p.IsAvailable() {
				return p, nil
			}
		}
	}

	// Fall back to first available
	for _, p := range r.providers {
		if p.IsAvailable() {
			return p, nil
		}
	}

	return nil, fmt.Errorf("no agent CLI found; install Claude Code, OpenCode, Droid, or Codex")
}

// GetByName returns a specific agent provider by name.
func (r *Registry) GetByName(name string) (AgentProvider, error) {
	for _, p := range r.providers {
		if p.Name() == name {
			return p, nil
		}
	}
	return nil, fmt.Errorf("agent %q not found", name)
}

// ListAll returns info about all known agents (installed or not).
func (r *Registry) ListAll() []types.AgentInfo {
	all := make([]types.AgentInfo, 0, len(r.providers))
	for _, p := range r.providers {
		all = append(all, types.AgentInfo{
			Name:      p.Name(),
			Installed: p.IsAvailable(),
		})
	}
	return all
}

// Preferred returns the configured preferred agent name.
func (r *Registry) Preferred() string {
	return r.preferred
}
