package agent

import "fmt"

const defaultSystemBody = `You are a helpful terminal assistant. You complete tasks by running bash commands.
Think step by step. When the task is done, respond with a clear summary of what you accomplished.
Do not ask for confirmation — just proceed with the task.`

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
