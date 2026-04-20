package agent

import (
	"bytes"
	"context"
	"io"
	"strconv"
	"testing"
	"time"

	"github.com/codewandler/llm"
	"github.com/codewandler/llm/llmtest"
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
	return New(
		fake.NewProvider(),
		append([]Option{WithWorkspace(t.TempDir()), WithToolTimeout(5 * time.Second), WithOutput(&buf)}, opts...)...,
	), &buf
}

// blockingProvider creates a provider whose stream never sends events.
// doProcess can only exit via ctx.Done() → deterministic cancel test.
type blockingProviderImpl struct{}

func (blockingProviderImpl) Name() string { return "blocking" }
func (blockingProviderImpl) Models() llm.Models {
	return llm.Models{{ID: "blocking/default", Name: "blocking", Provider: "blocking", Aliases: []string{llm.ModelDefault}}}
}
func (blockingProviderImpl) CreateStream(ctx context.Context, _ llm.Buildable) (llm.Stream, error) {
	ch := make(chan llm.Envelope)
	go func() {
		<-ctx.Done()
		close(ch)
	}()
	return ch, nil
}

func blockingProvider() llm.Provider { return blockingProviderImpl{} }

type captureProvider struct {
	create func(context.Context, llm.Buildable) (llm.Stream, error)
}

func (p captureProvider) Name() string { return "capture" }
func (p captureProvider) Models() llm.Models {
	return llm.Models{{ID: "capture/default", Name: "capture", Provider: "capture", Aliases: []string{llm.ModelDefault}}}
}
func (p captureProvider) CreateStream(ctx context.Context, src llm.Buildable) (llm.Stream, error) {
	return p.create(ctx, src)
}

func singleTextStream() llm.Stream {
	return llmtest.SendEvents(
		llmtest.TextEvent("done"),
		llmtest.CompletedEvent(llm.StopReasonEndTurn),
	)
}

func TestNewInferenceOptions_AppliesOverrides(t *testing.T) {
	opts := NewInferenceOptions(
		WithModel("claude-sonnet"),
		WithMaxTokens(2048),
		WithThinking(llm.ThinkingOff),
		WithEffort(llm.EffortHigh),
		WithTemperature(0.7),
	)

	assert.Equal(t, "claude-sonnet", opts.Model)
	assert.Equal(t, 2048, opts.MaxTokens)
	assert.Equal(t, llm.ThinkingOff, opts.Thinking)
	assert.Equal(t, llm.EffortHigh, opts.Effort)
	assert.Equal(t, 0.7, opts.Temperature)
}

func TestRunTurn_CompletesMultiStep(t *testing.T) {
	// fake provider: call 1 → tool_use (bash "echo hello"), call 2 → text "done"
	a, buf := newTestAgent(t)
	initialMsgs := len(a.messages) // system prompt only

	err := a.RunTurn(context.Background(), 1, "say hello")
	require.NoError(t, err)

	// History grew: system + user + assistant(tool) + tool_result + assistant(text) = 5
	assert.Greater(t, len(a.messages), initialMsgs+1, "messages should grow across steps")

	// Output contains step headers for both steps
	out := buf.String()
	assert.Contains(t, out, "Step 1")
	assert.Contains(t, out, "Step 2")

	// Usage recorded with turnID
	recs := a.Tracker().Filter(usage.ByTurnID(strconv.Itoa(1)))
	assert.NotEmpty(t, recs)
}

func TestRunTurn_MaxStepsReached(t *testing.T) {
	// fake returns tool_use on first call → maxSteps=1 → loop exhausted
	a, _ := newTestAgent(t, WithMaxSteps(1))

	err := a.RunTurn(context.Background(), 1, "do something")
	assert.ErrorIs(t, err, ErrMaxStepsReached)
}

func TestRunStep_SetsTopLevelRequestCacheHintFromMessages(t *testing.T) {
	a, _ := newTestAgent(t, WithMaxSteps(1))
	a.messages = a.initialMessages.Append(llm.User("do something"))

	var got llm.Request
	provider := captureProvider{create: func(_ context.Context, src llm.Buildable) (llm.Stream, error) {
		var err error
		got, err = src.BuildRequest(context.Background())
		require.NoError(t, err)
		return singleTextStream(), nil
	}}
	a.provider = provider

	done, err := a.runStep(context.Background(), 1, 1, new(int))
	require.NoError(t, err)
	assert.True(t, done)
	require.NotNil(t, got.CacheHint)
	assert.True(t, got.CacheHint.Enabled)
	assert.Equal(t, string(llm.CacheTTL1h), got.CacheHint.TTL)
}

