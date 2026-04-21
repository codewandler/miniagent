package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/codewandler/agentapis/api/unified"
	"github.com/codewandler/agentapis/conversation"
	"github.com/codewandler/agentcore/interfaces"
	acoreTool "github.com/codewandler/agentcore/tool"
	"github.com/codewandler/agentcore/tools/filesystem"
	"github.com/codewandler/agentcore/tools/shell"
	"github.com/codewandler/agentcore/tools/toolmgmt"
	"github.com/codewandler/agentcore/tools/web"
	llmproviders "github.com/codewandler/llmproviders"
	"github.com/codewandler/llmproviders/registry"
	"github.com/codewandler/miniagent/agent/usage"
	nanoid "github.com/matoous/go-nanoid/v2"
)

// Agent runs an agentic loop: model → tools → model → tools → ...
// A single Agent instance is reused across REPL turns; conversation history
// and usage records accumulate across turns.
type Agent struct {
	service        *llmproviders.Service
	provider       registry.Provider
	session        *conversation.Session
	tracker        *usage.Tracker
	allTools       []acoreTool.Tool
	activation     *ActivationManager
	inference      InferenceOptions
	maxSteps       int
	out            io.Writer
	workspace      string
	toolTimeout    time.Duration
	systemOverride string
	sessionID      string
}

// Option configures the Agent.
type Option func(*Agent)

// InferenceOption configures InferenceOptions.
type InferenceOption func(*InferenceOptions)

// InferenceOptions holds the model/inference parameters used for each LLM call.
type InferenceOptions struct {
	Model       string
	MaxTokens   int
	Thinking    unified.ThinkingMode
	Effort      unified.Effort
	Temperature float64
}

