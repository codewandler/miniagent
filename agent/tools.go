package agent

import (
	"context"
	"fmt"
	"os/exec"
	"time"

	"github.com/codewandler/llm/tool"
)

const maxOutputBytes = 20 * 1024 // 20 KB

func truncateBytes(b []byte, max int) string {
	if len(b) <= max {
		return string(b)
	}
	return string(b[:max]) + fmt.Sprintf(
		"\n[...truncated, showing first %d of %d bytes]", max, len(b),
	)
}

// BashParams is the typed input for the bash tool.
type BashParams struct {
	Command string `json:"command" jsonschema:"description=Shell command to execute,required"`
}

// BashResult is the typed output returned to the model as JSON.
type BashResult struct {
	Output string `json:"output"`
}

// BashDefinition returns the tool.Definition for the bash tool.
func BashDefinition() tool.Definition {
	return tool.NewSpec[BashParams](
		"bash",
		"Execute a bash command in the workspace directory. Returns combined stdout and stderr. For efficiency, combine multiple steps into a single command using shell operators (&&, ||, ;, pipes, subshells).",
	).Definition()
}

// NewBashHandler creates a NamedHandler that executes bash commands
// in the given workspace with a per-command timeout.
func NewBashHandler(workspace string, timeout time.Duration) tool.NamedHandler {
	return tool.NewHandler[BashParams, BashResult]("bash",
		func(ctx context.Context, in BashParams) (*BashResult, error) {
			cmdCtx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()

			cmd := exec.CommandContext(cmdCtx, "bash", "-c", in.Command)
			cmd.Dir = workspace
			out, err := cmd.CombinedOutput()
			output := truncateBytes(out, maxOutputBytes)

			if cmdCtx.Err() == context.DeadlineExceeded {
				return &BashResult{
					Output: fmt.Sprintf("timeout: command exceeded %s", timeout),
				}, nil
			}
			if err != nil {
				if exitErr, ok := err.(*exec.ExitError); ok {
					return &BashResult{
						Output: fmt.Sprintf("exit %d:\n%s", exitErr.ExitCode(), output),
					}, nil
				}
				return &BashResult{Output: fmt.Sprintf("error: %s", err)}, nil
			}
			return &BashResult{Output: output}, nil
		},
	)
}
