package agent

import "github.com/codewandler/llmadapter/unified"

func (a *Agent) reasoningConfig() (unified.ReasoningConfig, bool) {
	switch a.inference.Thinking {
	case ThinkingModeOff:
		return unified.ReasoningConfig{}, false
	case ThinkingModeAuto, "":
		return unified.ReasoningConfig{}, false
	default:
		return unified.ReasoningConfig{Effort: a.inference.Effort, Expose: true}, true
	}
}
