package agent

import "github.com/codewandler/agentsdk/conversation"

func (a *Agent) resolveRouteIdentity() {
	a.providerIdentity = conversation.ProviderIdentity{}
	a.resolvedProvider = ""
	a.resolvedModel = ""
	summary, ok := a.autoResult.RouteSummary(a.sourceAPI, a.inference.Model)
	if !ok {
		return
	}
	a.resolvedProvider = summary.Provider
	a.resolvedModel = summary.NativeModel
	a.providerIdentity = conversation.ProviderIdentity{
		ProviderName: summary.Provider,
		APIKind:      string(summary.ProviderAPI),
		NativeModel:  summary.NativeModel,
	}
}
