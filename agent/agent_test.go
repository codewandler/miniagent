package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	acoreTool "github.com/codewandler/agentsdk/tool"
	"github.com/codewandler/llmadapter/adapt"
	"github.com/codewandler/llmadapter/adapterconfig"
	"github.com/codewandler/llmadapter/unified"
	"github.com/invopop/jsonschema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewInferenceOptions_AppliesOverrides(t *testing.T) {
	opts := NewInferenceOptions(WithModel("claude-sonnet"), WithMaxTokens(2048), WithThinking(ThinkingModeOff), WithEffort(unified.ReasoningEffortHigh), WithTemperature(0.7))
	assert.Equal(t, "claude-sonnet", opts.Model)
	assert.Equal(t, 2048, opts.MaxTokens)
	assert.Equal(t, ThinkingModeOff, opts.Thinking)
	assert.Equal(t, unified.ReasoningEffortHigh, opts.Effort)
	assert.Equal(t, 0.7, opts.Temperature)
}

func TestDefaultInferenceOptions(t *testing.T) {
	opts := DefaultInferenceOptions()
	assert.NotEmpty(t, opts.Model)
	assert.Greater(t, opts.MaxTokens, 0)
	assert.Equal(t, ThinkingModeAuto, opts.Thinking)
}

func TestAgentReasoningConfigRequiresExplicitThinking(t *testing.T) {
	a := &Agent{inference: DefaultInferenceOptions()}
	_, ok := a.reasoningConfig()
	require.False(t, ok)

	a.inference.Thinking = ThinkingModeOn
	cfg, ok := a.reasoningConfig()
	require.True(t, ok)
	assert.True(t, cfg.Expose)
	assert.Equal(t, unified.ReasoningEffortMedium, cfg.Effort)
}

func TestAgent_ResetClearsState(t *testing.T) {
	a := New(
		WithClient(newFakeClient()),
		WithWorkspace(t.TempDir()),
		WithToolTimeout(5*time.Second),
		WithInferenceOptions(InferenceOptions{
			Model:     TestServiceID + "/" + TestModelID,
			MaxTokens: 1000,
		}),
	)
	oldSessionID := a.sessionID
	a.Reset()
	assert.NotEmpty(t, a.sessionID)
	assert.NotEqual(t, oldSessionID, a.sessionID, "sessionID should change after Reset")
}

func TestAgent_ParamsSummary(t *testing.T) {
	a := &Agent{
		inference:        DefaultInferenceOptions(),
		resolvedProvider: TestServiceID,
		resolvedModel:    TestModelID,
	}
	summary := a.ParamsSummary()
	assert.Contains(t, summary, "model:")
	assert.Contains(t, summary, "resolved_instance:")
	assert.Contains(t, summary, "resolved_model:")
	assert.Contains(t, summary, "thinking:")
	assert.Contains(t, summary, "effort:")
}

func TestAgentAutoMuxUsesIntentAndDynamicModels(t *testing.T) {
	var got adapterconfig.AutoOptions
	a := New(
		func(a *Agent) {
			a.autoMux = func(opts adapterconfig.AutoOptions) (adapterconfig.AutoResult, error) {
				got = opts
				return adapterconfig.AutoResult{
					Client: newFakeClient(),
					Config: adapterconfig.Config{Routes: []adapterconfig.RouteConfig{{
						SourceAPI:   opts.SourceAPI,
						Model:       opts.Intents[0].Name,
						Provider:    "test",
						ProviderAPI: adapt.ApiOpenAIResponses,
						NativeModel: TestModelID,
					}, {
						SourceAPI:     opts.SourceAPI,
						Provider:      "test",
						ProviderAPI:   adapt.ApiOpenAIResponses,
						DynamicModels: true,
					}}},
					Enabled: []adapterconfig.AutoProvider{{Name: "test", Type: "test"}},
				}, nil
			}
		},
		WithWorkspace(t.TempDir()),
		WithInferenceOptions(NewInferenceOptions(WithModel("gpt-4.1-mini"), WithMaxTokens(1000))),
	)
	require.NotNil(t, a)
	require.True(t, got.DynamicModels)
	require.Equal(t, adapt.ApiOpenAIResponses, got.SourceAPI)
	require.Len(t, got.Intents, 1)
	require.Equal(t, DefaultInferenceOptions().Model, got.Intents[0].Name)
	require.Equal(t, adapt.ApiOpenAIResponses, got.Intents[0].SourceAPI)
}

