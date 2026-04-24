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
	autoResult       adapterconfig.AutoResult
	providerIdentity conversation.ProviderIdentity
	resolvedProvider string
	resolvedModel    string
	sourceAPI        adapt.ApiKind
	session          *conversation.Session
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
		result, err := adapterconfig.AutoMuxClient(adapterconfig.AutoOptions{
			EnableEnv:         true,
			EnableLocalClaude: true,
			UseModelDB:        true,
			SourceAPI:         a.sourceAPI,
			Intents: []adapterconfig.AutoIntent{{
				Name:      a.inference.Model,
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
	a.session = a.newSession()
	return nil
}

func (a *Agent) newSession() *conversation.Session {
	opts := []conversation.Option{
		conversation.WithSessionID(conversation.SessionID(a.sessionID)),
		conversation.WithModel(a.inference.Model),
		conversation.WithMaxOutputTokens(a.inference.MaxTokens),
		conversation.WithTemperature(a.inference.Temperature),
		conversation.WithSystem(BuildSystemPrompt(a.workspace, a.systemOverride)),
		conversation.WithTools(acoreTool.UnifiedToolsFrom(a.activation.ActiveTools())),
		conversation.WithToolChoice(unified.ToolChoice{Mode: unified.ToolChoiceAuto}),
	}
	if reasoning, ok := a.reasoningConfig(); ok {
		opts = append(opts, conversation.WithReasoning(reasoning))
	}
	return conversation.New(opts...)
}

func (a *Agent) resolveRouteIdentity() {
	a.providerIdentity = conversation.ProviderIdentity{}
	a.resolvedProvider = ""
	a.resolvedModel = ""
	if len(a.autoResult.Config.Routes) == 0 {
		return
	}
	for _, route := range a.autoResult.Config.Routes {
		if route.SourceAPI != a.sourceAPI {
			continue
		}
		if route.Model != "" && route.Model != a.inference.Model {
			continue
		}
		a.resolvedProvider = route.Provider
		a.resolvedModel = route.NativeModel
		a.providerIdentity = conversation.ProviderIdentity{
			ProviderName: route.Provider,
			APIKind:      string(route.ProviderAPI),
			NativeModel:  route.NativeModel,
		}
		if a.resolvedModel == "" {
			a.resolvedModel = route.Model
		}
		return
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
	a.session = a.newSession()
}

// ErrMaxStepsReached is returned by RunTurn when the step loop is exhausted.
var ErrMaxStepsReached = errors.New("maximum steps reached - task may be incomplete")

// RunTurn executes one REPL turn (or one-shot task).
func (a *Agent) RunTurn(ctx context.Context, turnID int, task string) error {
	reqBuilder := conversation.NewRequest().
		Model(a.inference.Model).
		MaxOutputTokens(a.inference.MaxTokens).
		Temperature(a.inference.Temperature).
		Tools(acoreTool.UnifiedToolsFrom(a.activation.ActiveTools())).
		ToolChoice(unified.ToolChoice{Mode: unified.ToolChoiceAuto}).
		User(task).
		Stream(true)
	if reasoning, ok := a.reasoningConfig(); ok {
		reqBuilder.Reasoning(reasoning)
	}
	req := reqBuilder.Build()

	if a.verbose {
		display.PrintResolvedModel(a.out, fmt.Sprintf("input=%s  instance=%s  resolved=%s", a.inference.Model, a.resolvedProvider, a.resolvedModel))
	}

	handler := a.newRunnerEventHandler(turnID)
	_, err := runner.RunTurn(
		ctx,
		a.session,
		a.client,
		req,
		runner.WithMaxSteps(a.maxSteps),
		runner.WithTools(a.activation.ActiveTools()),
		runner.WithToolCtx(a.newToolCtx(ctx)),
		runner.WithToolTimeout(a.toolTimeout),
		runner.WithProviderIdentity(a.providerIdentity),
		runner.WithEventHandler(handler.handle),
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
