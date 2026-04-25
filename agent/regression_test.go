package agent

import (
	"context"
	"testing"

	"github.com/codewandler/agentsdk/conversation"
	"github.com/codewandler/agentsdk/runnertest"
	"github.com/codewandler/llmadapter/adapt"
	"github.com/codewandler/llmadapter/adapterconfig"
	"github.com/codewandler/llmadapter/unified"
	"github.com/stretchr/testify/require"
)

func TestRegressionAutoMuxUsesRequestedModelAlias(t *testing.T) {
	var got adapterconfig.AutoOptions
	a := New(
		func(a *Agent) {
			a.autoMux = func(opts adapterconfig.AutoOptions) (adapterconfig.AutoResult, error) {
				got = opts
				return adapterconfig.AutoResult{
					Client: newFakeClient(),
					Config: adapterconfig.Config{
						Providers: []adapterconfig.ProviderConfig{{Name: "claude", Type: "claude"}},
						Routes: []adapterconfig.RouteConfig{{
							SourceAPI:   opts.SourceAPI,
							Model:       opts.Intents[0].Name,
							Provider:    "claude",
							ProviderAPI: adapt.ApiAnthropicMessages,
							NativeModel: "claude-haiku-4-5-20251001",
						}},
					},
					Enabled: []adapterconfig.AutoProvider{{Name: "claude", Type: "claude"}},
				}, nil
			}
		},
		WithWorkspace(t.TempDir()),
		WithInferenceOptions(NewInferenceOptions(WithModel("haiku"), WithMaxTokens(1000))),
	)

	require.NotNil(t, a)
	require.Len(t, got.Intents, 1)
	require.Equal(t, "haiku", got.Intents[0].Name)
	require.Equal(t, "claude", a.resolvedProvider)
	require.Equal(t, "claude-haiku-4-5-20251001", a.resolvedModel)
	require.Equal(t, "claude", a.providerIdentity.ProviderName)
	require.Equal(t, string(adapt.ApiAnthropicMessages), a.providerIdentity.APIKind)
	require.Equal(t, "claude-haiku-4-5-20251001", a.providerIdentity.NativeModel)
}

func TestRegressionRouteUsageDimsFollowRunnerRoute(t *testing.T) {
	client := runnertest.NewClient([]unified.Event{
		runnertest.Route("claude", "anthropic.messages", "messages", "haiku", "claude-haiku-4-5-20251001"),
		unified.NewUsageEvent(
			unified.TokenItems{{Kind: unified.TokenKindInputNew, Count: 10}},
			unified.CostItems{{Kind: unified.CostKindInput, Amount: 0.01}},
		),
		unified.TextDeltaEvent{Text: "ok"},
		unified.CompletedEvent{FinishReason: unified.FinishReasonStop, MessageID: "msg_1"},
	})
	a := New(
		WithClient(client),
		WithWorkspace(t.TempDir()),
		WithInferenceOptions(NewInferenceOptions(WithModel("haiku"), WithMaxTokens(1000))),
	)

	require.NoError(t, a.RunTurn(context.Background(), 1, "hello"))
	records := a.Tracker().Records()
	require.Len(t, records, 1)
	require.Equal(t, "claude", records[0].Dims.Provider)
	require.Equal(t, "claude-haiku-4-5-20251001", records[0].Dims.Model)
	require.Equal(t, "1", records[0].Dims.TurnID)
	require.Equal(t, a.SessionID(), records[0].Dims.SessionID)
	require.Equal(t, "llmadapter", records[0].Source)
	require.Equal(t, 10, records[0].Usage.Tokens.Count(unified.TokenKindInputNew))
	require.Equal(t, 0.01, records[0].Usage.Costs.ByKind(unified.CostKindInput))
}

