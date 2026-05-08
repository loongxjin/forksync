package agent

// OpenCodeAdapter implements AgentProvider for OpenCode CLI.
//
// Invocation: opencode run [--session <id>] <message>
//
// OpenCode CLI uses the "run" subcommand for non-interactive execution.
// There is no autonomous-mode flag — OpenCode executes directly without confirmation in run mode.
type OpenCodeAdapter struct {
	baseAdapter
}

func NewOpenCodeAdapter() *OpenCodeAdapter {
	return &OpenCodeAdapter{baseAdapter{
		binary:    "opencode",
		name:      "opencode",
		buildArgs: OpenCodeBuildArgs,
	}}
}

// OpenCodeBuildArgs constructs the CLI arguments for an OpenCode invocation.
func OpenCodeBuildArgs(sessionID, prompt string) []string {
	args := []string{"run"}
	if sessionID != "" {
		args = append(args, "--session", sessionID)
	}
	args = append(args, prompt)
	return args
}
