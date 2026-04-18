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
	"time"

	"github.com/codewandler/agentcore/interfaces"
	acoreTool "github.com/codewandler/agentcore/tool"
	"github.com/codewandler/agentcore/tools/filesystem"
	"github.com/codewandler/agentcore/tools/shell"
	"github.com/codewandler/agentcore/tools/toolmgmt"
	"github.com/codewandler/llm"
	"github.com/codewandler/llm/msg"
	"github.com/codewandler/llm/tool"
	"github.com/codewandler/llm/usage"
	nanoid "github.com/matoous/go-nanoid/v2"
)

// Agent runs an agentic loop: LLM → tools → LLM → tools → ...
// A single Agent instance is reused across REPL turns; conversation history
// and usage records accumulate across turns.
type Agent struct {
	provider        llm.Provider
	messages        msg.Messages
	tracker         *usage.Tracker
	initialMessages msg.Messages
	toolDefs        []tool.Definition
	allTools        []acoreTool.Tool
	activation      *ActivationManager
	inference       InferenceOptions
	maxSteps        int
	out             io.Writer
	workspace       string
	toolTimeout     time.Duration
	systemOverride  string
	sessionID       string
}

// Option configures the Agent.
type Option func(*Agent)

// InferenceOption configures InferenceOptions.
type InferenceOption func(*InferenceOptions)

// InferenceOptions holds the model/inference parameters used for each LLM call.
type InferenceOptions struct {
	Model       string
	MaxTokens   int
	Thinking    llm.ThinkingMode
	Effort      llm.Effort
	Temperature float64
}

