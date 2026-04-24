package agent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/codewandler/agentsdk/conversation"
	"github.com/codewandler/agentsdk/runner"
	agentruntime "github.com/codewandler/agentsdk/runtime"
	acoreTool "github.com/codewandler/agentsdk/tool"
	"github.com/codewandler/agentsdk/tools/standard"
	"github.com/codewandler/agentsdk/tools/web"
	coreusage "github.com/codewandler/agentsdk/usage"
	"github.com/codewandler/llmadapter/adapt"
	"github.com/codewandler/llmadapter/adapterconfig"
	"github.com/codewandler/llmadapter/unified"
	"github.com/codewandler/miniagent/agent/display"
	nanoid "github.com/matoous/go-nanoid/v2"
)

// Agent runs an agentic loop. A single Agent instance is reused across REPL
// turns; conversation history and usage records accumulate across turns.
type Agent struct {
	client           unified.Client
	autoMux          func(adapterconfig.AutoOptions) (adapterconfig.AutoResult, error)
	autoResult       adapterconfig.AutoResult
	providerIdentity conversation.ProviderIdentity
	resolvedProvider string
	resolvedModel    string
	sourceAPI        adapt.ApiKind
	runtime          *agentruntime.Agent
	tracker          *coreusage.Tracker
	allTools         []acoreTool.Tool
	activation       *ActivationManager
	inference        InferenceOptions
	maxSteps         int
	out              io.Writer
	workspace        string
	toolTimeout      time.Duration
	systemOverride   string
	sessionID        string
	verbose          bool
}

// New creates an Agent. All settings are configurable via Options.
func New(opts ...Option) *Agent {
	a, err := NewE(opts...)
	if err != nil {
		panic(fmt.Sprintf("failed to initialize agent runtime: %v", err))
	}
	return a
}

// NewE creates an Agent and returns initialization errors to callers that can
// surface them cleanly.
func NewE(opts ...Option) (*Agent, error) {
	sessionID, _ := nanoid.Generate("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789", 8)
	a := &Agent{
		inference:   DefaultInferenceOptions(),
		maxSteps:    30,
		out:         os.Stdout,
		toolTimeout: 30 * time.Second,
		sessionID:   sessionID,
		sourceAPI:   adapt.ApiOpenAIResponses,
	}
	for _, o := range opts {
		o(a)
	}

	ws := a.workspace
	if ws == "" {
		ws, _ = os.Getwd()
	}
	ws, _ = filepath.Abs(ws)
	a.workspace = ws

	a.tracker = coreusage.NewTracker()
	a.setupTools(a.workspace, a.toolTimeout)
	if err := a.initRuntime(); err != nil {
		return nil, err
	}
	return a, nil
}

func (a *Agent) initRuntime() error {
	if a.client == nil {
		autoMux := a.autoMux
		if autoMux == nil {
			autoMux = adapterconfig.AutoMuxClient
		}
		result, err := autoMux(adapterconfig.AutoOptions{
			EnableEnv:         true,
			EnableLocalClaude: true,
			EnableLocalCodex:  true,
			UseModelDB:        true,
			DynamicModels:     true,
			SourceAPI:         a.sourceAPI,
			Intents: []adapterconfig.AutoIntent{{
				Name:      DefaultInferenceOptions().Model,
				SourceAPI: a.sourceAPI,
			}},
		})
		if err != nil {
			return fmt.Errorf("auto-detect llmadapter providers: %w", err)
		}
		a.client = result.Client
		a.autoResult = result
	}
	a.resolveRouteIdentity()
	runtimeAgent, err := agentruntime.New(a.client, a.runtimeOptions()...)
	if err != nil {
		return err
	}
	a.runtime = runtimeAgent
	return nil
}

