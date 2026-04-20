package agent

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strconv"
	"testing"
	"time"

	"github.com/codewandler/agentapis/api/unified"
	"github.com/codewandler/agentapis/client"
	"github.com/codewandler/agentapis/conversation"
	"github.com/codewandler/llm/usage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestAgent(t *testing.T, opts ...Option) (*Agent, *bytes.Buffer) {
	t.Helper()
	var buf bytes.Buffer
	return New(newFakeStreamer(), append([]Option{WithWorkspace(t.TempDir()), WithToolTimeout(5 * time.Second), WithOutput(&buf)}, opts...)...), &buf
}

type fakeStreamer struct {
	calls []unified.Request
	n     int
}

func newFakeStreamer() *fakeStreamer { return &fakeStreamer{} }
func (f *fakeStreamer) Stream(_ context.Context, req unified.Request) (<-chan client.StreamResult, error) {
	f.calls = append(f.calls, req)
	ch := make(chan client.StreamResult, 8)
	go func() {
		defer close(ch)
		responseID := "resp_" + strconv.Itoa(f.n+1)
		if f.n == 0 {
			ch <- client.StreamResult{Event: unified.NewToolCallEvent("bash-1", "bash", map[string]any{"command": "echo hello"})}
			ch <- client.StreamResult{Event: unified.NewUsageEvent(unified.TokenItems{{Kind: unified.TokenKindInputNew, Count: 1}, {Kind: unified.TokenKindOutput, Count: 1}}, nil)}
			ch <- client.StreamResult{Event: unified.StreamEvent{Type: unified.StreamEventCompleted, Completed: &unified.Completed{StopReason: unified.StopReasonToolUse}, Lifecycle: &unified.Lifecycle{Ref: unified.StreamRef{ResponseID: responseID}}}}
		} else {
			ch <- client.StreamResult{Event: unified.StreamEvent{Type: unified.StreamEventContentDelta, ContentDelta: &unified.ContentDelta{ContentBase: unified.ContentBase{Ref: unified.StreamRef{ResponseID: responseID}, Kind: unified.ContentKindText, Data: "done"}}}}
			ch <- client.StreamResult{Event: unified.NewUsageEvent(unified.TokenItems{{Kind: unified.TokenKindInputNew, Count: 1}, {Kind: unified.TokenKindOutput, Count: 1}}, nil)}
			ch <- client.StreamResult{Event: unified.StreamEvent{Type: unified.StreamEventCompleted, Completed: &unified.Completed{StopReason: unified.StopReasonEndTurn}, Lifecycle: &unified.Lifecycle{Ref: unified.StreamRef{ResponseID: responseID}}}}
		}
		f.n++
	}()
	return ch, nil
}

type blockingStreamer struct{}

func (blockingStreamer) Stream(ctx context.Context, _ unified.Request) (<-chan client.StreamResult, error) {
	ch := make(chan client.StreamResult)
	go func() {
		<-ctx.Done()
		close(ch)
	}()
	return ch, nil
}

type captureStreamer struct {
	create func(context.Context, unified.Request) (<-chan client.StreamResult, error)
}

func (p captureStreamer) Stream(ctx context.Context, req unified.Request) (<-chan client.StreamResult, error) {
	return p.create(ctx, req)
}

func singleTextStream() <-chan client.StreamResult {
	ch := make(chan client.StreamResult, 2)
	ch <- client.StreamResult{Event: unified.StreamEvent{Type: unified.StreamEventContentDelta, ContentDelta: &unified.ContentDelta{ContentBase: unified.ContentBase{Kind: unified.ContentKindText, Data: "done"}}}}
	ch <- client.StreamResult{Event: unified.StreamEvent{Type: unified.StreamEventCompleted, Completed: &unified.Completed{StopReason: unified.StopReasonEndTurn}}}
	close(ch)
	return ch
}

func TestNewInferenceOptions_AppliesOverrides(t *testing.T) {
	opts := NewInferenceOptions(WithModel("claude-sonnet"), WithMaxTokens(2048), WithThinking(unified.ThinkingModeOff), WithEffort(unified.EffortHigh), WithTemperature(0.7))
	assert.Equal(t, "claude-sonnet", opts.Model)
	assert.Equal(t, 2048, opts.MaxTokens)
	assert.Equal(t, unified.ThinkingModeOff, opts.Thinking)
	assert.Equal(t, unified.EffortHigh, opts.Effort)
	assert.Equal(t, 0.7, opts.Temperature)
}

func TestRunTurn_CompletesMultiStep(t *testing.T) {
	a, buf := newTestAgent(t)
	initialHistory := len(a.session.History())
	err := a.RunTurn(context.Background(), 1, "say hello")
	require.NoError(t, err)
	assert.Greater(t, len(a.session.History()), initialHistory+1)
	out := buf.String()
	assert.Contains(t, out, "Step 1")
	assert.Contains(t, out, "Step 2")
	recs := a.Tracker().Filter(usage.ByTurnID(strconv.Itoa(1)))
	assert.NotEmpty(t, recs)
}

