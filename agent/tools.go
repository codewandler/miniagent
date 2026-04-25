package agent

import (
	"time"

	"github.com/codewandler/agentsdk/tools/standard"
	"github.com/codewandler/agentsdk/tools/web"
)

func (a *Agent) setupTools(workspace string, toolTimeout time.Duration) {
	a.allTools = standard.Tools(standard.Options{
		WebSearchProvider:     web.DefaultSearchProviderFromEnv(),
		IncludeToolManagement: true,
	})
	a.activation = NewActivationManager(a.allTools)
}
