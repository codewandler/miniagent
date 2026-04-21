package agent

import (
	"context"
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
	acoreTool "github.com/codewandler/agentsdk/tool"
	"github.com/codewandler/agentsdk/tools/filesystem"
	"github.com/codewandler/agentsdk/tools/shell"
	"github.com/codewandler/agentsdk/tools/toolmgmt"
	"github.com/codewandler/agentsdk/tools/web"
	llmproviders "github.com/codewandler/llmproviders"
	"github.com/codewandler/llmproviders/registry"
	"github.com/codewandler/miniagent/agent/display"
	"github.com/codewandler/miniagent/agent/usage"
	nanoid "github.com/matoous/go-nanoid/v2"
)

// Agent runs an agentic loop: model → tools → model → tools → ...
// A single Agent instance is reused across REPL turns; conversation history
// and usage records accumulate across turns.
type Agent struct {
	service        *llmproviders.Service
	provider       registry.Provider
	resolvedModel  string
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
	verbose        bool
}

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
	a.resolvedModel = resolvedModel

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

// setupTools initializes all tools from agentsdk packages.
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
	providerName := ""
	if a.provider != nil {
		providerName = a.provider.Name()
	}
	if providerName != "" || a.resolvedModel != "" {
		return fmt.Sprintf("model: %s  resolved_instance: %s  resolved_model: %s  thinking: %s  effort: %s", a.inference.Model, providerName, a.resolvedModel, a.inference.Thinking, a.inference.Effort)
	}
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

	if a.verbose {
		providerName := ""
		if a.provider != nil {
			providerName = a.provider.Name()
		}
		display.PrintResolvedModel(a.out, fmt.Sprintf("input=%s  instance=%s  resolved=%s", a.inference.Model, providerName, a.resolvedModel))
	}

	var stepsCompleted int
	for step := 1; step <= a.maxSteps; step++ {
		nextReq, done, err := a.runStep(ctx, turnID, step, &stepsCompleted, req)
		if err != nil {
			return err
		}
		if done {
			if stepsCompleted > 1 {
				display.PrintTurnUsage(a.out, turnID, a.aggregateTurn(turnID))
			}
			return nil
		}
		req = nextReq
	}
	if stepsCompleted > 1 {
		display.PrintTurnUsage(a.out, turnID, a.aggregateTurn(turnID))
	}
	return ErrMaxStepsReached
}

func (a *Agent) runStep(ctx context.Context, turnID, step int, stepsCompleted *int, req conversation.Request) (conversation.Request, bool, error) {
	display.PrintStepHeader(a.out, step, a.maxSteps)
	stream, err := a.session.Request(ctx, req)
	if err != nil {
		return conversation.Request{}, false, fmt.Errorf("request conversation stream: %w", err)
	}

	sd := display.NewStepDisplay(a.out)
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
				return conversation.Request{}, false, fmt.Errorf("provider=%s model=%s step=%d: %w", a.providerName(), a.resolvedModel, step, e.Err)
			}
			return conversation.Request{}, false, fmt.Errorf("provider=%s model=%s step=%d: stream error", a.providerName(), a.resolvedModel, step)
		}
	}
	sd.End()
	if ctx.Err() != nil {
		return conversation.Request{}, false, ctx.Err()
	}
	display.PrintStepUsage(a.out, step, stepUsage, "")
	if !sawCompleted {
		return conversation.Request{}, false, fmt.Errorf("provider=%s model=%s step=%d: stream ended without completed event (or provider returned an empty stream without terminal events)", a.providerName(), a.resolvedModel, step)
	}
	*stepsCompleted++
	if len(toolCalls) == 0 {
		if stopReason == unified.StopReasonMaxTokens {
			fmt.Fprintf(a.out, "\n%s⚠ model hit output token limit%s\n", display.BrightYellow, display.Reset)
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
		display.PrintToolResult(a.out, output, isError)
		followup.ToolResult(tc.ID, output)
	}
	return followup.Build(), false, nil
}

// ---------------------------------------------------------------------------
// Usage tracking helpers
// ---------------------------------------------------------------------------

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

func (a *Agent) providerName() string {
	if a.provider == nil {
		return ""
	}
	return a.provider.Name()
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
