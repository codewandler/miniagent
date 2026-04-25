package integration

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/codewandler/agentsdk/runnertest"
	"github.com/codewandler/llmadapter/unified"
	"github.com/codewandler/miniagent/agent"
	"github.com/stretchr/testify/require"
)

func TestThinkingToolLoopReplaysSignedReasoning(t *testing.T) {
	client := runnertest.NewClient(
		[]unified.Event{
			unified.ContentBlockStartEvent{Index: 0, Kind: unified.ContentKindReasoning},
			unified.ReasoningDeltaEvent{Index: 0, Text: "need a directory listing", Signature: "signed-thinking"},
			unified.ContentBlockDoneEvent{Index: 0, Kind: unified.ContentKindReasoning},
			unified.ToolCallDoneEvent{Index: 1, ID: "call_dir", Name: "dir_tree", Args: runnertest.ToolCall("dir_tree", "call_dir", 1, `{"path":".","depth":1}`).Arguments},
			unified.CompletedEvent{FinishReason: unified.FinishReasonToolCall, MessageID: "msg_tool"},
		},
		runnertest.TextStream("done", "msg_done"),
	)
	var out bytes.Buffer
	a := agent.New(
		agent.WithClient(client),
		agent.WithWorkspace(t.TempDir()),
		agent.WithToolTimeout(5*time.Second),
		agent.WithOutput(&out),
		agent.WithInferenceOptions(agent.NewInferenceOptions(agent.WithThinking(agent.ThinkingModeOn))),
	)

	require.NoError(t, a.RunTurn(context.Background(), 1, "inspect the directory"))
	require.Len(t, client.Requests(), 2)

	var assistant unified.Message
	for _, msg := range client.RequestAt(1).Messages {
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