func TestRunTurn_MaxStepsReached(t *testing.T) {
	a, _ := newTestAgent(t, WithMaxSteps(1))
	err := a.RunTurn(context.Background(), 1, "do something")
	assert.ErrorIs(t, err, ErrMaxStepsReached)
}

func TestSessionBuildRequestUsesInferenceDefaults(t *testing.T) {
	var got unified.Request
	a, _ := newTestAgent(t, WithMaxSteps(1))
	a.streamer = captureStreamer{create: func(_ context.Context, req unified.Request) (<-chan client.StreamResult, error) {
		got = req
		return singleTextStream(), nil
	}}
	a.initSession()
	doneReq, done, err := a.runStep(context.Background(), 1, 1, new(int), conversation.NewRequest().User("do something").Build())
	require.NoError(t, err)
	assert.True(t, done)
	assert.Equal(t, conversation.Request{}, doneReq)
	assert.Equal(t, a.inference.MaxTokens, got.MaxTokens)
	assert.Equal(t, a.inference.Effort, got.Effort)
	assert.Equal(t, a.inference.Thinking, got.Thinking)
}

func TestRunTurn_CancelledContext(t *testing.T) {
	var buf bytes.Buffer
	a := New(blockingStreamer{}, WithWorkspace(t.TempDir()), WithToolTimeout(5*time.Second), WithOutput(&buf))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := a.RunTurn(ctx, 1, "do something")
	assert.ErrorIs(t, err, context.Canceled)
}

func TestRunTurn_NoHistoryCommitOnCancel(t *testing.T) {
	var buf bytes.Buffer
	a := New(blockingStreamer{}, WithWorkspace(t.TempDir()), WithToolTimeout(5*time.Second), WithOutput(&buf))
	initialLen := len(a.session.History())
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(5 * time.Millisecond); cancel() }()
	_ = a.RunTurn(ctx, 1, "do something")
	assert.Equal(t, initialLen, len(a.session.History()))
}

func TestRunTurn_HistoryPersistsAcrossTurns(t *testing.T) {
	a, _ := newTestAgent(t)
	require.NoError(t, a.RunTurn(context.Background(), 1, "first task"))
	afterTurn1 := len(a.session.History())
	require.NoError(t, a.RunTurn(context.Background(), 2, "second task"))
	afterTurn2 := len(a.session.History())
	assert.Greater(t, afterTurn2, afterTurn1)
	assert.NotEmpty(t, a.Tracker().Filter(usage.ByTurnID(strconv.Itoa(1))))
	assert.NotEmpty(t, a.Tracker().Filter(usage.ByTurnID(strconv.Itoa(2))))
}

func TestNewIncludesWebSearchWhenTavilyConfigured(t *testing.T) {
	t.Setenv("TAVILY_API_KEY", "test-key")
	t.Setenv("WEBSEARCH_PROVIDER", "tavily")
	a := New(blockingStreamer{}, WithWorkspace(t.TempDir()), WithOutput(io.Discard))
	var names []string
	for _, def := range a.activeToolSpecs() { names = append(names, def.Name) }
	require.Contains(t, names, "web_fetch")
	require.Contains(t, names, "web_search")
}

func TestNewOmitsWebSearchWithoutTavilyKey(t *testing.T) {
	t.Setenv("TAVILY_API_KEY", "")
	t.Setenv("WEBSEARCH_PROVIDER", "")
	a := New(blockingStreamer{}, WithWorkspace(t.TempDir()), WithOutput(io.Discard))
	var names []string
	for _, def := range a.activeToolSpecs() { names = append(names, def.Name) }
	require.Contains(t, names, "web_fetch")
	require.NotContains(t, names, "web_search")
}

func TestAggregateTurnPreservesNonOverlappingOutputAndReasoning(t *testing.T) {
	a := &Agent{tracker: usage.NewTracker()}
	a.tracker.Record(usage.Record{Dims: usage.Dims{TurnID: "1"}, Tokens: usage.TokenItems{{Kind: usage.KindInput, Count: 10}, {Kind: usage.KindCacheRead, Count: 5}, {Kind: usage.KindOutput, Count: 21}, {Kind: usage.KindReasoning, Count: 9}}, Cost: usage.Cost{Total: 1.5, Input: 0.2, CacheRead: 0.1, Output: 0.8, Reasoning: 0.4}})
	agg := a.aggregateTurn(1)
	assert.Equal(t, 15, agg.Tokens.TotalInput())
	assert.Equal(t, 30, agg.Tokens.TotalOutput())
	assert.Equal(t, 21, agg.Tokens.Count(usage.KindOutput))
	assert.Equal(t, 9, agg.Tokens.Count(usage.KindReasoning))
	assert.Equal(t, 1.5, agg.Cost.Total)
}

func TestRunTurn_StreamError(t *testing.T) {
	a := New(captureStreamer{create: func(context.Context, unified.Request) (<-chan client.StreamResult, error) {
		ch := make(chan client.StreamResult, 1)
		ch <- client.StreamResult{Err: errors.New("boom")}
		close(ch)
		return ch, nil
	}}, WithWorkspace(t.TempDir()), WithOutput(io.Discard))
	err := a.RunTurn(context.Background(), 1, "oops")
	require.Error(t, err)
}