func TestRegressionReplayResumeKeepsCanonicalHistoryUnchanged(t *testing.T) {
	dir := t.TempDir()
	firstClient := runnertest.NewClient(
		runnertest.TextStream("first response", "msg_first"),
		runnertest.TextStream("second response", "msg_second"),
	)
	first := New(
		WithClient(firstClient),
		WithWorkspace(t.TempDir()),
		WithSessionStoreDir(dir),
		WithInferenceOptions(InferenceOptions{Model: TestServiceID + "/" + TestModelID, MaxTokens: 1000}),
	)

	require.NoError(t, first.RunTurn(context.Background(), 1, "first task"))
	require.NoError(t, first.RunTurn(context.Background(), 2, "second task"))
	storePath := first.SessionStorePath()
	require.NotEmpty(t, storePath)

	secondClient := runnertest.NewClient(runnertest.TextStream("third response", "msg_third"))
	second := New(
		WithClient(secondClient),
		WithWorkspace(t.TempDir()),
		WithSessionStoreDir(dir),
		WithResumeSession(storePath),
		WithInferenceOptions(InferenceOptions{Model: TestServiceID + "/" + TestModelID, MaxTokens: 1000}),
	)

	require.Equal(t, first.SessionID(), second.SessionID())
	require.NoError(t, second.RunTurn(context.Background(), 1, "third task"))
	require.Len(t, secondClient.Requests(), 1)
	messages := secondClient.RequestAt(0).Messages
	require.Len(t, messages, 5)
	require.Equal(t, unified.RoleUser, messages[0].Role)
	requireMessageText(t, messages[0], "first task")
	require.Equal(t, unified.RoleAssistant, messages[1].Role)
	requireMessageText(t, messages[1], "first response")
	require.Equal(t, unified.RoleUser, messages[2].Role)
	requireMessageText(t, messages[2], "second task")
	require.Equal(t, unified.RoleAssistant, messages[3].Role)
	requireMessageText(t, messages[3], "second response")
	require.Equal(t, unified.RoleUser, messages[4].Role)
	requireMessageText(t, messages[4], "third task")
	for _, msg := range messages {
		require.NotEqual(t, unified.RoleSystem, msg.Role, "replay projection must not insert compaction summaries")
	}
}

func TestRegressionNativeContinuationKeepsCacheAndDoesNotReplayHistory(t *testing.T) {
	dir := t.TempDir()
	providerIdentity := conversation.ProviderIdentity{
		ProviderName: TestServiceID,
		APIKind:      "openai.responses",
		NativeModel:  TestModelID,
	}
	firstClient := runnertest.NewClient(runnertest.TextStream("first response", "resp_first"))
	first := New(
		WithClient(firstClient),
		WithWorkspace(t.TempDir()),
		WithSessionStoreDir(dir),
		WithInferenceOptions(InferenceOptions{Model: TestServiceID + "/" + TestModelID, MaxTokens: 1000}),
	)
	first.providerIdentity = providerIdentity

	require.NoError(t, first.RunTurn(context.Background(), 1, "first task"))
	storePath := first.SessionStorePath()
	require.NotEmpty(t, storePath)
	require.Equal(t, unified.CachePolicyOn, firstClient.RequestAt(0).CachePolicy)

	secondClient := runnertest.NewClient(runnertest.TextStream("second response", "resp_second"))
	second := New(
		WithClient(secondClient),
		WithWorkspace(t.TempDir()),
		WithSessionStoreDir(dir),
		WithResumeSession(storePath),
		WithInferenceOptions(InferenceOptions{Model: TestServiceID + "/" + TestModelID, MaxTokens: 1000}),
	)
	second.providerIdentity = providerIdentity

	require.Equal(t, first.SessionID(), second.SessionID())
	require.NoError(t, second.RunTurn(context.Background(), 1, "second task"))
	require.Len(t, secondClient.Requests(), 1)
	req := secondClient.RequestAt(0)
	require.Equal(t, unified.CachePolicyOn, req.CachePolicy)
	require.Equal(t, "miniagent:"+first.SessionID(), req.CacheKey)
	require.Len(t, req.Messages, 1)
	requireMessageText(t, req.Messages[0], "second task")
	previousResponseID, ok, err := unified.GetExtension[string](req.Extensions, unified.ExtOpenAIPreviousResponseID)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "resp_first", previousResponseID)
}
