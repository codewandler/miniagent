package agent

import (
	"time"

	"github.com/codewandler/agentsdk/tools/standard"
)

func (a *Agent) setupTools(workspace string, toolTimeout time.Duration) {
	a.toolset = standard.DefaultToolset()
	a.allTools = a.toolset.Tools()
}
