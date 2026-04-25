package agent

import (
	"context"

	"github.com/codewandler/agentsdk/conversation"
	agentruntime "github.com/codewandler/agentsdk/runtime"
	acoreTool "github.com/codewandler/agentsdk/tool"
	"github.com/codewandler/llmadapter/unified"
)

func (a *Agent) initRuntime() error {
	if a.client == nil {
		result, err := agentruntime.AutoMuxClient(a.inference.Model, a.sourceAPI, a.autoMux)
		if err != nil {
			return err
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

func (a *Agent) baseRuntimeOptions(includeSessionID bool) []agentruntime.Option {
	opts := []agentruntime.Option{
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
	if includeSessionID {
		opts = append([]agentruntime.Option{agentruntime.WithSessionOptions(conversation.WithSessionID(conversation.SessionID(a.sessionID)))}, opts...)
	}
	if reasoning, ok := a.reasoningConfig(); ok {
		opts = append(opts, agentruntime.WithReasoning(reasoning))
	}
	return opts
}

func (a *Agent) runtimeOptions() []agentruntime.Option {
	opts := a.baseRuntimeOptions(true)
	if a.session != nil {
		opts = append(opts, agentruntime.WithSession(a.session))
	}
	return opts
}
