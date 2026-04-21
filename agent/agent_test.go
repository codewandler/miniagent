package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/codewandler/agentapis/api/unified"
	"github.com/codewandler/agentapis/client"
	"github.com/codewandler/agentapis/conversation"
	acoreTool "github.com/codewandler/agentsdk/tool"
	"github.com/invopop/jsonschema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewInferenceOptions_AppliesOverrides(t *testing.T) {
	opts := NewInferenceOptions(WithModel("claude-sonnet"), WithMaxTokens(2048), WithThinking(unified.ThinkingModeOff), WithEffort(unified.EffortHigh), WithTemperature(0.7))
	assert.Equal(t, "claude-sonnet", opts.Model)
	assert.Equal(t, 2048, opts.MaxTokens)
	assert.Equal(t, unified.ThinkingModeOff, opts.Thinking)
	assert.Equal(t, unified.EffortHigh, opts.Effort)
	assert.Equal(t, 0.7, opts.Temperature)
}

func TestDefaultInferenceOptions(t *testing.T) {
	opts := DefaultInferenceOptions()
	assert.NotEmpty(t, opts.Model)
	assert.Greater(t, opts.MaxTokens, 0)
}

func TestAgent_ResetClearsState(t *testing.T) {
	svc := newFakeService()
	require.NotNil(t, svc, "newFakeService() returned nil")

	a := New(svc,
		WithWorkspace(t.TempDir()),
		WithToolTimeout(5*time.Second),
		WithInferenceOptions(InferenceOptions{
			Model:     TestServiceID + "/" + TestModelID, // Uses fake service's real ServiceID/ModelID
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
		inference:     DefaultInferenceOptions(),
		provider:      &testFakeProvider{streamer: testFakeStreamer{}},
		resolvedModel: TestModelID,
	}
	summary := a.ParamsSummary()
	assert.Contains(t, summary, "model:")
	assert.Contains(t, summary, "resolved_instance:")
	assert.Contains(t, summary, "resolved_model:")
	assert.Contains(t, summary, "thinking:")
	assert.Contains(t, summary, "effort:")
}

type testStreamer struct{}

func (t testStreamer) Stream(ctx context.Context, req unified.Request) (<-chan client.StreamResult, error) {
	ch := make(chan client.StreamResult)
	go func() {
		<-ctx.Done()
		close(ch)
	}()
	return ch, nil
}

func (t testStreamer) Name() string { return "test" }

func TestAgent_OutWriter(t *testing.T) {
	var buf bytes.Buffer
	a := &Agent{out: &buf}
	assert.Equal(t, &buf, a.Out())
}

func TestAgent_SessionID(t *testing.T) {
	a := &Agent{sessionID: "abc123"}
	assert.Equal(t, "abc123", a.SessionID())
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
	svc := newFakeService()
	require.NotNil(t, svc, "newFakeService() returned nil")

	a := New(svc,
		WithWorkspace(t.TempDir()),
		WithToolTimeout(5*time.Second),
		WithMaxSteps(1),
		WithInferenceOptions(InferenceOptions{
			Model:     TestServiceID + "/" + TestModelID,
			MaxTokens: 1000,
		}),
	)

	// Replace the session with one that returns errors
	a.session = conversation.New(errStreamer{})

	var buf bytes.Buffer
	a.out = &buf

	err := a.RunTurn(context.Background(), 1, "oops")
	require.Error(t, err)
}

func TestRunTurn_StreamErrorIncludesDiagnostics(t *testing.T) {
	svc := newFakeService()
	require.NotNil(t, svc, "newFakeService() returned nil")

	a := New(svc,
		WithWorkspace(t.TempDir()),
		WithToolTimeout(5*time.Second),
		WithMaxSteps(1),
		WithVerbose(true),
		WithInferenceOptions(InferenceOptions{
			Model:     TestServiceID + "/" + TestModelID,
			MaxTokens: 1000,
		}),
	)

	a.provider = &testFakeProvider{streamer: testFakeStreamer{}}
	a.resolvedModel = TestModelID
	a.session = conversation.New(errStreamer{}, conversation.WithModel(TestModelID))

	var buf bytes.Buffer
	a.out = &buf

	err := a.RunTurn(context.Background(), 1, "oops")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "provider=test")
	assert.Contains(t, err.Error(), "model="+TestModelID)
}

type errStreamer struct{}

func (e errStreamer) Stream(ctx context.Context, req unified.Request) (<-chan client.StreamResult, error) {
	ch := make(chan client.StreamResult, 1)
	ch <- client.StreamResult{Err: errors.New("stream error")}
	close(ch)
	return ch, nil
}

func (e errStreamer) Name() string { return "err" }

type recordingStreamer struct {
	requests []unified.Request
	streams  [][]client.StreamResult
}

func (r *recordingStreamer) Stream(_ context.Context, req unified.Request) (<-chan client.StreamResult, error) {
	r.requests = append(r.requests, req)
	idx := len(r.requests) - 1
	var items []client.StreamResult
	if idx < len(r.streams) {
		items = r.streams[idx]
	}
	ch := make(chan client.StreamResult, len(items))
	for _, item := range items {
		ch <- item
	}
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

func completedToolCallStream(responseID string, calls ...unified.ToolCall) []client.StreamResult {
	out := make([]client.StreamResult, 0, len(calls)+1)
	for _, call := range calls {
		call := call
		out = append(out, client.StreamResult{Event: unified.StreamEvent{
			Type: unified.StreamEventToolCall,
			StreamToolCall: &unified.StreamToolCall{
				Ref:  unified.StreamRef{ResponseID: responseID},
				ID:   call.ID,
				Name: call.Name,
				Args: call.Args,
			},
			ToolCall: &unified.ToolCall{ID: call.ID, Name: call.Name, Args: call.Args},
		}})
	}
	out = append(out, client.StreamResult{Event: unified.StreamEvent{
		Type: unified.StreamEventCompleted,
		Lifecycle: &unified.Lifecycle{Scope: unified.LifecycleScopeResponse, State: unified.LifecycleStateDone, Ref: unified.StreamRef{ResponseID: responseID}},
		Completed: &unified.Completed{StopReason: unified.StopReasonToolUse},
	}})
	return out
}

func completedTextStream(responseID, text string) []client.StreamResult {
	return []client.StreamResult{
		{Event: unified.StreamEvent{Type: unified.StreamEventContentDelta, ContentDelta: &unified.ContentDelta{ContentBase: unified.ContentBase{Ref: unified.StreamRef{ResponseID: responseID}, Kind: unified.ContentKindText, Data: text}}}},
		{Event: unified.StreamEvent{Type: unified.StreamEventCompleted, Lifecycle: &unified.Lifecycle{Scope: unified.LifecycleScopeResponse, State: unified.LifecycleStateDone, Ref: unified.StreamRef{ResponseID: responseID}}, Completed: &unified.Completed{StopReason: unified.StopReasonEndTurn}}},
	}
}

func TestRunTurn_CancelDuringToolExecutionFlushesCanceledToolResult(t *testing.T) {
	streamer := &recordingStreamer{streams: [][]client.StreamResult{
		completedToolCallStream("resp_tool", unified.ToolCall{ID: "call_1", Name: "cancel_tool", Args: map[string]any{"x": 1}}),
		completedTextStream("resp_followup", "ack"),
	}}
	provider := &testFakeProvider{streamer: streamer}
	svc := newFakeService()
	a := New(svc,
		WithWorkspace(t.TempDir()),
		WithToolTimeout(5*time.Second),
		WithMaxSteps(2),
		WithInferenceOptions(InferenceOptions{Model: TestServiceID + "/" + TestModelID, MaxTokens: 1000}),
	)
	a.provider = provider
	a.resolvedModel = TestModelID
	a.session = conversation.New(provider.streamer, conversation.WithModel(TestModelID))
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
	require.Len(t, streamer.requests, 2)
	toolMessages := streamer.requests[1].Messages
	require.Len(t, toolMessages, 3)
	msg := toolMessages[2]
	require.Equal(t, unified.RoleTool, msg.Role)
	require.Len(t, msg.Parts, 1)
	require.NotNil(t, msg.Parts[0].ToolResult)
	assert.Equal(t, "call_1", msg.Parts[0].ToolResult.ToolCallID)
	assert.Equal(t, canceledToolResult, msg.Parts[0].ToolResult.ToolOutput)
	assert.True(t, msg.Parts[0].ToolResult.IsError)
}

func TestRunTurn_CancelDuringFirstToolMarksRemainingToolCallsCanceled(t *testing.T) {
	streamer := &recordingStreamer{streams: [][]client.StreamResult{
		completedToolCallStream("resp_tool",
			unified.ToolCall{ID: "call_1", Name: "cancel_tool", Args: map[string]any{"x": 1}},
			unified.ToolCall{ID: "call_2", Name: "cancel_tool", Args: map[string]any{"x": 2}},
		),
		completedTextStream("resp_followup", "ack"),
	}}
	provider := &testFakeProvider{streamer: streamer}
	svc := newFakeService()
	a := New(svc,
		WithWorkspace(t.TempDir()),
		WithToolTimeout(5*time.Second),
		WithMaxSteps(2),
		WithInferenceOptions(InferenceOptions{Model: TestServiceID + "/" + TestModelID, MaxTokens: 1000}),
	)
	a.provider = provider
	a.resolvedModel = TestModelID
	a.session = conversation.New(provider.streamer, conversation.WithModel(TestModelID))
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
	require.Len(t, streamer.requests, 2)
	toolMessages := streamer.requests[1].Messages
	require.Len(t, toolMessages, 4)
	for i, expectedID := range []string{"call_1", "call_2"} {
		msg := toolMessages[i+2]
		require.Equal(t, unified.RoleTool, msg.Role)
		require.Len(t, msg.Parts, 1)
		require.NotNil(t, msg.Parts[0].ToolResult)
		assert.Equal(t, expectedID, msg.Parts[0].ToolResult.ToolCallID)
		assert.Equal(t, canceledToolResult, msg.Parts[0].ToolResult.ToolOutput)
		assert.True(t, msg.Parts[0].ToolResult.IsError)
	}
}