// DefaultInferenceOptions returns the default inference settings.
func DefaultInferenceOptions() InferenceOptions {
	return InferenceOptions{
		Model:       "codex/gpt-5.4",
		MaxTokens:   16_000,
		Thinking:    llm.ThinkingOn,
		Effort:      llm.EffortMedium,
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
func WithModel(m string) InferenceOption {
	return func(o *InferenceOptions) { o.Model = m }
}

// WithMaxTokens sets the maximum output tokens per LLM call.
func WithMaxTokens(n int) InferenceOption {
	return func(o *InferenceOptions) { o.MaxTokens = n }
}

// WithThinking sets the thinking mode.
func WithThinking(m llm.ThinkingMode) InferenceOption {
	return func(o *InferenceOptions) { o.Thinking = m }
}

// WithEffort sets the effort level.
func WithEffort(e llm.Effort) InferenceOption {
	return func(o *InferenceOptions) { o.Effort = e }
}

// WithTemperature sets the sampling temperature.
func WithTemperature(t float64) InferenceOption {
	return func(o *InferenceOptions) { o.Temperature = t }
}

// WithInferenceOptions sets all inference options at once.
func WithInferenceOptions(opts InferenceOptions) Option {
	return func(a *Agent) { a.inference = opts }
}

// WithMaxSteps sets the maximum agent loop iterations per turn (default: 30).
func WithMaxSteps(n int) Option { return func(a *Agent) { a.maxSteps = n } }

// WithOutput sets the output writer (default: os.Stdout).
// Tests pass a *bytes.Buffer to capture and suppress output.
func WithOutput(w io.Writer) Option { return func(a *Agent) { a.out = w } }

// WithWorkspace sets the working directory (default: current working directory).
func WithWorkspace(dir string) Option { return func(a *Agent) { a.workspace = dir } }

// WithToolTimeout sets the per-tool call timeout (default: 30s).
func WithToolTimeout(d time.Duration) Option { return func(a *Agent) { a.toolTimeout = d } }

// WithSystemOverride sets a custom system prompt body (default: built from workspace).
func WithSystemOverride(prompt string) Option { return func(a *Agent) { a.systemOverride = prompt } }

// New creates an Agent. All settings are configurable via Options.
// Defaults: workspace = cwd, toolTimeout = 30s, maxSteps = 30.
func New(provider llm.Provider, opts ...Option) *Agent {
	sessionID, _ := nanoid.Generate("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789", 8)
	a := &Agent{
		provider:    provider,
		inference:   DefaultInferenceOptions(),
		maxSteps:    30,
		out:         os.Stdout,
		toolTimeout: 30 * time.Second,
		sessionID:   sessionID,
	}
	for _, o := range opts {
		o(a)
	}

	// Resolve workspace to absolute path
	ws := a.workspace
	if ws == "" {
		ws, _ = os.Getwd()
	}
	ws, _ = filepath.Abs(ws)
	a.workspace = ws

	a.tracker = usage.NewTracker(
		usage.WithCostCalculator(usage.Default()),
	)

	// System prompt with cache hint for REPL efficiency
	prompt := BuildSystemPrompt(ws, a.systemOverride)
	initMsg := msg.Messages{
		msg.System(prompt).Cache(msg.CacheTTL1h).Build(),
	}
	a.initialMessages = initMsg
	a.messages = initMsg

	// Build agentcore tools
	a.setupTools(a.workspace, a.toolTimeout)

	return a
}

// setupTools initializes all tools from agentcore packages
func (a *Agent) setupTools(workspace string, toolTimeout time.Duration) {
	// Collect all tools
	var allTools []acoreTool.Tool

	// Shell tools (bash)
	shellTools := shell.Tools()
	allTools = append(allTools, shellTools...)

	// Filesystem tools
	fsTools := filesystem.Tools()
	allTools = append(allTools, fsTools...)

	a.allTools = allTools

	// Create activation manager
	a.activation = NewActivationManager(allTools)

	// Add toolmgmt tools now that we have the activation manager
	tmTools := toolmgmt.Tools()
	a.allTools = append(a.allTools, tmTools...)
	a.activation.allTools = a.allTools

	// Convert agentcore tools to llm/tool definitions
	for _, t := range a.allTools {
		a.toolDefs = append(a.toolDefs, convertToolDefinition(t))
	}
}

// SessionID returns the current session identifier.
func (a *Agent) SessionID() string { return a.sessionID }

// Tracker returns the usage tracker for session-level reporting.
func (a *Agent) Tracker() *usage.Tracker { return a.tracker }

// Out returns the output writer (for REPL to write to the same destination).
func (a *Agent) Out() io.Writer { return a.out }

// ParamsSummary returns a short human-readable summary of the active model
// parameters for display before the REPL prompt.
func (a *Agent) ParamsSummary() string {
	return fmt.Sprintf("model: %s  thinking: %s  effort: %s", a.inference.Model, a.inference.Thinking, a.inference.Effort)
}

// Reset clears conversation history back to the initial system prompt,
// starts a fresh usage tracker, and generates a new session ID.
// Called by the REPL /new command.
func (a *Agent) Reset() {
	a.messages = a.initialMessages
	a.tracker = usage.NewTracker(usage.WithCostCalculator(usage.Default()))
	a.sessionID, _ = nanoid.Generate("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789", 8)
}

// ErrMaxStepsReached is returned by RunTurn when the step loop is exhausted
// before the model produced a tool-free response. Partial output may have been
// produced. Callers can inspect this with errors.Is.
var ErrMaxStepsReached = errors.New("maximum steps reached — task may be incomplete")

// RunTurn executes one REPL turn (or one-shot task). Appends a user message,
// runs the step loop, and returns nil on success.
func (a *Agent) RunTurn(ctx context.Context, turnID int, task string) error {
	// Snapshot for rollback on error (see DESIGN §History rollback)
	snapshot := len(a.messages)
	rollback := func() { a.messages = a.messages[:snapshot] }

	a.messages = a.messages.Append(msg.User(task).Build())

	var stepsCompleted int

	for step := 1; step <= a.maxSteps; step++ {
		// runStep returns (done, error) — no errContinue sentinel.
		done, err := a.runStep(ctx, turnID, step, &stepsCompleted)
		if err != nil {
			// always rollback inside the loop.
			// Every error from runStep leaves history in an invalid
			// alternating-role state. errMaxStepsReached is only
			// returned AFTER the loop (no rollback needed there).
			rollback()
			return err
		}
		if done {
			if stepsCompleted > 1 {
				turnRec := a.aggregateTurn(turnID)
				printTurnUsage(a.out, turnID, turnRec)
			}
			return nil
		}
		// done=false, err=nil → model called tools, continue to next step
	}

	// Loop exhausted — no rollback (history ends with assistant message = valid state)
	if stepsCompleted > 1 {
		turnRec := a.aggregateTurn(turnID)
		printTurnUsage(a.out, turnID, turnRec)
	}
	return ErrMaxStepsReached
}

// runStep executes one LLM call → tool dispatch cycle. Returns:
//   - (true, nil):   turn completed (StopReasonEndTurn or StopReasonMaxTokens)
//   - (false, nil):  model called tools, continue to next step
//   - (_, error):    error — caller should rollback
func (a *Agent) runStep(
	ctx context.Context,
	turnID int,
	step int,
	stepsCompleted *int,
) (done bool, err error) {
	printStepHeader(a.out, step, a.maxSteps)

	// Pass *RequestBuilder directly — it implements Buildable.
	// Provider calls BuildRequest() internally (validates + returns Request).
	// For multi-step turns, add a cache hint to the last accumulated message so
	// the growing conversation history is progressively cached. Anthropic and
	// Bedrock place a cache breakpoint at this position on the wire; on the first
	// step (only system + user present) the system prompt hint already handles it.
	messages := a.messages
	if n := len(messages); n > 1 && messages[n-1].CacheHint == nil {
		cp := make(msg.Messages, n)
		copy(cp, messages)
		cp[n-1].CacheHint = msg.NewCacheHint(msg.CacheTTL5m)
		messages = cp
	}

	rb := llm.NewRequestBuilder().
		Model(a.inference.Model).
		MaxTokens(a.inference.MaxTokens).
		Thinking(a.inference.Thinking).
		Effort(a.inference.Effort).
		Temperature(a.inference.Temperature).
		Append(messages...).
		Tools(a.toolDefs...)

	stream, err := a.provider.CreateStream(ctx, rb)
	if err != nil {
		return false, fmt.Errorf("create stream: %w", err)
	}

	// ── Stream processing with live callbacks ──

	sd := newStepDisplay(a.out)
	var stepUsage usage.Record
	var resolvedModel string

	// Create tool handlers for all agentcore tools
	toolHandlers := a.createToolHandlers()

	result := llm.NewEventProcessor(ctx, stream).
		OnEvent(llm.TypedEventHandler[*llm.StreamStartedEvent](func(ev *llm.StreamStartedEvent) {
			if ev.Model != "" {
				resolvedModel = ev.Model
			}
		})).
		OnEvent(llm.TypedEventHandler[*llm.ModelResolvedEvent](func(ev *llm.ModelResolvedEvent) {
			if ev.Resolved != "" {
				resolvedModel = ev.Resolved
			}
		})).
		OnReasoningDelta(func(chunk string) {
			sd.WriteReasoning(chunk)
		}).
		OnTextDelta(func(chunk string) {
			sd.WriteText(chunk)
		}).
		OnEvent(llm.TypedEventHandler[*llm.ToolCallEvent](func(ev *llm.ToolCallEvent) {
			tc := ev.ToolCall
			sd.PrintToolCall(tc.ToolName(), tc.ToolArgs())
		})).
		OnEvent(llm.TypedEventHandler[*llm.UsageUpdatedEvent](func(ev *llm.UsageUpdatedEvent) {
			rec := ev.Record
			rec.Dims.TurnID = strconv.Itoa(turnID)
			a.tracker.Record(rec)
			stepUsage = rec
		})).
		HandleTool(toolHandlers...).
		Result()

	sd.End()

	// ── Display tool results ──
	for _, tr := range result.ToolResults() {
		output := extractBashOutput(tr.ToolOutput())
		printToolResult(a.out, output, tr.IsError())
	}

	// ── Per-step usage ──

	printStepUsage(a.out, step, stepUsage, resolvedModel)

	// ── Branch on stop reason (error paths return before appending to history) ──

	switch result.StopReason() {
	case llm.StopReasonCancelled:
		return false, context.Canceled

	case llm.StopReasonError:
		if rerr := result.Error(); rerr != nil {
			return false, rerr
		}
		return false, errors.New("stream error")
	}

	// ── Append to conversation history (success and tool-use paths only) ──

	a.messages = a.messages.Append(result.Next())
	*stepsCompleted++

	switch result.StopReason() {
	case llm.StopReasonToolUse:
		return false, nil // continue to next step

	case llm.StopReasonMaxTokens:
		fmt.Fprintf(a.out, "\n%s⚠ model hit output token limit%s\n", ansiBrightYellow, ansiReset)
		return true, nil // partial but usable

	default: // StopReasonEndTurn and others
		return true, nil // success
	}
}

// createToolHandlers creates handlers for all agentcore tools
func (a *Agent) createToolHandlers() []tool.NamedHandler {
	var handlers []tool.NamedHandler

	for _, t := range a.allTools {
		toolCopy := t // capture for closure
		handlers = append(handlers, tool.NewHandler[json.RawMessage, interface{}](
			toolCopy.Name(),
			func(ctx context.Context, input json.RawMessage) (*interface{}, error) {
				// Create agentcore context with activation state, wrapping the caller's ctx
				toolCtx := &agentcoreToolContext{
					ctx:        ctx,
					workspace:  a.workspace,
					activation: a.activation,
					extra:      make(map[string]interface{}),
				}
				toolCtx.extra[toolmgmt.KeyActivationState] = a.activation

				// Execute the agentcore tool
				result, err := toolCopy.Execute(toolCtx, input)
				if err != nil {
					return nil, err
				}

				// Convert result to interface{} and return as pointer
				output := interface{}(result.String())
				return &output, nil
			},
		))
	}

	return handlers
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

// convertToolDefinition converts an agentcore tool to an llm/tool Definition
func convertToolDefinition(t acoreTool.Tool) tool.Definition {
	// Get the schema from the agentcore tool and convert it to map[string]any
	schema := t.Schema()

	// Marshal and unmarshal to get clean map[string]any
	raw, _ := json.Marshal(schema)
	var params map[string]any
	_ = json.Unmarshal(raw, &params)

	// Clean up metadata fields
	delete(params, "$schema")
	delete(params, "$id")

	return tool.Definition{
		Name:        t.Name(),
		Description: t.Description(),
		Parameters:  params,
	}
}

// agentcoreToolContext implements acoreTool.Ctx for agentcore tools
type agentcoreToolContext struct {
	ctx        context.Context
	workspace  string
	activation interfaces.ActivationState
	extra      map[string]interface{}
}

func (c *agentcoreToolContext) WorkDir() string {
	return c.workspace
}

func (c *agentcoreToolContext) Extra() map[string]interface{} {
	return c.extra
}

// Deadline and Done implement context.Context
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

func (c *agentcoreToolContext) Value(key interface{}) interface{} {
	if c.ctx == nil {
		return nil
	}
	return c.ctx.Value(key)
}

// AgentID and SessionID implement agentcore/tool.Ctx
func (c *agentcoreToolContext) AgentID() string {
	return ""
}

func (c *agentcoreToolContext) SessionID() string {
	return ""
}
