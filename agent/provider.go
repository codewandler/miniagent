package agent

import (
	"github.com/codewandler/agentsdk/conversation"
	agentruntime "github.com/codewandler/agentsdk/runtime"
)

func (a *Agent) resolveRouteIdentity() {
	a.providerIdentity = conversation.ProviderIdentity{}
	a.resolvedProvider = ""
	a.resolvedModel = ""
	identity, summary, ok := agentruntime.RouteIdentity(a.autoResult, a.sourceAPI, a.inference.Model)
	if !ok {
		return
	}
	a.resolvedProvider = summary.Provider
	a.resolvedModel = summary.NativeModel
	a.providerIdentity = identity
}
