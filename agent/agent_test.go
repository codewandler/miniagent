package agent

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/codewandler/llm"
	"github.com/codewandler/llm/provider/fake"
	"github.com/codewandler/llm/usage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestAgent creates an Agent backed by the fake provider.
// Output goes to a buffer (suppresses terminal noise in tests).
func newTestAgent(t *testing.T, opts ...Option) (*Agent, *bytes.Buffer) {
	t.Helper()
	var buf bytes.Buffer
	all := append([]Option{WithOutput(&buf)}, opts...)
	return New(
		fake.NewProvider(),
		t.TempDir(),
		5*time.Second,
		"", // default system prompt
		all...,
	), &buf
}

// blockingProvider creates a provider whose stream never sends events.
// doProcess can only exit via ctx.Done() → deterministic cancel test.
// Uses llm.NewProvider + StreamFunc: baseProvider.CreateStream just
// delegates to the streamer without model resolution, so this is safe.
func blockingProvider() llm.Provider {
	return llm.NewProvider("blocking",
		llm.WithStreamer(llm.StreamFunc(
			func(_ context.Context, _ llm.Buildable) (llm.Stream, error) {
				ch := make(chan llm.Envelope) // unbuffered, never written to
				return ch, nil
			},
		)),
	)
}

func TestRunTurn_CompletesMultiStep(t *testing.T) {
	// fake provider: call 1 → tool_use (bash "echo hello"), call 2 → text "done"
	a, buf := newTestAgent(t)
	initialMsgs := len(a.messages) // system prompt only

	err := a.RunTurn(context.Background(), "1", "say hello")
	require.NoError(t, err)

	// History grew: system + user + assistant(tool) + tool_result + assistant(text) = 5
	assert.Greater(t, len(a.messages), initialMsgs+1, "messages should grow across steps")

	// Output contains step headers for both steps
	out := buf.String()
	assert.Contains(t, out, "Step 1")
	assert.Contains(t, out, "Step 2")

	// Usage recorded with turnID
	recs := a.Tracker().Filter(usage.ByTurnID("1"))
	assert.NotEmpty(t, recs)
}

func TestRunTurn_MaxStepsReached(t *testing.T) {
	// fake returns tool_use on first call → maxSteps=1 → loop exhausted
	a, _ := newTestAgent(t, WithMaxSteps(1))

	err := a.RunTurn(context.Background(), "1", "do something")
	assert.ErrorIs(t, err, ErrMaxStepsReached)
}

// [REVIEW FIX #1]: use blocking provider — no buffered events → deterministic cancel.
func TestRunTurn_CancelledContext(t *testing.T) {
	var buf bytes.Buffer
	a := New(blockingProvider(), t.TempDir(), 5*time.Second, "", WithOutput(&buf))

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before RunTurn

	err := a.RunTurn(ctx, "1", "do something")
	assert.ErrorIs(t, err, context.Canceled)
}

// [REVIEW FIX #1]: use blocking provider for deterministic rollback test.
func TestRunTurn_RollbackOnCancel(t *testing.T) {
	var buf bytes.Buffer
	a := New(blockingProvider(), t.TempDir(), 5*time.Second, "", WithOutput(&buf))
	initialLen := len(a.messages)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_ = a.RunTurn(ctx, "1", "do something")
	assert.Equal(t, initialLen, len(a.messages), "messages should be rolled back")
}

func TestRunTurn_NoRollbackOnMaxSteps(t *testing.T) {
	a, _ := newTestAgent(t, WithMaxSteps(1))
	initialLen := len(a.messages)

	_ = a.RunTurn(context.Background(), "1", "do something")
	assert.Greater(t, len(a.messages), initialLen,
		"messages should NOT be rolled back on max-steps (history is valid)")
}

func TestRunTurn_HistoryPersistsAcrossTurns(t *testing.T) {
	a, _ := newTestAgent(t)

	// Turn 1: fake does tool_use → text (2 steps)
	err := a.RunTurn(context.Background(), "1", "first task")
	require.NoError(t, err)
	afterTurn1 := len(a.messages)

	// Turn 2: fake's called flag is true → returns text-only (1 step).
	// Exact count: +2 messages (user + assistant). If the fake's state machine
	// changes this will fail loudly rather than silently accepting a different structure.
	err = a.RunTurn(context.Background(), "2", "second task")
	require.NoError(t, err)
	afterTurn2 := len(a.messages)

	assert.Equal(t, afterTurn1+2, afterTurn2,
		"turn 2 (text-only, 1 step) should add exactly user + assistant = 2 messages")

	// Both turns have usage records
	assert.NotEmpty(t, a.Tracker().Filter(usage.ByTurnID("1")))
	assert.NotEmpty(t, a.Tracker().Filter(usage.ByTurnID("2")))
}
