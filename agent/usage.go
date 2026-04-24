package agent

import (
	"strconv"
	"strings"

	"github.com/codewandler/agentsdk/conversation"
	"github.com/codewandler/agentsdk/runner"
	coreusage "github.com/codewandler/agentsdk/usage"
	"github.com/codewandler/llmadapter/unified"
)

func (a *Agent) applyRouteIdentity(ev runner.RouteEvent) {
	a.providerIdentity = ev.ProviderIdentity
	a.resolvedProvider = ev.ProviderIdentity.ProviderName
	a.resolvedModel = ev.ProviderIdentity.NativeModel
}

func (a *Agent) recordTransportUsage(turnID int, u unified.Usage, identity conversation.ProviderIdentity, model string) coreusage.Record {
	providerName, modelName := a.providerAndModel(identity, model)
	rec := coreusage.FromUnified(u, coreusage.Dims{
		Provider:  providerName,
		Model:     modelName,
		TurnID:    strconv.Itoa(turnID),
		SessionID: a.sessionID,
	})
	rec.Source = "llmadapter"
	return rec
}

func (a *Agent) providerAndModel(identity conversation.ProviderIdentity, fallbackModel string) (string, string) {
	providerName := identity.ProviderName
	model := identity.NativeModel
	if providerName == "" {
		providerName = a.resolvedProvider
	}
	if model == "" {
		model = a.resolvedModel
	}
	if model == "" {
		model = fallbackModel
	}
	if model == "" {
		model = a.inference.Model
	}
	if providerName == "" && len(model) > 0 && model[0] != '/' {
		parts := strings.SplitN(model, "/", 2)
		if len(parts) == 2 {
			providerName, model = parts[0], parts[1]
		}
	}
	if providerName != "" && strings.HasPrefix(model, providerName+"/") {
		model = strings.TrimPrefix(model, providerName+"/")
	}
	return providerName, model
}

func (a *Agent) providerName() string {
	return a.resolvedProvider
}

// aggregateTurn sums all usage records for a given turn ID.
func (a *Agent) aggregateTurn(turnID int) coreusage.Record {
	return coreusage.Merge(a.tracker.Filter(coreusage.ByTurnID(strconv.Itoa(turnID)), coreusage.ExcludeEstimates())...)
}