func (a *Agent) runtimeOptions() []agentruntime.Option {
	opts := []agentruntime.Option{
		agentruntime.WithSessionOptions(conversation.WithSessionID(conversation.SessionID(a.sessionID))),
		agentruntime.WithModel(a.inference.Model),
		agentruntime.WithMaxOutputTokens(a.inference.MaxTokens),
		agentruntime.WithTemperature(a.inference.Temperature),
		agentruntime.WithSystem(BuildSystemPrompt(a.workspace, a.systemOverride)),
		agentruntime.WithTools(a.activation.ActiveTools()),
		agentruntime.WithToolChoice(unified.ToolChoice{Mode: unified.ToolChoiceAuto}),
		agentruntime.WithMaxSteps(a.maxSteps),
		agentruntime.WithToolTimeout(a.toolTimeout),
		agentruntime.WithProviderIdentity(a.providerIdentity),
		agentruntime.WithToolContextFactory(func(ctx context.Context) acoreTool.Ctx {
			return a.newToolCtx(ctx)
		}),
	}
	if reasoning, ok := a.reasoningConfig(); ok {
		opts = append(opts, agentruntime.WithReasoning(reasoning))
	}
	return opts
}

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

// setupTools initializes all tools from agentsdk packages.
func (a *Agent) setupTools(workspace string, toolTimeout time.Duration) {
	a.allTools = standard.Tools(standard.Options{
		WebSearchProvider:     web.DefaultSearchProviderFromEnv(),
		IncludeToolManagement: true,
	})
	a.activation = NewActivationManager(a.allTools)
}

// SessionID returns the current session identifier.
func (a *Agent) SessionID() string { return a.sessionID }

// Tracker returns the usage tracker for session-level reporting.
func (a *Agent) Tracker() *coreusage.Tracker { return a.tracker }

// Out returns the output writer.
func (a *Agent) Out() io.Writer { return a.out }

// ParamsSummary returns a short human-readable summary of the active model parameters.
func (a *Agent) ParamsSummary() string {
	if a.resolvedProvider != "" || a.resolvedModel != "" {
		return fmt.Sprintf("model: %s  resolved_instance: %s  resolved_model: %s  thinking: %s  effort: %s", a.inference.Model, a.resolvedProvider, a.resolvedModel, a.inference.Thinking, a.inference.Effort)
	}
	return fmt.Sprintf("model: %s  thinking: %s  effort: %s", a.inference.Model, a.inference.Thinking, a.inference.Effort)
}

// Reset clears conversation history, usage tracker, and generates a new session ID.
func (a *Agent) Reset() {
	a.tracker.Reset()
	a.sessionID, _ = nanoid.Generate("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789", 8)
	if a.runtime != nil {
		a.runtime.ResetSession(conversation.WithSessionID(conversation.SessionID(a.sessionID)))
	}
}

// ErrMaxStepsReached is returned by RunTurn when the step loop is exhausted.
var ErrMaxStepsReached = errors.New("maximum steps reached - task may be incomplete")

// RunTurn executes one REPL turn (or one-shot task).
func (a *Agent) RunTurn(ctx context.Context, turnID int, task string) error {
	if a.verbose {
		display.PrintResolvedModel(a.out, fmt.Sprintf("input=%s  instance=%s  resolved=%s", a.inference.Model, a.resolvedProvider, a.resolvedModel))
	}

	handler := a.newRunnerEventHandler(turnID)
	_, err := a.runtime.RunTurn(
		ctx,
		task,
		agentruntime.WithTurnMaxSteps(a.maxSteps),
		agentruntime.WithTurnTools(a.activation.ActiveTools()),
		agentruntime.WithTurnProviderIdentity(a.providerIdentity),
		agentruntime.WithTurnEventHandler(handler.handle),
	)
	if handler.stepsCompleted > 1 {
		display.PrintTurnUsage(a.out, turnID, a.aggregateTurn(turnID))
	}
	if errors.Is(err, runner.ErrMaxStepsReached) {
		return ErrMaxStepsReached
	}
	if err != nil {
		return fmt.Errorf("provider=%s model=%s: %w", a.providerName(), a.resolvedModel, err)
	}
	return nil
}

func (a *Agent) reasoningConfig() (unified.ReasoningConfig, bool) {
	switch a.inference.Thinking {
	case ThinkingModeOff:
		return unified.ReasoningConfig{}, false
	case ThinkingModeAuto, "":
		return unified.ReasoningConfig{Effort: a.inference.Effort}, true
	default:
		return unified.ReasoningConfig{Effort: a.inference.Effort, Expose: true}, true
	}
}
