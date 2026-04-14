package agent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/codewandler/llm"
	"github.com/codewandler/llm/msg"
	"github.com/codewandler/llm/tool"
	"github.com/codewandler/llm/usage"
)

// Agent runs an agentic loop: LLM → bash tool → LLM → bash tool → ...
// A single Agent instance is reused across REPL turns; conversation history
// and usage records accumulate across turns.
type Agent struct {
	provider    llm.Provider
	messages    msg.Messages
	tracker     *usage.Tracker
	toolDefs    []tool.Definition
	toolHandler tool.NamedHandler
	model       string
	maxSteps    int
	maxTokens   int
	out         io.Writer
}

// Option configures the Agent.
type Option func(*Agent)

// WithModel sets the model alias or full path (default: "default").
func WithModel(m string) Option { return func(a *Agent) { a.model = m } }

// WithMaxSteps sets the maximum agent loop iterations per turn (default: 30).
func WithMaxSteps(n int) Option { return func(a *Agent) { a.maxSteps = n } }

// WithMaxTokens sets the maximum output tokens per LLM call (default: 16000).
func WithMaxTokens(n int) Option { return func(a *Agent) { a.maxTokens = n } }

// WithOutput sets the output writer (default: os.Stdout).
// Tests pass a *bytes.Buffer to capture and suppress output.
func WithOutput(w io.Writer) Option { return func(a *Agent) { a.out = w } }

// New creates an Agent. workspace must be an absolute path to an existing
// directory. cmdTimeout limits each individual bash command.
func New(
	provider llm.Provider,
	workspace string,
	cmdTimeout time.Duration,
	systemOverride string,
	opts ...Option,
) *Agent {
	a := &Agent{
		provider:  provider,
		model:     "default",
		maxSteps:  30,
		maxTokens: 16_000,
		out:       os.Stdout,
	}
	for _, o := range opts {
		o(a)
	}

	a.tracker = usage.NewTracker(
		usage.WithCostCalculator(usage.Default()),
	)

	// System prompt with cache hint for REPL efficiency
	prompt := BuildSystemPrompt(workspace, systemOverride)
	a.messages = msg.Messages{
		msg.System(prompt).Cache(msg.CacheTTL1h).Build(),
	}

	a.toolDefs = []tool.Definition{BashDefinition()}
	a.toolHandler = NewBashHandler(workspace, cmdTimeout)

	return a
}

// Tracker returns the usage tracker for session-level reporting.
func (a *Agent) Tracker() *usage.Tracker { return a.tracker }

// Out returns the output writer (for REPL to write to the same destination).
func (a *Agent) Out() io.Writer { return a.out }

// ErrMaxStepsReached is returned by RunTurn when the step loop is exhausted
// before the model produced a tool-free response. Partial output may have been
// produced. Callers can inspect this with errors.Is.
var ErrMaxStepsReached = errors.New("maximum steps reached — task may be incomplete")

// RunTurn executes one REPL turn (or one-shot task). Appends a user message,
// runs the step loop, and returns nil on success.
func (a *Agent) RunTurn(ctx context.Context, turnID, task string) error {
	// Snapshot for rollback on error (see DESIGN §History rollback)
	snapshot := len(a.messages)
	rollback := func() { a.messages = a.messages[:snapshot] }

	a.messages = a.messages.Append(msg.User(task).Build())

	var stepsCompleted int

	for step := 1; step <= a.maxSteps; step++ {
		// [REVIEW FIX #5]: runStep returns (done, error) — no errContinue sentinel.
		done, err := a.runStep(ctx, turnID, step, &stepsCompleted)
		if err != nil {
			// [REVIEW FIX #4]: always rollback inside the loop.
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
	turnID string,
	step int,
	stepsCompleted *int,
) (done bool, err error) {
	printStepHeader(a.out, step, a.maxSteps)

	// Pass *RequestBuilder directly — it implements Buildable.
	// Provider calls BuildRequest() internally (validates + returns Request).
	rb := llm.NewRequestBuilder().
		Model(a.model).
		MaxTokens(a.maxTokens).
		Append(a.messages...).
		Tools(a.toolDefs...)

	stream, err := a.provider.CreateStream(ctx, rb)
	if err != nil {
		return false, fmt.Errorf("create stream: %w", err)
	}

	// ── Stream processing with live callbacks ──

	sd := newStepDisplay(a.out)
	var stepUsage usage.Record

	result := llm.NewEventProcessor(ctx, stream).
		OnReasoningDelta(func(chunk string) {
			sd.WriteReasoning(chunk)
		}).
		OnTextDelta(func(chunk string) {
			sd.WriteText(chunk)
		}).
		OnEvent(llm.TypedEventHandler[*llm.ToolCallEvent](func(ev *llm.ToolCallEvent) {
			tc := ev.ToolCall
			command, _ := tc.ToolArgs()["command"].(string)
			sd.PrintToolCall(tc.ToolName(), command)
		})).
		OnEvent(llm.TypedEventHandler[*llm.UsageUpdatedEvent](func(ev *llm.UsageUpdatedEvent) {
			rec := ev.Record
			rec.Dims.TurnID = turnID
			a.tracker.Record(rec)
			stepUsage = rec
		})).
		HandleTool(a.toolHandler).
		Result()

	sd.End()

	// ── Display tool results ──
	// [REVIEW FIX #2]: commands already shown live via ToolCallEvent.
	// Only show result lines here — no command duplication.
	for _, tr := range result.ToolResults() {
		output := extractBashOutput(tr.ToolOutput())
		printToolResult(a.out, output, tr.IsError())
	}

	// ── Per-step usage ──

	printStepUsage(a.out, step, stepUsage)

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

// aggregateTurn sums all usage records for a given turn ID.
// TODO: upstream an AggregateRecords([]Record) helper to the usage package.
func (a *Agent) aggregateTurn(turnID string) usage.Record {
	recs := a.tracker.Filter(usage.ByTurnID(turnID), usage.ExcludeEstimates())
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