// DefaultInferenceOptions returns the default inference settings.
func DefaultInferenceOptions() InferenceOptions {
	return InferenceOptions{
		Model:       "codex/gpt-5.4",
		MaxTokens:   16_000,
		Thinking:    unified.ThinkingModeOn,
		Effort:      unified.EffortMedium,
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
func WithThinking(m unified.ThinkingMode) InferenceOption {
	return func(o *InferenceOptions) { o.Thinking = m }
}

// WithEffort sets the effort level.
func WithEffort(e unified.Effort) InferenceOption { return func(o *InferenceOptions) { o.Effort = e } }

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

// New creates an Agent. All settings are configurable via Options.
func New(service *llmproviders.Service, opts ...Option) *Agent {
	sessionID, _ := nanoid.Generate("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789", 8)
	a := &Agent{
		service:     service,
		inference:   DefaultInferenceOptions(),
		maxSteps:    30,
		out:         os.Stdout,
		toolTimeout: 30 * time.Second,
		sessionID:   sessionID,
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

	a.tracker = usage.NewTracker()
	a.setupTools(a.workspace, a.toolTimeout)
	if err := a.initSession(); err != nil {
		panic(fmt.Sprintf("failed to initialize agent session: %v", err))
	}
	return a
}

func (a *Agent) initSession() error {
	provider, resolvedModel, err := a.service.ProviderFor(a.inference.Model)
	if err != nil {
		return fmt.Errorf("failed to get provider for model %q: %w", a.inference.Model, err)
	}
	a.provider = provider

	prompt := BuildSystemPrompt(a.workspace, a.systemOverride)
	a.session = provider.CreateSession(
		conversation.WithModel(resolvedModel),
		conversation.WithMaxTokens(a.inference.MaxTokens),
		conversation.WithTemperature(a.inference.Temperature),
		conversation.WithThinking(a.inference.Thinking),
		conversation.WithEffort(a.inference.Effort),
		conversation.WithSystem(prompt),
		conversation.WithCapabilities(conversation.Capabilities{}),
	)
	return nil
}

// setupTools initializes all tools from agentcore packages.
func (a *Agent) setupTools(workspace string, toolTimeout time.Duration) {
	var allTools []acoreTool.Tool
	allTools = append(allTools, shell.Tools()...)
	allTools = append(allTools, filesystem.Tools()...)
	allTools = append(allTools, web.Tools(web.DefaultSearchProviderFromEnv())...)
	a.allTools = allTools
	a.activation = NewActivationManager(allTools)
	tmTools := toolmgmt.Tools()
	a.allTools = append(a.allTools, tmTools...)
	a.activation.allTools = a.allTools
}

// SessionID returns the current session identifier.
func (a *Agent) SessionID() string { return a.sessionID }

// Tracker returns the usage tracker for session-level reporting.
func (a *Agent) Tracker() *usage.Tracker { return a.tracker }

// Out returns the output writer.
func (a *Agent) Out() io.Writer { return a.out }

// ParamsSummary returns a short human-readable summary of the active model parameters.
func (a *Agent) ParamsSummary() string {
	return fmt.Sprintf("model: %s  thinking: %s  effort: %s", a.inference.Model, a.inference.Thinking, a.inference.Effort)
}

// Reset clears conversation history, usage tracker, and generates a new session ID.
func (a *Agent) Reset() {
	a.session.Reset()
	a.tracker.Reset()
	a.sessionID, _ = nanoid.Generate("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789", 8)
	if err := a.initSession(); err != nil {
		panic(fmt.Sprintf("failed to reset agent session: %v", err))
	}
}

// ErrMaxStepsReached is returned by RunTurn when the step loop is exhausted.
var ErrMaxStepsReached = errors.New("maximum steps reached — task may be incomplete")

// RunTurn executes one REPL turn (or one-shot task).
func (a *Agent) RunTurn(ctx context.Context, turnID int, task string) error {
	req := conversation.NewRequest().
		MaxTokens(a.inference.MaxTokens).
		Temperature(a.inference.Temperature).
		Thinking(a.inference.Thinking).
		Effort(a.inference.Effort).
		Tools(a.activeToolSpecs()).
		ToolChoice(unified.ToolChoiceAuto{}).
		User(task).
		Build()

	var stepsCompleted int
	for step := 1; step <= a.maxSteps; step++ {
		nextReq, done, err := a.runStep(ctx, turnID, step, &stepsCompleted, req)
		if err != nil {
			return err
		}
		if done {
			if stepsCompleted > 1 {
				printTurnUsage(a.out, turnID, a.aggregateTurn(turnID))
			}
			return nil
		}
		req = nextReq
	}
	if stepsCompleted > 1 {
		printTurnUsage(a.out, turnID, a.aggregateTurn(turnID))
	}
	return ErrMaxStepsReached
}

func (a *Agent) runStep(ctx context.Context, turnID, step int, stepsCompleted *int, req conversation.Request) (conversation.Request, bool, error) {
	printStepHeader(a.out, step, a.maxSteps)
	stream, err := a.session.Request(ctx, req)
	if err != nil {
		return conversation.Request{}, false, fmt.Errorf("request conversation stream: %w", err)
	}

	sd := newStepDisplay(a.out)
	var toolCalls []unified.ToolCall
	var stepUsage usage.Record
	var sawCompleted bool
	var stopReason unified.StopReason
	for ev := range stream {
		switch e := ev.(type) {
		case conversation.TextDeltaEvent:
			sd.WriteText(e.Text)
		case conversation.ReasoningDeltaEvent:
			sd.WriteReasoning(e.Text)
		case conversation.ToolCallEvent:
			toolCalls = append(toolCalls, e.ToolCall)
			sd.PrintToolCall(e.ToolCall.Name, e.ToolCall.Args)
		case conversation.UsageEvent:
			rec := a.recordTransportUsage(turnID, e.Usage)
			a.tracker.Record(rec)
			stepUsage = mergeUsageRecord(stepUsage, rec)
		case conversation.CompletedEvent:
			sawCompleted = true
			stopReason = e.StopReason
		case conversation.ErrorEvent:
			sd.End()
			if e.Err != nil {
				return conversation.Request{}, false, e.Err
			}
			return conversation.Request{}, false, errors.New("stream error")
		}
	}
	sd.End()
	if ctx.Err() != nil {
		return conversation.Request{}, false, ctx.Err()
	}
	printStepUsage(a.out, step, stepUsage, "")
	if !sawCompleted {
		return conversation.Request{}, false, errors.New("stream error")
	}
	*stepsCompleted++
	if len(toolCalls) == 0 {
		if stopReason == unified.StopReasonMaxTokens {
			fmt.Fprintf(a.out, "\n%s⚠ model hit output token limit%s\n", ansiBrightYellow, ansiReset)
		}
		return conversation.Request{}, true, nil
	}
	followup := conversation.NewRequest().
		MaxTokens(a.inference.MaxTokens).
		Temperature(a.inference.Temperature).
		Thinking(a.inference.Thinking).
		Effort(a.inference.Effort).
		Tools(a.activeToolSpecs()).
		ToolChoice(unified.ToolChoiceAuto{})
	for _, tc := range toolCalls {
		output, isError := a.executeTool(ctx, tc)
		printToolResult(a.out, output, isError)
		followup.ToolResult(tc.ID, output)
	}
	return followup.Build(), false, nil
}

func (a *Agent) executeTool(ctx context.Context, tc unified.ToolCall) (string, bool) {
	for _, t := range a.activation.ActiveTools() {
		if t.Name() != tc.Name {
			continue
		}
		input, _ := json.Marshal(tc.Args)
		toolCtx := &agentcoreToolContext{ctx: ctx, workspace: a.workspace, activation: a.activation, extra: make(map[string]any), sessionID: a.sessionID}
		toolCtx.extra[toolmgmt.KeyActivationState] = a.activation
		result, err := t.Execute(toolCtx, input)
		if err != nil {
			return err.Error(), true
		}
		return result.String(), result.IsError()
	}
	return fmt.Sprintf("tool not found: %s", tc.Name), true
}

func (a *Agent) activeToolSpecs() []unified.Tool {
	active := a.activation.ActiveTools()
	out := make([]unified.Tool, 0, len(active))
	for _, t := range active {
		out = append(out, convertUnifiedToolDefinition(t))
	}
	return out
}

func (a *Agent) recordTransportUsage(turnID int, u unified.StreamUsage) usage.Record {
	providerName, modelName := a.providerAndModel(u)
	items := unifiedToUsageTokens(u.Tokens)
	rec := usage.Record{
		Tokens: items,
		Dims: usage.Dims{
			Provider:  providerName,
			Model:     modelName,
			RequestID: u.RequestID,
			TurnID:    strconv.Itoa(turnID),
			SessionID: a.sessionID,
		},
		RecordedAt: time.Now(),
	}
	return rec
}

func (a *Agent) providerAndModel(u unified.StreamUsage) (string, string) {
	providerName := ""
	model := a.inference.Model
	if a.provider != nil {
		providerName = a.provider.Name()
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

func unifiedToUsageTokens(items unified.TokenItems) usage.TokenItems {
	var out usage.TokenItems
	for _, item := range items {
		switch item.Kind {
		case unified.TokenKindInputNew:
			out = append(out, usage.TokenItem{Kind: usage.KindInput, Count: item.Count})
		case unified.TokenKindInputCacheRead:
			out = append(out, usage.TokenItem{Kind: usage.KindCacheRead, Count: item.Count})
		case unified.TokenKindInputCacheWrite:
			out = append(out, usage.TokenItem{Kind: usage.KindCacheWrite, Count: item.Count})
		case unified.TokenKindOutput:
			out = append(out, usage.TokenItem{Kind: usage.KindOutput, Count: item.Count})
		case unified.TokenKindOutputReasoning:
			out = append(out, usage.TokenItem{Kind: usage.KindReasoning, Count: item.Count})
		}
	}
	return out.NonZero()
}

func mergeUsageRecord(dst, src usage.Record) usage.Record {
	if dst.RecordedAt.IsZero() {
		dst.RecordedAt = src.RecordedAt
	}
	counts := map[usage.TokenKind]int{}
	for _, r := range []usage.Record{dst, src} {
		for _, item := range r.Tokens {
			counts[item.Kind] += item.Count
		}
		dst.Cost.Total += r.Cost.Total
		dst.Cost.Input += r.Cost.Input
		dst.Cost.Output += r.Cost.Output
		dst.Cost.Reasoning += r.Cost.Reasoning
		dst.Cost.CacheRead += r.Cost.CacheRead
		dst.Cost.CacheWrite += r.Cost.CacheWrite
		if dst.Cost.Source == "" {
			dst.Cost.Source = r.Cost.Source
		}
		if dst.Dims.Provider == "" {
			dst.Dims = r.Dims
		}
	}
	dst.Tokens = nil
	for kind, count := range counts {
		if count > 0 {
			dst.Tokens = append(dst.Tokens, usage.TokenItem{Kind: kind, Count: count})
		}
	}
	return dst
}

// aggregateTurn sums all usage records for a given turn ID.
func (a *Agent) aggregateTurn(turnID int) usage.Record {
	recs := a.tracker.Filter(usage.ByTurnID(strconv.Itoa(turnID)), usage.ExcludeEstimates())
	var agg usage.Record
	counts := make(map[usage.TokenKind]int)
	for _, r := range recs {
		for _, item := range r.Tokens {
			counts[item.Kind] += item.Count
		}
		agg.Cost.Total += r.Cost.Total
		agg.Cost.Input += r.Cost.Input
		agg.Cost.Output += r.Cost.Output
		agg.Cost.Reasoning += r.Cost.Reasoning
		agg.Cost.CacheRead += r.Cost.CacheRead
		agg.Cost.CacheWrite += r.Cost.CacheWrite
	}
	for kind, count := range counts {
		agg.Tokens = append(agg.Tokens, usage.TokenItem{Kind: kind, Count: count})
	}
	return agg
}

func convertUnifiedToolDefinition(t acoreTool.Tool) unified.Tool {
	schema := t.Schema()
	raw, _ := json.Marshal(schema)
	var params map[string]any
	_ = json.Unmarshal(raw, &params)
	delete(params, "$schema")
	delete(params, "$id")
	return unified.Tool{Name: t.Name(), Description: t.Description(), Parameters: params, Strict: true}
}

type agentcoreToolContext struct {
	ctx        context.Context
	workspace  string
	activation interfaces.ActivationState
	extra      map[string]any
	sessionID  string
}

func (c *agentcoreToolContext) WorkDir() string       { return c.workspace }
func (c *agentcoreToolContext) Extra() map[string]any { return c.extra }
func (c *agentcoreToolContext) Deadline() (time.Time, bool) {
	if c.ctx == nil {
		return time.Time{}, false
	}
	return c.ctx.Deadline()
}
func (c *agentcoreToolContext) Done() <-chan struct{} {
	if c.ctx == nil {
		return nil
	}
	return c.ctx.Done()
}
func (c *agentcoreToolContext) Err() error {
	if c.ctx == nil {
		return nil
	}
	return c.ctx.Err()
}
func (c *agentcoreToolContext) Value(key any) any {
	if c.ctx == nil {
		return nil
	}
	return c.ctx.Value(key)
}
func (c *agentcoreToolContext) AgentID() string   { return "" }
func (c *agentcoreToolContext) SessionID() string { return c.sessionID }
