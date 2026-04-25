package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/codewandler/llmadapter/unified"
	"github.com/codewandler/miniagent/agent"
	"github.com/stretchr/testify/require"
)

type thinkingReplayClient struct {
	requests []unified.Request
	streams  [][]unified.Event
}

func (c *thinkingReplayClient) Request(_ context.Context, req unified.Request) (<-chan unified.Event, error) {
	c.requests = append(c.requests, req)
	idx := len(c.requests) - 1
	var events []unified.Event
	if idx < len(c.streams) {
		events = c.streams[idx]
	}
	out := make(chan unified.Event, len(events))
	for _, ev := range events {
		out <- ev
	}
	close(out)
	return out, nil
}

func TestThinkingToolLoopReplaysSignedReasoning(t *testing.T) {
	client := &thinkingReplayClient{streams: [][]unified.Event{
		{
			unified.ContentBlockStartEvent{Index: 0, Kind: unified.ContentKindReasoning},
			unified.ReasoningDeltaEvent{Index: 0, Text: "need a directory listing", Signature: "signed-thinking"},
			unified.ContentBlockDoneEvent{Index: 0, Kind: unified.ContentKindReasoning},
			unified.ToolCallDoneEvent{Index: 1, ID: "call_dir", Name: "dir_tree", Args: json.RawMessage(`{"path":".","depth":1}`)},
			unified.CompletedEvent{FinishReason: unified.FinishReasonToolCall, MessageID: "msg_tool"},
		},
		{
			unified.TextDeltaEvent{Text: "done"},
			unified.CompletedEvent{FinishReason: unified.FinishReasonStop, MessageID: "msg_done"},
		},
	}}
	var out bytes.Buffer
	a := agent.New(
		agent.WithClient(client),
		agent.WithWorkspace(t.TempDir()),
		agent.WithToolTimeout(5*time.Second),
		agent.WithOutput(&out),
		agent.WithInferenceOptions(agent.NewInferenceOptions(agent.WithThinking(agent.ThinkingModeOn))),
	)

	require.NoError(t, a.RunTurn(context.Background(), 1, "inspect the directory"))
	require.Len(t, client.requests, 2)

	var assistant unified.Message
	for _, msg := range client.requests[1].Messages {
		if msg.Role == unified.RoleAssistant {
			assistant = msg
			break
		}
	}
	require.Equal(t, unified.RoleAssistant, assistant.Role)
	require.NotEmpty(t, assistant.Content)
	reasoning, ok := assistant.Content[0].(unified.ReasoningPart)
	require.True(t, ok)
	require.Equal(t, "need a directory listing", reasoning.Text)
	require.Equal(t, "signed-thinking", reasoning.Signature)
}
