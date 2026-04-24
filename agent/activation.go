package agent

import (
	"github.com/codewandler/agentsdk/activation"
	acoreTool "github.com/codewandler/agentsdk/tool"
)

// ActivationManager is kept as a miniagent-local alias while callers migrate
// to agentsdk/activation directly.
type ActivationManager = activation.Manager

// NewActivationManager creates a new ActivationManager with all tools initially active.
func NewActivationManager(allTools []acoreTool.Tool) *ActivationManager {
	return activation.New(allTools...)
}
