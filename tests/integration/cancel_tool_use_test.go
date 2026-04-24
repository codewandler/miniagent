package integration

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/codewandler/miniagent/agent"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCancelDuringToolUse_FollowUpTurnSucceeds hits a real Anthropic backend
// to verify that cancelling a turn mid-tool-execution does not leave orphaned
// tool_use blocks in the conversation history. The original bug manifested as:
//
//	HTTP 400: invalid_request_error: messages.N: `tool_use` ids were found
//	without `tool_result` blocks immediately after: toolu_...
//
// The fix flushes [Canceled] tool_result messages before returning, so the
// next turn's request has a consistent conversation history.
func TestCancelDuringToolUse_FollowUpTurnSucceeds(t *testing.T) {
	if os.Getenv("MINIAGENT_INTEGRATION") == "" {
		t.Skip("set MINIAGENT_INTEGRATION=1 to run integration tests")
	}

	var buf bytes.Buffer
	a := agent.New(
		agent.WithWorkspace(t.TempDir()),
		agent.WithToolTimeout(10*time.Second),
		agent.WithMaxSteps(5),
		agent.WithOutput(&buf),
		agent.WithInferenceOptions(agent.InferenceOptions{
			Model:       "claude/haiku",
			MaxTokens:   4096,
			Temperature: 0.0,
			Thinking:    agent.ThinkingModeOff,
		}),
	)

	// --- Turn 1: prompt that will trigger tool use, then cancel mid-execution ---
	ctx1, cancel1 := context.WithCancel(context.Background())

	// Give the model just enough time to start streaming tool calls, then cancel.
	// We use a goroutine with a short timer so the cancel fires while tools run.
	done := make(chan error, 1)
	go func() {
		done <- a.RunTurn(ctx1, 1, `Use the bash tool to run: echo "hello from tool" && sleep 30`)
	}()

	// Wait a bit for the model to emit tool_use and the tool to start executing,
	// then cancel (simulating Ctrl-C).
	time.Sleep(3 * time.Second)
	cancel1()

	err := <-done
	// The turn should return context.Canceled (not a 400 error).
	if err != nil {
		assert.ErrorIs(t, err, context.Canceled,
			"turn 1 should fail with context.Canceled, got: %v", err)
	}
	t.Logf("Turn 1 output:\n%s", buf.String())

	// --- Turn 2: follow-up turn on the SAME agent (same conversation history) ---
	buf.Reset()
	ctx2, cancel2 := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel2()

	err = a.RunTurn(ctx2, 2, `Respond with exactly the word: pong`)
	// This is the critical assertion: the follow-up must NOT fail with the
	// "tool_use ids were found without tool_result blocks" error.
	require.NoError(t, err, "turn 2 should succeed; conversation history should be consistent after cancel. Got: %v", err)

	out := stripANSI(buf.String())
	t.Logf("Turn 2 output:\n%s", out)
	assert.Contains(t, strings.ToLower(out), "pong",
		"expected follow-up turn to contain 'pong'")
}
