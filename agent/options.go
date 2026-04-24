package agent

import (
	"io"
	"time"

	"github.com/codewandler/llmadapter/unified"
)

// Option configures the Agent.
type Option func(*Agent)

// InferenceOption configures InferenceOptions.
type InferenceOption func(*InferenceOptions)

// InferenceOptions holds the model/inference parameters used for each LLM call.
type InferenceOptions struct {
	Model       string
	MaxTokens   int
	Thinking    ThinkingMode
	Effort      unified.ReasoningEffort
	Temperature float64
}

type ThinkingMode string

const (
	ThinkingModeAuto ThinkingMode = "auto"
	ThinkingModeOn   ThinkingMode = "on"
	ThinkingModeOff  ThinkingMode = "off"
)

// DefaultInferenceOptions returns the default inference settings.
func DefaultInferenceOptions() InferenceOptions {
	return InferenceOptions{
		Model:       "codex/gpt-5.4",
		MaxTokens:   16_000,
		Thinking:    ThinkingModeOn,
		Effort:      unified.ReasoningEffortMedium,
		Temperature: 0.1,
	}
}

// NewInferenceOptions builds inference settings from defaults plus options.
func NewInferenceOptions(opts ...InferenceOption) InferenceOptions {
	cfg := DefaultInferenceOptions()
	for _, opt := range opts {
		opt(&cfg)
	}
	return cfg
}

// WithModel sets the model alias or full path.
func WithModel(m string) InferenceOption { return func(o *InferenceOptions) { o.Model = m } }

// WithMaxTokens sets the maximum output tokens per LLM call.
func WithMaxTokens(n int) InferenceOption { return func(o *InferenceOptions) { o.MaxTokens = n } }

// WithThinking sets the thinking mode.
func WithThinking(m ThinkingMode) InferenceOption {
	return func(o *InferenceOptions) { o.Thinking = m }
}

// WithEffort sets the effort level.
func WithEffort(e unified.ReasoningEffort) InferenceOption {
	return func(o *InferenceOptions) { o.Effort = e }
}

// WithTemperature sets the sampling temperature.
func WithTemperature(t float64) InferenceOption {
	return func(o *InferenceOptions) { o.Temperature = t }
}

// WithInferenceOptions sets all inference options at once.
func WithInferenceOptions(opts InferenceOptions) Option { return func(a *Agent) { a.inference = opts } }

// WithMaxSteps sets the maximum agent loop iterations per turn (default: 30).
func WithMaxSteps(n int) Option { return func(a *Agent) { a.maxSteps = n } }

// WithOutput sets the output writer (default: os.Stdout).
func WithOutput(w io.Writer) Option { return func(a *Agent) { a.out = w } }

// WithWorkspace sets the working directory (default: current working directory).
func WithWorkspace(dir string) Option { return func(a *Agent) { a.workspace = dir } }

// WithToolTimeout sets the per-tool call timeout (default: 30s).
func WithToolTimeout(d time.Duration) Option { return func(a *Agent) { a.toolTimeout = d } }

// WithSystemOverride sets a custom system prompt body (default: built from workspace).
func WithSystemOverride(prompt string) Option { return func(a *Agent) { a.systemOverride = prompt } }

// WithVerbose enables verbose runtime diagnostics.
func WithVerbose(verbose bool) Option { return func(a *Agent) { a.verbose = verbose } }

// WithClient injects a llmadapter client. Production callers should normally
// rely on auto-detection instead.
func WithClient(client unified.Client) Option { return func(a *Agent) { a.client = client } }
