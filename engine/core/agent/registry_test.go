package agent

import (
	"testing"
)

func TestNewRegistry(t *testing.T) {
	r := NewRegistry("")
	if r == nil {
		t.Fatal("NewRegistry should return non-nil")
	}
	if len(r.providers) == 0 {
		t.Error("registry should have providers registered")
	}
}

func TestRegistryDiscover(t *testing.T) {
	r := NewRegistry("")
	agents := r.Discover()

	// Should return a list (may be empty if no agents installed, but not nil)
	if agents == nil {
		t.Error("Discover should not return nil")
	}
}

func TestRegistryDiscoverWithMocks(t *testing.T) {
	mockInstalled := &MockProvider{
		NameFunc:        func() string { return "mock-installed" },
		IsAvailableFunc: func() bool { return true },
	}
	mockNotInstalled := &MockProvider{
		NameFunc:        func() string { return "mock-unavailable" },
		IsAvailableFunc: func() bool { return false },
	}

	r := &Registry{
		providers: []AgentProvider{mockInstalled, mockNotInstalled},
		preferred: "",
	}

	agents := r.Discover()
	if len(agents) != 1 {
		t.Fatalf("Discover should find 1 agent; got %d", len(agents))
	}
	if agents[0].Name != "mock-installed" {
		t.Errorf("found agent = %q; want %q", agents[0].Name, "mock-installed")
	}
}

func TestRegistryGetPreferred_Explicit(t *testing.T) {
	mock1 := &MockProvider{
		NameFunc:        func() string { return "alpha" },
		IsAvailableFunc: func() bool { return true },
	}
	mock2 := &MockProvider{
		NameFunc:        func() string { return "beta" },
		IsAvailableFunc: func() bool { return true },
	}

	r := &Registry{
		providers: []AgentProvider{mock1, mock2},
		preferred: "beta",
	}

	p, err := r.GetPreferred()
	if err != nil {
		t.Fatalf("GetPreferred: %v", err)
	}
	if p.Name() != "beta" {
		t.Errorf("preferred = %q; want %q", p.Name(), "beta")
	}
}

func TestRegistryGetPreferred_FirstAvailable(t *testing.T) {
	mock1 := &MockProvider{
		NameFunc:        func() string { return "alpha" },
		IsAvailableFunc: func() bool { return false },
	}
	mock2 := &MockProvider{
		NameFunc:        func() string { return "beta" },
		IsAvailableFunc: func() bool { return true },
	}

	r := &Registry{
		providers: []AgentProvider{mock1, mock2},
		preferred: "",
	}

	p, err := r.GetPreferred()
	if err != nil {
		t.Fatalf("GetPreferred: %v", err)
	}
	if p.Name() != "beta" {
		t.Errorf("preferred = %q; want %q", p.Name(), "beta")
	}
}

func TestRegistryGetPreferred_NoneAvailable(t *testing.T) {
	mock1 := &MockProvider{
		NameFunc:        func() string { return "alpha" },
		IsAvailableFunc: func() bool { return false },
	}

	r := &Registry{
		providers: []AgentProvider{mock1},
		preferred: "",
	}

	_, err := r.GetPreferred()
	if err == nil {
		t.Error("GetPreferred should return error when no agents available")
	}
}

func TestRegistryGetPreferred_NotInstalled(t *testing.T) {
	mock1 := &MockProvider{
		NameFunc:        func() string { return "alpha" },
		IsAvailableFunc: func() bool { return true },
	}

	r := &Registry{
		providers: []AgentProvider{mock1},
		preferred: "nonexistent",
	}

	// Preferred not found, should fall back to first available
	p, err := r.GetPreferred()
	if err != nil {
		t.Fatalf("GetPreferred: %v", err)
	}
	if p.Name() != "alpha" {
		t.Errorf("fallback = %q; want %q", p.Name(), "alpha")
	}
}

func TestRegistryGetByName(t *testing.T) {
	mock1 := &MockProvider{
		NameFunc:        func() string { return "alpha" },
		IsAvailableFunc: func() bool { return true },
	}
	mock2 := &MockProvider{
		NameFunc:        func() string { return "beta" },
		IsAvailableFunc: func() bool { return true },
	}

	r := &Registry{
		providers: []AgentProvider{mock1, mock2},
		preferred: "",
	}

	p, err := r.GetByName("beta")
	if err != nil {
		t.Fatalf("GetByName: %v", err)
	}
	if p.Name() != "beta" {
		t.Errorf("GetByName = %q; want %q", p.Name(), "beta")
	}
}

func TestRegistryGetByName_NotFound(t *testing.T) {
	r := NewRegistry("")
	_, err := r.GetByName("nonexistent")
	if err == nil {
		t.Error("GetByName should return error for unknown agent")
	}
}

func TestRegistryListAll(t *testing.T) {
	mock1 := &MockProvider{
		NameFunc:        func() string { return "alpha" },
		IsAvailableFunc: func() bool { return true },
	}
	mock2 := &MockProvider{
		NameFunc:        func() string { return "beta" },
		IsAvailableFunc: func() bool { return false },
	}

	r := &Registry{
		providers: []AgentProvider{mock1, mock2},
		preferred: "alpha",
	}

	all := r.ListAll()
	if len(all) != 2 {
		t.Fatalf("ListAll should return 2; got %d", len(all))
	}
	// First should be installed
	if !all[0].Installed {
		t.Error("alpha should be installed")
	}
	// Second should not be installed
	if all[1].Installed {
		t.Error("beta should not be installed")
	}
}
