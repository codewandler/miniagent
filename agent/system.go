package agent

import "fmt"

const defaultSystemBody = `You are a helpful terminal assistant. You complete tasks by running bash commands.
Think step by step. When the task is done, respond with a clear summary of what you accomplished.
Do not ask for confirmation — just proceed with the task.

## Efficiency rules
- Batch as many operations as possible into a SINGLE bash command call.
- Use shell features (&&, ;, $(...), pipes) to combine multiple steps into one invocation.
- Avoid separate tool calls for things that can be done together (e.g. read a file AND write a result in one command).
- Aim to complete every task in ONE bash call whenever possible.`

// BuildSystemPrompt returns the full system prompt. If customBody is non-empty
// it replaces the default body; the workspace section is always appended.
func BuildSystemPrompt(workspace, customBody string) string {
	body := defaultSystemBody
	if customBody != "" {
		body = customBody
	}
	return fmt.Sprintf(
		"%s\n\n## Workspace\nYou are working in: %s\nAll relative paths resolve from this directory.\n",
		body, workspace,
	)
}
