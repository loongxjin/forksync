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

	tests := []struct {
		name     string
		new      bool
		prompt   string
		wantArgs []string
	}{
		{
			name:     "new session",
			new:      true,
			prompt:   "resolve conflicts",
			wantArgs: []string{"--print", "--dangerously-skip-permissions", "--output-format", "json", "resolve conflicts"},
		},
		{
			name:     "resume session",
			new:      false,
			prompt:   "resolve more",
			wantArgs: []string{"--print", "--dangerously-skip-permissions", "--output-format", "json", "--resume", "sess-123", "resolve more"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var args []string
			if tt.new {
				args = []string{
					"--print",
					"--dangerously-skip-permissions",
					"--output-format", "json",
					tt.prompt,
				}
			} else {
				args = []string{
					"--print",
					"--dangerously-skip-permissions",
					"--output-format", "json",
					"--resume", "sess-123",
					tt.prompt,
				}
			}
			// Verify the args match expected
			if len(args) != len(tt.wantArgs) {
				t.Fatalf("args = %v; want %v", args, tt.wantArgs)
			}
			for i, got := range args {
				if got != tt.wantArgs[i] {
					t.Errorf("args[%d] = %q; want %q", i, got, tt.wantArgs[i])
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
		wantArgs  []string
	}{
		{
			name:      "new session",
			sessionID: "",
			prompt:    "resolve conflicts",
			wantArgs:  []string{"run", "resolve conflicts"},
		},
		{
			name:      "resume session",
			sessionID: "sess-456",
			prompt:    "resolve more",
			wantArgs:  []string{"run", "--session", "sess-456", "resolve more"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := a.buildArgs(tt.sessionID, tt.prompt)
			if len(args) != len(tt.wantArgs) {
				t.Fatalf("args = %v; want %v", args, tt.wantArgs)
			}
			for i, got := range args {
				if got != tt.wantArgs[i] {
					t.Errorf("args[%d] = %q; want %q", i, got, tt.wantArgs[i])
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
		wantArgs  []string
	}{
		{
			name:      "new session",
			sessionID: "",
			prompt:    "resolve conflicts",
			wantArgs:  []string{"exec", "--auto", "high", "resolve conflicts"},
		},
		{
			name:      "resume session",
			sessionID: "sess-789",
			prompt:    "resolve more",
			wantArgs:  []string{"exec", "--auto", "high", "--session-id", "sess-789", "resolve more"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := a.buildArgs(tt.sessionID, tt.prompt)
			if len(args) != len(tt.wantArgs) {
				t.Fatalf("args = %v; want %v", args, tt.wantArgs)
			}
			for i, got := range args {
				if got != tt.wantArgs[i] {
					t.Errorf("args[%d] = %q; want %q", i, got, tt.wantArgs[i])
				}
			}
		})
	}
}

func TestCodexAdapter_BuildArgs(t *testing.T) {
	a := &CodexAdapter{binary: "codex"}

	tests := []struct {
		name     string
		resume   bool
		prompt   string
		wantArgs []string
	}{
		{
			name:     "new session",
			resume:   false,
			prompt:   "resolve conflicts",
			wantArgs: []string{"--dangerously-bypass-approvals-and-sandbox", "resolve conflicts"},
		},
		{
			name:     "resume session",
			resume:   true,
			prompt:   "resolve more",
			wantArgs: []string{"resume", "--last", "--dangerously-bypass-approvals-and-sandbox", "resolve more"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := a.buildArgs(tt.resume, tt.prompt)
			if len(args) != len(tt.wantArgs) {
				t.Fatalf("args = %v; want %v", args, tt.wantArgs)
			}
			for i, got := range args {
				if got != tt.wantArgs[i] {
					t.Errorf("args[%d] = %q; want %q", i, got, tt.wantArgs[i])
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