// [REVIEW FIX #1]: use blocking provider — no buffered events → deterministic cancel.
func TestRunTurn_CancelledContext(t *testing.T) {
	var buf bytes.Buffer
	a := New(blockingProvider(), WithWorkspace(t.TempDir()), WithToolTimeout(5*time.Second), WithOutput(&buf))

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before RunTurn

	err := a.RunTurn(ctx, 1, "do something")
	assert.ErrorIs(t, err, context.Canceled)
}

// [REVIEW FIX #1]: use blocking provider for deterministic rollback test.
func TestRunTurn_RollbackOnCancel(t *testing.T) {
	var buf bytes.Buffer
	a := New(blockingProvider(), WithWorkspace(t.TempDir()), WithToolTimeout(5*time.Second), WithOutput(&buf))
	initialLen := len(a.messages)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(5 * time.Millisecond)
		cancel()
	}()

	_ = a.RunTurn(ctx, 1, "do something")
	assert.Equal(t, initialLen, len(a.messages), "messages should be rolled back")
}

func TestRunTurn_NoRollbackOnMaxSteps(t *testing.T) {
	a, _ := newTestAgent(t, WithMaxSteps(1))
	initialLen := len(a.messages)

	_ = a.RunTurn(context.Background(), 1, "do something")
	assert.Greater(t, len(a.messages), initialLen,
		"messages should NOT be rolled back on max-steps (history is valid)")
}

func TestRunTurn_HistoryPersistsAcrossTurns(t *testing.T) {
	a, _ := newTestAgent(t)

	// Turn 1: fake does tool_use → text (2 steps)
	err := a.RunTurn(context.Background(), 1, "first task")
	require.NoError(t, err)
	afterTurn1 := len(a.messages)

	// Turn 2: fake's called flag is true → returns text-only (1 step).
	// Exact count: +2 messages (user + assistant). If the fake's state machine
	// changes this will fail loudly rather than silently accepting a different structure.
	err = a.RunTurn(context.Background(), 2, "second task")
	require.NoError(t, err)
	afterTurn2 := len(a.messages)

	assert.Equal(t, afterTurn1+2, afterTurn2,
		"turn 2 (text-only, 1 step) should add exactly user + assistant = 2 messages")

	// Both turns have usage records
	assert.NotEmpty(t, a.Tracker().Filter(usage.ByTurnID(strconv.Itoa(1))))
	assert.NotEmpty(t, a.Tracker().Filter(usage.ByTurnID(strconv.Itoa(2))))
}

func TestNewIncludesWebSearchWhenTavilyConfigured(t *testing.T) {
	t.Setenv("TAVILY_API_KEY", "test-key")
	t.Setenv("WEBSEARCH_PROVIDER", "tavily")

	a := New(blockingProvider(), WithWorkspace(t.TempDir()), WithOutput(io.Discard))

	var names []string
	for _, def := range a.toolDefs {
		names = append(names, def.Name)
	}
	require.Contains(t, names, "web_fetch")
	require.Contains(t, names, "web_search")
}

func TestNewOmitsWebSearchWithoutTavilyKey(t *testing.T) {
	t.Setenv("TAVILY_API_KEY", "")
	t.Setenv("WEBSEARCH_PROVIDER", "")

	a := New(blockingProvider(), WithWorkspace(t.TempDir()), WithOutput(io.Discard))

	var names []string
	for _, def := range a.toolDefs {
		names = append(names, def.Name)
	}
	require.Contains(t, names, "web_fetch")
	require.NotContains(t, names, "web_search")
}

func TestAggregateTurnPreservesNonOverlappingOutputAndReasoning(t *testing.T) {
	a := &Agent{tracker: usage.NewTracker()}
	a.tracker.Record(usage.Record{
		Dims:   usage.Dims{TurnID: "1"},
		Tokens: usage.TokenItems{{Kind: usage.KindInput, Count: 10}, {Kind: usage.KindCacheRead, Count: 5}, {Kind: usage.KindOutput, Count: 21}, {Kind: usage.KindReasoning, Count: 9}},
		Cost:   usage.Cost{Total: 1.5, Input: 0.2, CacheRead: 0.1, Output: 0.8, Reasoning: 0.4},
	})
	agg := a.aggregateTurn(1)
	assert.Equal(t, 15, agg.Tokens.TotalInput())
	assert.Equal(t, 30, agg.Tokens.TotalOutput())
	assert.Equal(t, 21, agg.Tokens.Count(usage.KindOutput))
	assert.Equal(t, 9, agg.Tokens.Count(usage.KindReasoning))
	assert.Equal(t, 1.5, agg.Cost.Total)
	assert.Equal(t, 0.8, agg.Cost.Output)
	assert.Equal(t, 0.4, agg.Cost.Reasoning)
}