func TestAgent_OutWriter(t *testing.T) {
	var buf bytes.Buffer
	a := &Agent{out: &buf}
	assert.Equal(t, &buf, a.Out())
}

func TestAgent_SessionID(t *testing.T) {
	a := &Agent{sessionID: "abc123"}
	assert.Equal(t, "abc123", a.SessionID())
}

func TestAgent_PersistsAndResumesSession(t *testing.T) {
	dir := t.TempDir()
	firstClient := &recordingClient{streams: [][]unified.Event{completedTextStream("first response")}}
	first := New(
		WithClient(firstClient),
		WithWorkspace(t.TempDir()),
		WithSessionStoreDir(dir),
		WithInferenceOptions(InferenceOptions{Model: TestServiceID + "/" + TestModelID, MaxTokens: 1000}),
	)

	require.NoError(t, first.RunTurn(context.Background(), 1, "first task"))
	storePath := first.SessionStorePath()
	require.NotEmpty(t, storePath)

	secondClient := &recordingClient{streams: [][]unified.Event{completedTextStream("second response")}}
	second := New(
		WithClient(secondClient),
		WithWorkspace(t.TempDir()),
		WithSessionStoreDir(dir),
		WithResumeSession(storePath),
		WithInferenceOptions(InferenceOptions{Model: TestServiceID + "/" + TestModelID, MaxTokens: 1000}),
	)

	require.Equal(t, first.SessionID(), second.SessionID())
	require.NoError(t, second.RunTurn(context.Background(), 1, "second task"))
	require.Len(t, secondClient.requests, 1)
	require.Len(t, secondClient.requests[0].Messages, 3)
	requireMessageText(t, secondClient.requests[0].Messages[0], "first task")
	requireMessageText(t, secondClient.requests[0].Messages[1], "first response")
	requireMessageText(t, secondClient.requests[0].Messages[2], "second task")
}

func TestAgent_WithMaxSteps(t *testing.T) {
	a := &Agent{}
	WithMaxSteps(50)(a)
	assert.Equal(t, 50, a.maxSteps)
}

func TestAgent_WithToolTimeout(t *testing.T) {
	a := &Agent{}
	WithToolTimeout(10 * time.Second)(a)
	assert.Equal(t, 10*time.Second, a.toolTimeout)
}

func TestAgent_WithWorkspace(t *testing.T) {
	a := &Agent{}
	WithWorkspace("/tmp")(a)
	assert.Equal(t, "/tmp", a.workspace)
}

func TestRunTurn_StreamError(t *testing.T) {
	a := New(
		WithClient(errClient{}),
		WithWorkspace(t.TempDir()),
		WithToolTimeout(5*time.Second),
		WithMaxSteps(1),
		WithInferenceOptions(InferenceOptions{
			Model:     TestServiceID + "/" + TestModelID,
			MaxTokens: 1000,
		}),
	)

	var buf bytes.Buffer
	a.out = &buf

	err := a.RunTurn(context.Background(), 1, "oops")
	require.Error(t, err)
}

func TestRunTurn_StreamErrorIncludesDiagnostics(t *testing.T) {
	a := New(
		WithClient(errClient{}),
		WithWorkspace(t.TempDir()),
		WithToolTimeout(5*time.Second),
		WithMaxSteps(1),
		WithVerbose(true),
		WithInferenceOptions(InferenceOptions{
			Model:     TestServiceID + "/" + TestModelID,
			MaxTokens: 1000,
		}),
	)
	a.resolvedProvider = TestServiceID
	a.resolvedModel = TestModelID

	var buf bytes.Buffer
	a.out = &buf

	err := a.RunTurn(context.Background(), 1, "oops")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "provider="+TestServiceID)
	assert.Contains(t, err.Error(), "model="+TestModelID)
}

type errClient struct{}

func (e errClient) Request(ctx context.Context, req unified.Request) (<-chan unified.Event, error) {
	ch := make(chan unified.Event, 1)
	ch <- unified.ErrorEvent{Err: errors.New("stream error")}
	close(ch)
	return ch, nil
}

type blockingCancelTool struct {
	name    string
	started chan struct{}
}

