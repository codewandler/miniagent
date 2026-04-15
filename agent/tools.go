package agent

import (
	"context"
	"fmt"
	"os/exec"
	"syscall"
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
//
// Two problems the original implementation had:
//
//  1. Hanging on timeout: exec.CommandContext only kills the bash process, but
//     any children it spawned keep the stdout/stderr pipe FDs open. CombinedOutput
//     waits for all pipe writers to close, so it blocks indefinitely even after
//     the bash process is dead.
//
//     Fix: Setpgid puts bash and all its children in a fresh process group.
//     cmd.Cancel kills the whole group with SIGKILL. cmd.WaitDelay gives the
//     (now-dead) processes 2 s to release their FDs before Wait gives up and
//     returns regardless.
//
//  2. Swallowed parent-context cancellation: if the agent's context is cancelled
//     (user interrupt, max-steps, etc.) the original code returned a BashResult
//     instead of an error, so the agent loop never saw the cancellation and kept
//     running.
//
//     Fix: check ctx.Err() before anything else and return it as an actual error.
func NewBashHandler(workspace string, timeout time.Duration) tool.NamedHandler {
	return tool.NewHandler[BashParams, BashResult]("bash",
		func(ctx context.Context, in BashParams) (*BashResult, error) {
			cmdCtx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()

			cmd := exec.CommandContext(cmdCtx, "bash", "-c", in.Command)
			cmd.Dir = workspace

			// Place bash and all processes it forks into a new process group
			// (PGID == bash's PID). This lets us send SIGKILL to every member
			// of the group at once — including long-running children that bash
			// itself may have already exited from.
			cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

			// cmd.Cancel is called by exec.CommandContext when cmdCtx expires.
			// Kill the whole process group instead of just the bash process.
			// Negative PID means "kill process group with this PGID".
			cmd.Cancel = func() error {
				if cmd.Process != nil {
					_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
				}
				return nil
			}

			// Hard cap: if processes are still alive and holding pipe FDs open
			// after 2 s, cmd.Wait (and therefore CombinedOutput) returns anyway.
			// Without this, a child that ignores SIGKILL (or outlives it) would
			// make CombinedOutput block forever.
			cmd.WaitDelay = 2 * time.Second

			out, err := cmd.CombinedOutput()
			output := truncateBytes(out, maxOutputBytes)

			// The parent context was cancelled (Ctrl-C, agent shutdown, etc.).
			// Return a real error so the agent loop stops cleanly instead of
			// treating this as a normal tool result and continuing.
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}

			// The per-command timeout fired. Report it as output so the model
			// sees what (if anything) was produced before the cutoff.
			if cmdCtx.Err() == context.DeadlineExceeded {
				msg := fmt.Sprintf("timeout: command exceeded %s", timeout)
				if output != "" {
					msg += "\n" + output
				}
				return &BashResult{Output: msg}, nil
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
