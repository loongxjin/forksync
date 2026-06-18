package agent

// CodexAdapter implements AgentProvider for Codex CLI (OpenAI).
//
// Invocation:
//   - New session:  codex --dangerously-bypass-approvals-and-sandbox <prompt>
//   - Resume:       codex resume --last --dangerously-bypass-approvals-and-sandbox <prompt>
//
// Codex CLI uses "resume --last" as a subcommand to continue the last session.
type CodexAdapter struct {
	baseAdapter
}

func NewCodexAdapter() *CodexAdapter {
	return &CodexAdapter{baseAdapter{
		binary:    "codex",
		name:      "codex",
		buildArgs: CodexBuildArgs,
	}}
}

// CodexBuildArgs constructs the CLI arguments for a Codex invocation.
// Uses "codex exec" for non-interactive execution.
// sessionID is non-empty when resuming an existing session.
//
// TODO: Codex CLI currently only supports "resume --last" which resumes the
// most recent session, not a specific session ID. This can be unreliable when
// multiple sessions are active simultaneously.
func CodexBuildArgs(sessionID, prompt string) []string {
	args := []string{"exec"}
	if sessionID != "" {
		_ = sessionID // reserved for future use when Codex CLI supports targeting a specific session
		args = append(args, "resume", "--last")
	}
	args = append(args, "--dangerously-bypass-approvals-and-sandbox", prompt)
	return args
}
