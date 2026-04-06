package agent

import (
	"testing"
)

func TestClaudeAdapter_Name(t *testing.T) {
	a := NewClaudeAdapter()
	if a.Name() != "claude" {
		t.Errorf("Name() = %q; want %q", a.Name(), "claude")
	}
}

func TestOpenCodeAdapter_Name(t *testing.T) {
	a := NewOpenCodeAdapter()
	if a.Name() != "opencode" {
		t.Errorf("Name() = %q; want %q", a.Name(), "opencode")
	}
}

func TestDroidAdapter_Name(t *testing.T) {
	a := NewDroidAdapter()
	if a.Name() != "droid" {
		t.Errorf("Name() = %q; want %q", a.Name(), "droid")
	}
}

func TestCodexAdapter_Name(t *testing.T) {
	a := NewCodexAdapter()
	if a.Name() != "codex" {
		t.Errorf("Name() = %q; want %q", a.Name(), "codex")
	}
}

func TestAllAdapters_Interface(t *testing.T) {
	// Compile-time check that all adapters satisfy AgentProvider
	var _ AgentProvider = NewClaudeAdapter()
	var _ AgentProvider = NewOpenCodeAdapter()
	var _ AgentProvider = NewDroidAdapter()
	var _ AgentProvider = NewCodexAdapter()
}

func TestClaudeAdapter_BuildArgs(t *testing.T) {
	a := &ClaudeAdapter{binary: "claude"}

	tests := []struct {
		name      string
		sessionID string
		prompt    string
		wantFlags []string // check these flags appear in the args
	}{
		{
			name:      "new session",
			sessionID: "",
			prompt:    "resolve conflicts",
			wantFlags: []string{"--print", "--dangerously-skip-permissions"},
		},
		{
			name:      "resume session",
			sessionID: "sess-123",
			prompt:    "resolve more",
			wantFlags: []string{"--print", "--resume", "sess-123", "--dangerously-skip-permissions"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := a.buildArgs(tt.sessionID, tt.prompt)
			for _, flag := range tt.wantFlags {
				found := false
				for _, arg := range args {
					if arg == flag {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected flag %q in args %v", flag, args)
				}
			}
		})
	}
}

func TestOpenCodeAdapter_BuildArgs(t *testing.T) {
	a := &OpenCodeAdapter{binary: "opencode"}

	tests := []struct {
		name      string
		sessionID string
		prompt    string
		wantFlags []string
	}{
		{
			name:      "new session",
			sessionID: "",
			prompt:    "resolve conflicts",
			wantFlags: []string{"--prompt"},
		},
		{
			name:      "resume session",
			sessionID: "sess-456",
			prompt:    "resolve more",
			wantFlags: []string{"--continue", "--prompt"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := a.buildArgs(tt.sessionID, tt.prompt)
			for _, flag := range tt.wantFlags {
				found := false
				for _, arg := range args {
					if arg == flag {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected flag %q in args %v", flag, args)
				}
			}
		})
	}
}

func TestDroidAdapter_BuildArgs(t *testing.T) {
	a := &DroidAdapter{binary: "droid"}

	tests := []struct {
		name      string
		sessionID string
		prompt    string
		wantFlags []string
	}{
		{
			name:      "new session",
			sessionID: "",
			prompt:    "resolve conflicts",
			wantFlags: []string{"--auto", "high"},
		},
		{
			name:      "resume session",
			sessionID: "nonempty",
			prompt:    "resolve more",
			wantFlags: []string{"--resume"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := a.buildArgs(tt.sessionID, tt.prompt)
			for _, flag := range tt.wantFlags {
				found := false
				for _, arg := range args {
					if arg == flag {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected flag %q in args %v", flag, args)
				}
			}
		})
	}
}

func TestCodexAdapter_BuildArgs(t *testing.T) {
	a := &CodexAdapter{binary: "codex"}

	tests := []struct {
		name      string
		sessionID string
		prompt    string
		wantFlags []string
	}{
		{
			name:      "new session",
			sessionID: "",
			prompt:    "resolve conflicts",
			wantFlags: []string{"--dangerously-bypass-approvals-and-sandbox"},
		},
		{
			name:      "resume session",
			sessionID: "nonempty",
			prompt:    "resolve more",
			wantFlags: []string{"resume", "--last"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := a.buildArgs(tt.sessionID, tt.prompt)
			for _, flag := range tt.wantFlags {
				found := false
				for _, arg := range args {
					if arg == flag {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected flag %q in args %v", flag, args)
				}
			}
		})
	}
}

func TestExtractSessionID_Adapters(t *testing.T) {
	tests := []struct {
		name   string
		output string
		wantID string
	}{
		{
			name:   "claude session line",
			output: "Session: sess-abc123\nResolved 2 files",
			wantID: "sess-abc123",
		},
		{
			name:   "empty output",
			output: "",
			wantID: "",
		},
		{
			name:   "no session line",
			output: "just some output",
			wantID: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractSessionID(tt.output)
			if got != tt.wantID {
				t.Errorf("extractSessionID(%q) = %q; want %q", tt.output, got, tt.wantID)
			}
		})
	}
}
