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

	return nil, fmt.Errorf("no agent CLI found; install Claude Code, OpenCode, or Codex")
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

// ResolveProvider picks a single agent provider for a resolve run. When
// requested is non-empty, that named agent is returned (regardless of the
// preferred setting, matching the explicit --agent flag). Otherwise the
// preferred (or first available) agent is returned.
//
// preferred is the user's configured preferred agent name (cfg.Agent.Preferred,
// possibly empty). It is the single source of truth for agent selection shared
// by the interactive resolve path (app) and the auto-sync path (sync).
func ResolveProvider(preferred, requested string) (AgentProvider, error) {
	if requested != "" {
		reg := NewRegistry("")
		provider, err := reg.GetByName(requested)
		if err != nil {
			return nil, fmt.Errorf("agent %q not found: %w", requested, err)
		}
		return provider, nil
	}
	reg := NewRegistry(preferred)
	provider, err := reg.GetPreferred()
	if err != nil {
		return nil, fmt.Errorf("no agent available: %w", err)
	}
	return provider, nil
}
