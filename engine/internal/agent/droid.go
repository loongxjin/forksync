package agent

// DroidAdapter implements AgentProvider for Droid CLI (Factory).
//
// Invocation: droid exec --auto high [--session-id <id>] <prompt>
//
// Droid CLI uses the "exec" subcommand for non-interactive execution.
// The --auto flag controls autonomy level: low, medium, high.
// For conflict resolution, "high" is needed to allow file modifications and git operations.
type DroidAdapter struct {
	baseAdapter
}

func NewDroidAdapter() *DroidAdapter {
	return &DroidAdapter{baseAdapter{
		binary:    "droid",
		name:      "droid",
		buildArgs: DroidBuildArgs,
	}}
}

// DroidBuildArgs constructs the CLI arguments for a Droid exec invocation.
func DroidBuildArgs(sessionID, prompt string) []string {
	args := []string{"exec", "--auto", "high"}
	if sessionID != "" {
		args = append(args, "--session-id", sessionID)
	}
	args = append(args, prompt)
	return args
}
