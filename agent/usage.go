package agent

import (
	"strconv"

	"github.com/codewandler/agentsdk/runner"
	coreusage "github.com/codewandler/agentsdk/usage"
)

func (a *Agent) applyRouteIdentity(ev runner.RouteEvent) {
	a.providerIdentity = ev.ProviderIdentity
	a.resolvedProvider = ev.ProviderIdentity.ProviderName
	a.resolvedModel = ev.ProviderIdentity.NativeModel
}

func (a *Agent) recordRunnerUsage(turnID int, ev runner.UsageEvent) coreusage.Record {
	return coreusage.FromRunnerEvent(ev, coreusage.RunnerEventOptions{
		TurnID:        strconv.Itoa(turnID),
		SessionID:     a.sessionID,
		FallbackModel: a.inference.Model,
		RouteState: coreusage.RouteState{
			Provider: a.resolvedProvider,
			Model:    a.resolvedModel,
		},
	})
}

func (a *Agent) providerName() string {
	return a.resolvedProvider
}

// aggregateTurn sums all usage records for a given turn ID.
func (a *Agent) aggregateTurn(turnID int) coreusage.Record {
	return a.tracker.AggregateTurn(strconv.Itoa(turnID))
}
