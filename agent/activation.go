package agent

import (
	"github.com/codewandler/agentcore/interfaces"
	acoreTool "github.com/codewandler/agentcore/tool"
	"path/filepath"
)

// ActivationManager implements the ActivationState interface for tool management.
type ActivationManager struct {
	allTools  []acoreTool.Tool
	activeSet map[string]bool
}

// NewActivationManager creates a new ActivationManager with all tools initially active.
func NewActivationManager(allTools []acoreTool.Tool) *ActivationManager {
	activeSet := make(map[string]bool)
	for _, t := range allTools {
		activeSet[t.Name()] = true
	}
	return &ActivationManager{
		allTools:  allTools,
		activeSet: activeSet,
	}
}

// AllTools returns all registered tools.
func (a *ActivationManager) AllTools() []acoreTool.Tool {
	return a.allTools
}

// ActiveTools returns currently active tools.
func (a *ActivationManager) ActiveTools() []acoreTool.Tool {
	var active []acoreTool.Tool
	for _, t := range a.allTools {
		if a.activeSet[t.Name()] {
			active = append(active, t)
		}
	}
	return active
}

// Activate makes tools matching patterns active, returns list of activated tool names.
func (a *ActivationManager) Activate(patterns ...string) []string {
	var activated []string
	for _, t := range a.allTools {
		for _, pattern := range patterns {
			if matchesPattern(t.Name(), pattern) && !a.activeSet[t.Name()] {
				a.activeSet[t.Name()] = true
				activated = append(activated, t.Name())
				break
			}
		}
	}
	return activated
}

// Deactivate makes tools matching patterns inactive, returns list of deactivated tool names.
func (a *ActivationManager) Deactivate(patterns ...string) []string {
	var deactivated []string
	for _, t := range a.allTools {
		for _, pattern := range patterns {
			if matchesPattern(t.Name(), pattern) && a.activeSet[t.Name()] {
				a.activeSet[t.Name()] = false
				deactivated = append(deactivated, t.Name())
				break
			}
		}
	}
	return deactivated
}

// matchesPattern reports whether name matches a glob pattern.
func matchesPattern(name, pattern string) bool {
	matched, _ := filepath.Match(pattern, name)
	return matched
}

// Verify that ActivationManager implements ActivationState.
var _ interfaces.ActivationState = (*ActivationManager)(nil)
