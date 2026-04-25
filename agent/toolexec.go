package agent

import (
	"context"

	agentruntime "github.com/codewandler/agentsdk/runtime"
)

func (a *Agent) newToolCtx(ctx context.Context) *agentruntime.ToolContext {
	return agentruntime.NewToolContext(ctx,
		agentruntime.WithToolWorkDir(a.workspace),
		agentruntime.WithToolSessionID(a.sessionID),
		agentruntime.WithToolActivation(a.toolset.Activation()),
	)
}