func (t blockingCancelTool) Name() string        { return t.name }
func (t blockingCancelTool) Description() string { return "blocks until canceled" }
func (t blockingCancelTool) Guidance() string    { return "" }
func (t blockingCancelTool) Schema() *jsonschema.Schema {
	return &jsonschema.Schema{Type: "object"}
}
func (t blockingCancelTool) Execute(ctx acoreTool.Ctx, input json.RawMessage) (acoreTool.Result, error) {
	select {
	case <-t.started:
	default:
		close(t.started)
	}
	<-ctx.Done()
	return nil, ctx.Err()
}

func completedToolCallStream(calls ...unified.ToolCall) []unified.Event {
	out := make([]unified.Event, 0, len(calls)*2+1)
	for _, call := range calls {
		out = append(out,
			unified.ToolCallStartEvent{Index: call.Index, ID: call.ID, Name: call.Name},
			unified.ToolCallDoneEvent{Index: call.Index, ID: call.ID, Name: call.Name, Args: call.Arguments},
		)
	}
	out = append(out, unified.CompletedEvent{FinishReason: unified.FinishReasonToolCall, MessageID: "resp_tool"})
	return out
}

func completedTextStream(text string) []unified.Event {
	return []unified.Event{
		unified.TextDeltaEvent{Text: text},
		unified.CompletedEvent{FinishReason: unified.FinishReasonStop, MessageID: "resp_text"},
	}
}

func requireMessageText(t *testing.T, msg unified.Message, want string) {
	t.Helper()
	require.Len(t, msg.Content, 1)
	text, ok := msg.Content[0].(unified.TextPart)
	require.True(t, ok)
	require.Equal(t, want, text.Text)
}

func TestRunTurn_CancelDuringToolExecutionDoesNotCommitPartialTurn(t *testing.T) {
	streamer := &recordingClient{streams: [][]unified.Event{
		completedToolCallStream(unified.ToolCall{ID: "call_1", Name: "cancel_tool", Arguments: json.RawMessage(`{"x":1}`)}),
		completedTextStream("ack"),
	}}
	a := New(
		WithClient(streamer),
		WithWorkspace(t.TempDir()),
		WithToolTimeout(5*time.Second),
		WithMaxSteps(2),
		WithInferenceOptions(InferenceOptions{Model: TestServiceID + "/" + TestModelID, MaxTokens: 1000}),
	)
	tool := blockingCancelTool{name: "cancel_tool", started: make(chan struct{})}
	a.allTools = []acoreTool.Tool{tool}
	a.activation = NewActivationManager(a.allTools)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- a.RunTurn(ctx, 1, "run tool")
	}()

	<-tool.started
	cancel()

	err := <-done
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
	require.Len(t, streamer.requests, 1)

	err = a.RunTurn(context.Background(), 2, "continue")
	require.NoError(t, err)
	require.Len(t, streamer.requests, 2)
	assert.Len(t, streamer.requests[1].Messages, 1)
	assert.Equal(t, unified.RoleUser, streamer.requests[1].Messages[0].Role)
}

func TestRunTurn_CancelDuringFirstToolMarksRemainingToolCallsCanceled(t *testing.T) {
	streamer := &recordingClient{streams: [][]unified.Event{
		completedToolCallStream(
			unified.ToolCall{ID: "call_1", Name: "cancel_tool", Arguments: json.RawMessage(`{"x":1}`), Index: 0},
			unified.ToolCall{ID: "call_2", Name: "cancel_tool", Arguments: json.RawMessage(`{"x":2}`), Index: 1},
		),
	}}
	a := New(
		WithClient(streamer),
		WithWorkspace(t.TempDir()),
		WithToolTimeout(5*time.Second),
		WithMaxSteps(2),
		WithInferenceOptions(InferenceOptions{Model: TestServiceID + "/" + TestModelID, MaxTokens: 1000}),
	)
	tool := blockingCancelTool{name: "cancel_tool", started: make(chan struct{})}
	a.allTools = []acoreTool.Tool{tool}
	a.activation = NewActivationManager(a.allTools)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- a.RunTurn(ctx, 1, "run tools")
	}()

	<-tool.started
	cancel()

	err := <-done
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
	require.Len(t, streamer.requests, 1)
}
