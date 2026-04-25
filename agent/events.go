package agent

import (
	"fmt"

	"github.com/codewandler/agentsdk/runner"
	coreusage "github.com/codewandler/agentsdk/usage"
	"github.com/codewandler/llmadapter/unified"
	"github.com/codewandler/miniagent/agent/display"
)

func (a *Agent) newRunnerEventHandler(turnID int) *runnerEventHandler {
	return &runnerEventHandler{
		agent:       a,
		turnID:      turnID,
		printedCall: map[string]bool{},
	}
}

type runnerEventHandler struct {
	agent          *Agent
	turnID         int
	stepDisplay    *display.StepDisplay
	stepUsage      coreusage.Record
	stepsCompleted int
	printedCall    map[string]bool
}

func (h *runnerEventHandler) handle(event runner.Event) {
	switch ev := event.(type) {
	case runner.StepStartEvent:
		display.PrintStepHeader(h.agent.out, ev.Step, ev.MaxSteps)
		h.stepDisplay = display.NewStepDisplay(h.agent.out)
		h.stepUsage = coreusage.Record{}
	case runner.RouteEvent:
		h.agent.applyRouteIdentity(ev)
	case runner.TextDeltaEvent:
		if h.stepDisplay != nil {
			h.stepDisplay.WriteText(ev.Text)
		}
	case runner.ReasoningDeltaEvent:
		if h.stepDisplay != nil {
			h.stepDisplay.WriteReasoning(ev.Text)
		}
	case runner.ToolCallEvent:
		h.printToolCall(ev.Call)
	case runner.ToolResultEvent:
		display.PrintToolResult(h.agent.out, ev.Output, ev.IsError)
	case runner.UsageEvent:
		rec := h.agent.recordRunnerUsage(h.turnID, ev)
		h.agent.tracker.Record(rec)
		h.stepUsage = coreusage.Merge(h.stepUsage, rec)
	case runner.StepDoneEvent:
		if h.stepDisplay != nil {
			h.stepDisplay.End()
			h.stepDisplay = nil
		}
		display.PrintStepUsage(h.agent.out, ev.Step, h.stepUsage, ev.Model)
		h.stepsCompleted++
		if ev.FinishReason == unified.FinishReasonLength {
			fmt.Fprintf(h.agent.out, "\n%s! model hit output token limit%s\n", display.BrightYellow, display.Reset)
		}
	case runner.ErrorEvent:
		if h.stepDisplay != nil {
			h.stepDisplay.End()
			h.stepDisplay = nil
		}
	}
}

func (h *runnerEventHandler) printToolCall(call unified.ToolCall) {
	if h.stepDisplay == nil {
		return
	}
	key := call.ID
	if key == "" {
		key = fmt.Sprintf("%s:%d", call.Name, call.Index)
	}
	args, _ := runner.ToolCallArgsMap(call)
	if len(args) == 0 || h.printedCall[key] {
		return
	}
	h.printedCall[key] = true
	h.stepDisplay.PrintToolCall(call.Name, args)
}
