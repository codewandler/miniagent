package agent

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/codewandler/agentapis/api/unified"
	"github.com/codewandler/agentapis/client"
	"github.com/codewandler/agentapis/conversation"
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
		inference: DefaultInferenceOptions(),
	}
	summary := a.ParamsSummary()
	assert.Contains(t, summary, "model:")
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

type errStreamer struct{}

func (e errStreamer) Stream(ctx context.Context, req unified.Request) (<-chan client.StreamResult, error) {
	ch := make(chan client.StreamResult, 1)
	ch <- client.StreamResult{Err: errors.New("stream error")}
	close(ch)
	return ch, nil
}

func (e errStreamer) Name() string { return "err" }
