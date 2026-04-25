package agent

import (
	"context"
	"fmt"

	"github.com/codewandler/agentsdk/conversation"
	agentruntime "github.com/codewandler/agentsdk/runtime"
	acoreTool "github.com/codewandler/agentsdk/tool"
	"github.com/codewandler/llmadapter/adapterconfig"
	"github.com/codewandler/llmadapter/unified"
)

func (a *Agent) initRuntime() error {
	if a.client == nil {
		autoMux := a.autoMux
		if autoMux == nil {
			autoMux = adapterconfig.AutoMuxClient
		}
		result, err := autoMux(adapterconfig.AutoOptions{
			EnableEnv:         true,
			EnableLocalClaude: true,
			EnableLocalCodex:  true,
			UseModelDB:        true,
			DynamicModels:     true,
			SourceAPI:         a.sourceAPI,
			Intents: []adapterconfig.AutoIntent{{
				Name:      DefaultInferenceOptions().Model,
				SourceAPI: a.sourceAPI,
			}},
		})
		if err != nil {
			return fmt.Errorf("auto-detect llmadapter providers: %w", err)
		}
		a.client = result.Client
		a.autoResult = result
	}
	a.resolveRouteIdentity()
	if err := a.initSession(context.Background()); err != nil {
		return err
	}
	runtimeAgent, err := agentruntime.New(a.client, a.runtimeOptions()...)
	if err != nil {
		return err
	}
	a.runtime = runtimeAgent
	return nil
}

func (a *Agent) runtimeOptions() []agentruntime.Option {
	opts := []agentruntime.Option{
		agentruntime.WithSessionOptions(conversation.WithSessionID(conversation.SessionID(a.sessionID))),
		agentruntime.WithModel(a.inference.Model),
		agentruntime.WithMaxOutputTokens(a.inference.MaxTokens),
		agentruntime.WithTemperature(a.inference.Temperature),
		agentruntime.WithSystem(BuildSystemPrompt(a.workspace, a.systemOverride)),
		agentruntime.WithTools(a.toolset.ActiveTools()),
		agentruntime.WithToolChoice(unified.ToolChoice{Mode: unified.ToolChoiceAuto}),
		agentruntime.WithCachePolicy(unified.CachePolicyOn),
		agentruntime.WithCacheKey(a.cacheKey()),
		agentruntime.WithMaxSteps(a.maxSteps),
		agentruntime.WithToolTimeout(a.toolTimeout),
		agentruntime.WithProviderIdentity(a.providerIdentity),
		agentruntime.WithToolContextFactory(func(ctx context.Context) acoreTool.Ctx {
			return a.newToolCtx(ctx)
		}),
	}
	if reasoning, ok := a.reasoningConfig(); ok {
		opts = append(opts, agentruntime.WithReasoning(reasoning))
	}
	if a.session != nil {
		opts = append(opts, agentruntime.WithSession(a.session))
	}
	return opts
}
